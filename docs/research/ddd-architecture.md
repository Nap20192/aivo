# DDD-архитектура AIVO RMS: синтез

Дата: 2026-08-24. Синтез трёх отчётов (`docs/research/platform.md`, `docs/research/menu.md`,
`docs/research/pos.md`) + проверка по коду: `backend/internal/sharedkernel/`,
`backend/internal/domain/{platform,menu,pos}/`, `backend/pkg/session/`,
`backend/internal/pos/{ports,adapters/menubridge}/`, миграции, `CONTEXT-MAP.md`, `docs/PLATFORM.md`,
`docs/EVENTS.md`. Код не менялся; это архитектурное решение «как должно быть» и путь к нему.

---

## 1. Контекст-карта

Три bounded context'а: **Platform** (организации, auth, подписки, провижининг ресторанов, темы,
ассистент, customers/CRM), **Menu** (цифровое меню, заказы дайнера, сервис-запросы, handoff),
**POS** (смены, тикеты, fire). Все — один процесс, одна БД, Go-интерфейсы вместо RPC (ADR 0001).

### 1.1 Отношения в терминах DDD

```
                    ┌──────────────────────────────┐
                    │           Platform           │
                    │  (upstream почти для всех)   │
                    └──────┬──────────────┬────────┘
        customer-supplier  │              │  customer-supplier
        (тенант, лимиты,   │              │  (auth-сессия, кассир)
         default menu)     ▼              ▼
                    ┌──────────┐   ┌──────────┐
                    │   Menu   │◄──│   POS    │  conformist: POS живёт
                    │(upstream │   │(down-    │  в типах menu-домена;
                    │ для POS) │   │ stream)  │  menubridge — «полу-ACL»
                    └──────────┘   └──────────┘
                          ▲              ▲
                          └── shared kernel: internal/sharedkernel (ID, Entity,
                              AggregateRoot, DomainEvent) — у всех троих
```

| Пара | Отношение | Как выражено в коде |
|---|---|---|
| Platform → Menu | **Customer–supplier**, platform upstream по тенантности (владеет Restaurant-жизненным циклом, лимитами плана), но **downstream по данным заказов** (CRM читает `orders`/`order_lines` прямым SQL — `platform/adapters/postgres/customers.go:188`) | `Deps.Menu/MenuAdmin/MenuApp` в HTTP-композиции; `insertDefaultMenu` (`platform/adapters/postgres/postgres.go:283`) — платформа пишет прямо в `menus` |
| Menu → POS | **Customer–supplier + published contract**: menu — supplier агрегата Handoff и каталога; POS — потребитель | `pos/ports.Menu` (`pos/ports/ports.go:72`), `menubridge`, handoff preview/accept |
| POS → Menu (типы) | **Conformist**: POS использует типы menu-домена как свои | `pos/ports/ports.go:10` импортирует `menudomain`; `pos/app` вызывает `menudomain.NewOrderLine` для снапшота строк |
| POS → Platform | Customer–supplier: сессия, роли, имя кассира | middleware `h.pos` (`platform/adapters/http/pos.go:32`), fallback `Platform.User` (`pos.go:87`); FK `shifts.opened_by → users(id)` |
| Все ↔ sharedkernel | **Shared kernel** в узком смысле: только строительные блоки, не бизнес-типы | `internal/sharedkernel` — импортируется всеми domain-пакетами, сам не импортирует никого |

Отдельная аномалия: **`pkg/session` — «нелегальный» участник карты.** `pkg/session/ratelimit.go:7`
импортирует `aivo/internal/domain/menu` (тип `ServiceRequestKind` в `serviceKey`, `ratelimit.go:29-32`).
`pkg/` по определению — технические утилиты без знания домена; пакет сам признаётся в шапке
`cookie.go:1-7`, что это Menu-plumbing. Это нарушение направления зависимостей (pkg → internal/domain)
и заодно третий, рассинхронизированный экземпляр инварианта дедупа сервис-запросов (детали и баги —
menu-отчёт §1.3). Решение: см. Топ-10, п. 2.

### 1.2 Таблица `restaurants`: кто владеет

Факт: базовую таблицу создаёт **menu** (`migrations/menu/0001_init.up.sql:5` — id, slug, name),
**platform** расширяет её своими колонками (`migrations/platform/0001_init.up.sql:48` —
`ALTER TABLE restaurants ADD org_id/address/hours/contacts`) и провиженит строки. Go-типов два:
узкая проекция `menudomain.Restaurant{ID,Slug,Name}` (`domain/menu/domain.go:17`) и полная
`platformdomain.Restaurant` (`domain/platform/domain.go:162`).

**Вердикт: владелец — Platform.** Аргументы: жизненный цикл ресторана (регистрация, провижининг,
настройки, кастомный домен, событие `RestaurantProvisioned`) целиком в platform; menu ресторан
не создаёт и не удаляет — только ссылается для тенант-изоляции. Историческая инверсия (menu был
первым контекстом, platform «доехал» позже) — это груз истории, а не дизайн.

**Что делать — логическая передача, без физической:**

1. Зафиксировать в `CONTEXT-MAP.md`: агрегат Restaurant принадлежит platform; menu и pos держат
   **проекции-ссылки** (id, slug, name), которые не являются агрегатами их контекстов.
2. Все будущие изменения схемы `restaurants` — только в `migrations/platform/`.
3. `insertDefaultMenu` (`platform/adapters/postgres/postgres.go:283` — прямой INSERT в `menus`) —
   единственное место, где platform *пишет* в чужие таблицы ради атомарности провижининга. Оставить
   как осознанный компромисс, но оформить портом (узкий интерфейс `ProvisionDefaultMenu` со стороны
   menu, реализуемый menu-store'ом и вызываемый в транзакции platform) — чтобы связь была видна
   в сигнатурах, а не только в SQL-строке.
4. **Не** переносить `CREATE TABLE restaurants` в platform-миграции: это флипает порядок накатки
   батчей (menu-таблицы держат FK на restaurants и накатываются первыми). Переписывание истории
   миграций ради чистоты каталога — цена без выгоды, пока БД одна. Физический перенос — только
   если контексты когда-нибудь разъедутся по базам.

### 1.3 `customer_id` без FK в чужих контекстах

Три ссылки на platform-таблицу `customers` из чужих контекстов, оформленные по-разному:

| Колонка | FK? | Где |
|---|---|---|
| `orders.customer_id` (menu) | **Да** — FK добавляет platform-миграция (`platform/0003_customers.up.sql:25-26`, `ON DELETE SET NULL`) | колонка — `menu/0004:7` |
| `cart_handoffs.customer_id` (menu) | Нет («no FK, see header note») | `menu/0004_customers_handoffs.up.sql:20` |
| `tickets.customer_id` (pos) | Нет («no FK (cross-batch)») | `pos/0003_shift_cashier_ticket_customer.up.sql:8` |

**Вердикт: мягкая ссылка (без FK) — правильный паттерн для кросс-контекстной ссылки**, и pos/menu-
handoff уже делают верно: `customer_id` — это value object «идентификатор из чужого контекста»,
консистентность которого гарантирует владелец (platform), а не схема потребителя. FK на `orders` —
асимметрия: platform-миграция навязывает constraint чужой таблице, и порядок «сначала колонка в
menu-батче, потом FK в platform-батче» — ровно тот cross-batch-костыль, от которого pos/0003 честно
отказался.

Решение: конвенцию закрепить — **новые кросс-контекстные ссылки всегда без FK**; существующий FK на
`orders` не трогать (в одной БД он безвреден и даёт `SET NULL` при удалении аккаунта — полезно для
GDPR-подобного сценария), но код не должен на него полагаться, и при первом же расхождении батчей
он удаляется первым. Записать это в `CONTEXT-MAP.md` вместе с фактом «CRM platform читает
`orders/order_lines/tickets/ticket_lines` прямым SQL» (`platform/adapters/postgres/customers.go:188-210`)
— это read-модель поверх чужих таблиц, допустимая в монолите, но она молча сломается при миграциях
menu/pos, поэтому обязана быть задокументированной связью, а не сюрпризом.

### 1.4 Handoff как контракт между Menu и POS

Handoff — лучший пример осознанной границы в системе. Menu **владеет агрегатом** `Handoff`
(`domain/menu/handoff.go:49`) и всеми его инвариантами: single-use (`UsedAt` + условный UPDATE),
TTL 15 мин (`HandoffTTL`, `handoff.go:12`), ≤1 активного на стол, уникальность кода среди активных.
POS — **потребитель контракта**: preview (без email/телефона клиента) и accept.

Контрактность видна в самом типе `HandoffLine` (`handoff.go:37-44`): строка несёт *и* снапшот
(name/price/options — для превью официанту), *и* source-IDs (`MenuItemID`, `OptionIDs` — чтобы
accept шёл обычным POS-путём `AddLines` с перевалидацией через `menudomain.NewOrderLine`). Это не
размытая граница, а published language: menu публикует ровно то, что нужно потребителю, и POS не
доверяет снапшоту, а перепроверяет по живому меню.

Проблема не в контракте, а в том, **где живёт его исполнение**: вся сага accept
(consume `MarkHandoffUsed` → `posapp.AddLines` → компенсация `UnmarkHandoffUsed` на
`context.WithoutCancel` → link customer → touch guest) лежит в HTTP-адаптере platform
(`platform/adapters/http/handoff.go:219-269`), дергающем сторы **трёх** контекстов.

**Разрешение противоречия отчётов** (menu-отчёт, шаг 10: «`AcceptHandoff` в `menu/app` с
портом-колбеком на POS» vs pos-отчёт, шаг 4: «`posapp.AcceptHandoff` с расширением `ports.Menu`»):
**прав pos-отчёт**. Направление зависимости pos → menu уже существует и легитимно (`ports.Menu`,
menubridge); вариант menu-отчёта создал бы обратную зависимость menu → pos, которую ADR 0002
сознательно запрещает (Order/Menu отвязаны от till/shift). Инициатор accept — официант (актор POS),
результат — POS-тикет. Итог:

- `CreateHandoff` (валидация, кулдаун, генерация кода с ретраем) → `menu/app` (menu-отчёт, шаг 10 —
  в этой части верен);
- `AcceptHandoff` (consume → AddLines → компенсация → link) → `pos/app`; операции
  `HandoffByCode/MarkHandoffUsed/UnmarkHandoffUsed` добавляются в `pos/ports.Menu` и реализуются
  в menubridge;
- событие `HandoffAccepted` получает единственного хозяина — `posapp.AcceptHandoff` (снимает
  замечание «у события два хозяина» из pos-отчёта §3.4).

### 1.5 menubridge как ACL

`pos/adapters/menubridge/menubridge.go` — заявленный anticorruption layer, фактически **полу-ACL**:
ошибки транслируются (`mapErr`, `menubridge.go:27-32`: `menuports.ErrNotFound` → `ports.ErrNotFound`),
а типы — нет: методы возвращают `menudomain.MenuItem/Table/ServiceRequest` как есть, и даже сам порт
`pos/ports/ports.go:10` импортирует `menudomain`.

**Вердикт: официально признать отношение конформистским и не строить полный ACL.** Настоящий ACL
(POS-собственные DTO + маппинг в bridge) окупается, когда upstream-модель чужая/нестабильная/враждебная.
Здесь оба контекста в одном репозитории, у одной команды, а всё, что POS *хранит*, он и так снапшотит
в свои типы (`TicketLine`, `LineOption` — `domain/pos/domain.go:95,87`); утечка menu-типов ограничена
транзитным использованием в app-слое. Переиспользование `menudomain.NewOrderLine` из pos/app — тоже
осознанный конформизм: одно доменное правило валидации строки вместо копипасты.

Что оставить обязательным в bridge навсегда: (а) трансляция ошибок — POS никогда не видит чужих
сентинелей; (б) это **единственная** точка, где POS-код зовёт menu-код. Апгрейд до полного ACL —
только по триггеру «изменение menu-домена сломало компиляцию POS более одного раза за квартал».

---

## 2. Shared kernel

Сейчас (`backend/internal/sharedkernel/`): `ID = uuid.UUID` (alias, `entity.go:15`), `NewID/ParseID`,
`Entity{ID, CreatedAt}` (`entity.go:26`), `AggregateRoot` с буфером событий (`aggregate.go:6`),
`DomainEvent` + `EventBase` (`event.go:8,15`). Правило «не импортирует ни один контекст» соблюдено.

**Что должно быть (и есть):** ровно этот набор. Shared kernel — это код, изменение которого требует
согласия всех трёх контекстов, поэтому он обязан быть маленьким и почти неизменным. Identity,
базовый Entity, механика подъёма событий, интерфейс события — идеальные кандидаты: стабильны,
бизнес-смысла не несут.

**Единственный допустимый кандидат на добавление:** `type Cents int` — деньги в центах пересекают
границы контекстов (handoff-строки, payload'ы событий `total_cents`, контракт PLATFORM.md «integer
cents everywhere»). Но это опция, не требование; `Money{amount, currency}` — не добавлять (YAGNI до
мультивалютности, консенсус всех трёх отчётов).

**Чего там быть НЕ должно (проверочный список для будущих PR):**

- **Бизнес-типов любого контекста** — `Restaurant`, `Customer`, `Role`, `Plan`, статусы. Как только
  «общий» тип с поведением попадает в kernel, каждое его изменение блокируется тремя контекстами.
  Ресторан-проекция в menu (`menudomain.Restaurant`) — намеренно дублированная узкая копия, и это
  правильнее, чем общий тип.
- **Конкретных событий и их payload'ов** — каталог живёт в `docs/EVENTS.md`, типы событий — в
  domain-пакете контекста-владельца. Kernel даёт только интерфейс `DomainEvent`.
- **Портов и ошибок-сентинелей** — у каждого контекста свои (`menuports.ErrNotFound` ≠
  `pos/ports.ErrNotFound` — и menubridge существует ровно чтобы их разделять).
- **Утилит** — crypto, qrcode, session-куки остаются в `pkg/` (после развязки от домена, §1.1).
- **«Библиотеки на вырост»** — generic-репозиториев, event bus, UnitOfWork-абстракций. Kernel
  сейчас и так декоративен (единственный используемый элемент — `ID`; `AggregateRoot.Raise` не
  вызывается нигде, кроме `sharedkernel_test.go`). Он оправдан только потому, что план подключения
  outbox (§4) его задействует; расширять его до первого реального использования нельзя.

---

## 3. Итоговый список агрегатов системы

Сводка трёх отчётов; противоречий по границам агрегатов между отчётами нет (разногласие было только
по владению use case'ом accept — разрешено в §1.4). Session/CustomerSession, read-модели
(`GuestSummary`, `GuestOrder`, `CustomerOrder`, floor view POS) и `NotificationChannel`
(конфиг-запись 1:1) — **не агрегаты** и в список не входят.

### Platform

| Агрегат (корень) | Граница | Ключевые инварианты | Где enforced сейчас → куда |
|---|---|---|---|
| **Organization** | только org (name) | валидное имя | app → ок |
| **Subscription** (identity = OrgID) | план + статус | state machine `subTransitions` (`domain/platform/domain.go:125`), canceled терминален; free ⇒ active | домен `Transition`, **обходится** в `App.ChangePlan` (`platform/app/app.go:282,290`) → метод `Subscription.ChangePlan` только через `Transition` |
| **User** | учётка + роль + скоуп | роль ∈ enum; owner ⇒ `RestaurantID == nil`, staff ⇒ `!= nil` | конвенция в `Register`/`AddStaff` → фабрики `NewOwner`/`NewStaff` |
| **Restaurant** | реквизиты + hours + contacts + claim домена | валидный slug (`ValidSlug`, `domain.go:178`), ≤20 hours, валидный hostname; глобальная уникальность slug/domain — БД (правильно) | app + regex в app → VO `Slug`/`Hostname` в домене |
| **RestaurantTheme** (identity = RestaurantID) | theme JSON + design_md | accent enum, css-injection guard (`ValidCSSVar`), лимиты 64KB/256KB | `App.SaveTheme` (единая точка — хорошо) → доменный `Theme.Apply` |
| **AssistantThread** (+ AssistantMessage) | тред + сообщения | **decide-once**; решаемы только assistant-сообщения с действиями; действия — allowlist + тенант-скоуп refs | хендлер + SQL-предикат (`platform/adapters/postgres/assistant.go:119`) → `Thread.DecideMessage`, SQL остаётся защитой от гонки |
| **Customer** | аккаунт дайнера | email валиден/уникален (БД) | ок |
| **GuestProfile** (identity = (RestaurantID, CustomerID)) | notes + tags + first/last_seen | лимиты notes/tags; приватность «нет строки — нет видимости» = структурный инвариант схемы | app + схема → ок, задокументировать |

### Menu

| Агрегат (корень) | Граница | Ключевые инварианты | Где enforced сейчас → куда |
|---|---|---|---|
| **Order** | Order + OrderLine[] (VO-снапшоты) + OrderLineOption[] | ≥1 строки; строки валидны на момент подачи (qty≥1, available, опции принадлежат item, single/multi-группы — **дыра**, дубликаты опций считаются дважды, `domain/menu/domain.go:187-199`); снапшот неизменяем; write-once (без lifecycle — ADR 0002) | фабрика `NewOrderLine` (`domain.go:179`) + HTTP + store'ы → `domain.NewOrder` + группы/дубли в `NewOrderLine`; нужен `Order.TotalCents()` |
| **Handoff** | Handoff + HandoffLine[] | single-use; TTL 15 мин; ≤1 активного на стол; код уникален среди активных; строки валидированы при создании | БД + SQL + HTTP-адаптер platform → протокол в `menu/app` (create) и `pos/app` (accept), §1.4 |
| **MenuItem** | item + OptionGroup[] + Option[] + Allergen[] | цена ≥ 0 (**нигде** нет — ни CHECK, ни фабрики); аллергены — EU-14 enum; опции wholesale-редактирование | адаптер (scoped-подзапросы — хорошо) → CHECK в миграцию + валидация цены на границе |
| **Menu** | справочник | ровно один default на ресторан (межагрегатный — партиальный индекс, правильно); правила удаления `CanDeleteMenu` (`domain.go:58`) | БД + домен → ок |
| **Category** | справочник | принадлежность меню/ресторану | scoped SQL → ок |
| **Table** | стол + токен | глобально уникальный токен; регенерация = мгновенная инвалидация | БД → ок |
| **ServiceRequest** | сам по себе | ≤1 pending на (table, kind) — межагрегатный, БД-индекс (правильно); lifecycle `pending → acknowledged \| dismissed` — **строка без guard'а** (`domain.go:248`, любой UPDATE в `admin.go:339`) | БД + in-memory дубль в `pkg/session` (**убрать**) → typed status + `Close(outcome)` |

`LandingBlock` — полумёртвая ветка (живая v1-поверхность её не рендерит, админ-CRUD нет); судьбу
решить (удалить или доделать), в целевой список агрегатов не включаю.

### POS

| Агрегат (корень) | Граница | Ключевые инварианты | Где enforced сейчас → куда |
|---|---|---|---|
| **Shift** | смена без детей (тикеты — отдельно, намеренно) | одна открытая на ресторан (межагрегатный — partial unique index, правильно); иммутабельность после закрытия (`Shift.Close`, `domain/pos/domain.go:41` + WHERE-guard — лучший инвариант системы); expected = float + sales — **но sales снимается вне транзакции закрытия: гонка потери денег** (pos-отчёт §3.2) | домен + БД, гонка в app (`pos/app/app.go:127-155`) → атомарный транзакционный `CloseShift` |
| **Ticket** | Ticket + TicketLine[] | добавление только в открытый; закрытый иммутабелен (**дыра**: `FireTicket`/`AppendTicketNote`/`LinkTicketCustomer` статус не проверяют — `pos/adapters/postgres/postgres.go:300,167,290`); один открытый на стол (БД-индекс, правильно); снапшот строк | «получается» из формы SQL-запросов → `AND status='open'` в три WHERE + доменные `Ticket.Fire/Close` |

Кросс-агрегатные правила, которые **правильно** держит БД и не надо тащить в агрегаты: все
партиальные уникальные индексы (default-меню, open shift, open ticket, pending request, active
handoff code), глобальная уникальность slug/domain/email/table-token.

---

## 4. Интеграция: события vs прямые вызовы

Инфраструктура: outbox-таблица `events` (`migrations/platform/0004_events.up.sql`) с
`events_pending`-индексом, sqlc-запросы `InsertEvent/PendingEvents/MarkEventPublished`,
`sharedkernel.AggregateRoot.Raise/Events`, каталог `docs/EVENTS.md`. **Всё мертво: ни одной записи,
ни одного Raise, паблишера нет** — единогласный вывод всех трёх отчётов.

### Правило выбора

> **Если use case не может завершиться без ответа другого контекста — синхронный порт.
> Если другой контекст лишь реагирует на свершившийся факт — событие через outbox.
> Никогда не оба для одного взаимодействия.**

Синхронные порты (уже есть и остаются):

- POS → Menu: `ports.Menu` через menubridge — снапшот строки невозможен без живого item; accept
  невозможен без consume кода. Синхронность обязательна.
- Platform → Menu: лимит блюд (`ItemLimitFor`), команды ассистента, default menu при провижининге —
  ответ нужен внутри транзакции/запроса.
- POS → Platform: auth-middleware, имя кассира.

События (писать в outbox в той же транзакции, что и изменение агрегата, — как велит шапка
EVENTS.md):

- Аудит AI (`AssistantActionsApplied/Discarded` — сейчас только slog, а AGENTS.md требует
  устойчивый лог AI-рекомендаций);
- CRM/аналитика (`OrderPlaced`, `TicketClosed`, `TicketLinesAdded` — сейчас CRM читает чужие
  таблицы SQL-ом; события — путь к развязке этой связности);
- Прочее из каталога: `ShiftOpened/Closed`, `ServiceRequested/Closed`, `HandoffCreated/Accepted`,
  `RestaurantProvisioned`, `SubscriptionChanged` и т.д.

### Порядок подключения (консенсус отчётов: не раньше первого потребителя)

1. Сначала — дешёвые правки каталога `docs/EVENTS.md` (можно сразу, это документ): убрать `code` из
   `HandoffCreated` (предъявительский секрет живёт дольше TTL в outbox — menu-отчёт §3.3), добавить
   `restaurant_id` в payload `ServiceRequested`, добавить `ServiceRequestClosed`, `cashier` +
   `opening_float_cents` в `ShiftOpened`, `sales_cents` в `ShiftClosed`, семантику идемпотентного
   `LinesFired` (только реально прошитые строки, через `UPDATE ... RETURNING id`), точку эмиссии
   `TicketClosed` = закрытие смены в v1, новые `TicketLinesAdded`, `StaffUserAdded`,
   `AssistantActionsApplied/Discarded`, `CustomDomainClaimed/Released`.
2. Первое проводное событие — то, у которого есть реальный потребитель. Кандидат №1:
   `AssistantActionsApplied` (потребитель — аудит, заменяет slog). Кандидат №2: `OrderPlaced`
   (потребитель — будущий CRM-консьюмер вместо прямого SQL).
3. Механика: embed `sharedkernel.AggregateRoot` в корень, `Raise` в доменном методе/фабрике, app
   пишет `Events()` в `events` **перед commit той же транзакции**. Это требует транзакционного
   порта у app — он появляется естественно из фикса атомарного `CloseShift` (Топ-10, п. 1).
4. Паблишер (poll `events_pending` → deliver → `MarkEventPublished`, at-least-once, консьюмеры
   идемпотентны по `id`) — только вместе с первым **внешним** потребителем. До того потребители
   могут читать outbox напрямую как журнал.

Если embed-модель `AggregateRoot` не приживётся за два первых события — честно удалить её из
sharedkernel и писать outbox-строки прямо из app (запасной вариант из menu-отчёта, шаг 13); хуже
всего — держать мёртвую механику рядом с работающей другой.

---

## 5. Целевая структура пакетов и путь миграции

### Целевая структура

Сейчас домен живёт отдельно от контекстов (`internal/domain/{platform,menu,pos}` — см.
`CONTEXT-MAP.md`), что маскирует владение: `pkg/session` импортирует «просто domain/menu», POS
импортирует «просто domain/menu», и границы контекста не видно в путях. Цель — домен внутри
контекста:

```
backend/internal/
  sharedkernel/                  # без изменений: ID, Entity, AggregateRoot, DomainEvent
  platform/
    domain/                      # ← из internal/domain/platform
    app/  ports/  adapters/{http,postgres,billing,claudecli,s3}
  menu/
    domain/                      # ← из internal/domain/menu
    app/  ports/  adapters/{postgres}          # легаси http удалён
  pos/
    domain/                      # ← из internal/domain/pos
    app/  ports/  adapters/{postgres,menubridge}
backend/pkg/                     # crypto, qrcode, session — БЕЗ импортов internal/*
```

Правила импортов (закрепить в `CONTEXT-MAP.md` и проверять ревью):

1. `<ctx>/domain` → только `sharedkernel` + stdlib.
2. `<ctx>/app` → свой domain, свои ports, sharedkernel.
3. `<ctx>/adapters` → всё своего контекста; чужой контекст — **только** в выделенных
   bridge-адаптерах (menubridge; будущий порт default-menu) и только его `domain`/`ports`.
4. Кросс-контекстный импорт `pos → menu/domain` легален (конформизм, §1.5); `menu → pos` и
   `menu/pos → platform` (кроме sharedkernel) — запрещены.
5. `pkg/*` → никаких `internal/*`.

### Путь миграции (каждый шаг — отдельный коммит, `go build ./... && go test ./...` зелёные)

Шаги 1–4 — снятие блокеров (нарушений направления зависимостей), 5–7 — механический переезд,
8–10 — стягивание логики на свои места. Поведение не меняется нигде, кроме шага 1 (он чинит баг).

1. **Развязать `pkg/session` от домена.** Удалить `AllowServiceRequest`/`MarkAcknowledged` и тип
   `serviceKey` (`ratelimit.go:29-32,66-85`); в `SubmitServiceRequestHandler`
   (`menu/app/submit_service_request.go:49`) вместо них ловить unique violation индекса
   `service_requests_open_per_table_kind_idx` (или звать существующий `Store.HasOpenServiceRequest`,
   `menu/ports/store.go:70`) → `ErrServiceRequestAlreadyOpen` → 409. Импорт
   `aivo/internal/domain/menu` из `ratelimit.go:7` исчезает. Заодно закрыты баги «500 после
   рестарта» и «стол заблокирован после ack» (menu-отчёт §1.3). Тест: повторный запрос того же
   kind → 409; после dismiss — новый запрос проходит.
2. **Удалить мёртвый код.** `menu/adapters/postgres/menudb/` + `queries/menu/`,
   `pos/adapters/postgres/posdb/` + `queries/pos/` (sqlc-генераты, ни одного импортёра — либо так,
   либо решение мигрировать сторы на sqlc, но не «оба слоя рядом»); легаси
   `menu/adapters/http/handlers.go` (не смонтирован в `cmd/aivo-server`, поведение разошлось с v1) —
   удалить, перенеся ценные тест-сценарии на v1-поверхность. Компилируется тривиально.
3. **Переместить `internal/domain/menu` → `internal/menu/domain`.** Механический `gopls rename`
   пути импорта; импортёры (menu/app, menu/adapters, pos/app, pos/ports, menubridge,
   platform/adapters/http) правятся автоматически. Один коммит.
4. **Переместить `internal/domain/pos` → `internal/pos/domain`** — то же самое, импортёров меньше.
5. **Переместить `internal/domain/platform` → `internal/platform/domain`** — то же; обновить
   `CONTEXT-MAP.md` (раздел «Domain model lives apart» переписать на «domain внутри контекста»).
6. **`AcceptHandoff` → `pos/app`.** Расширить `pos/ports.Menu` методами
   `HandoffByCode/MarkHandoffUsed/UnmarkHandoffUsed`, реализовать в menubridge (трансляция ошибок
   как в `mapErr`), перенести сагу из `platform/adapters/http/handoff.go:219-269` в
   `posapp.AcceptHandoff`. Хендлер — декодер + вьюха. Юнит-тест саги с фейковыми портами
   (консумация, компенсация при ошибке AddLines).
7. **`CreateHandoff` → `menu/app`.** Перенести из `handoff.go:28-138` валидацию (лимиты 1..50/≤500 —
   константы рядом с `HandoffTTL` в `menu/domain`), кулдаун, ретрай кода. Тест на ретрай при
   `ErrConflict`.
8. **`AssistantService` → `platform/app`.** Перенести `assistantPrompt`, `applyAction`,
   батч-семантику apply из `platform/adapters/http/assistant.go:205-564` в app-методы
   `SendMessage/ApplyActions/DiscardActions`; понадобится узкий порт на MenuAdmin-команды (прецедент
   — `ItemLimitFor`). Исчезает дублирование «position нового меню» с `menuadmin.go:68-77`.
9. **Лимит блюд плана → `platform/app`.** `App.EnsureCanCreateItem(orgID, restaurantID)` вместо
   проверки в хендлере `menuadmin.go:459-473` — симметрия с лимитом ресторанов в `CreateRestaurant`.
10. **Политика TouchGuest в одну точку** app-слоя (сейчас три вызова в двух файлах:
    `diner.go:339`, `handoff.go:107`, `handoff.go:262`).

После шага 10 HTTP-адаптеры platform — только парсинг/куки/коды/вьюхи, и структура пакетов
совпадает с целевой. Дальше — обогащение домена и события (Топ-10, пп. 5–10) в любом порядке.

---

## 6. Топ-10 действий по приоритету

Критерий сортировки: сначала деньги и целостность данных, потом двойные источники правды, потом
структура. Пункты 1–5 — до любых переездов пакетов.

1. **Атомарное закрытие смены** (pos-отчёт §3.2, шаг 1). Один транзакционный метод стора:
   `SELECT ... FOR UPDATE` смены → закрыть тикеты → SQL-агрегат sales по уже закрытым →
   `Shift.Close` → `UPDATE shifts`. Сейчас `App.CloseShift` (`pos/app/app.go:127-155`) — пять
   запросов без общей транзакции; строки, добавленные между снимком продаж и `CloseTickets`, молча
   выпадают из кассовой сверки, а частичный отказ оставляет тикеты-сироты под закрытой сменой.
   Money path — вне очереди.
2. **Дедуп сервис-запросов: одна правда — БД** + развязка `pkg/session` от `internal/domain/menu`
   (шаг 1 плана §5). Чинит два прод-бага и единственное нарушение направления зависимостей уровня
   pkg→domain.
3. **Иммутабельность закрытого тикета**: `AND status='open'` в `FireTicket`, `AppendTicketNote`,
   `LinkTicketCustomer` (`pos/adapters/postgres/postgres.go:300-311,167-178,290-298`) + проверка
   RowsAffected в `LinkTicketCustomer`; в `App.AddLines` — отказ при тикете чужой смены.
4. **`ChangePlan` через state machine**: метод `Subscription.ChangePlan(plan, providerStatus)`
   поверх `Transition`, вместо ручной сборки `Subscription{}` в `platform/app/app.go:282,290` —
   сейчас `canceled` фактически не терминален. Плюс типизированный конфликт слага вместо
   `strings.Contains(err.Error(), "slug")` (`app.go:166-169`).
5. **`NewOrderLine`: enforce single/multi-групп и запрет дублей опций** (`menu/domain/domain.go:187-199`)
   + верхняя граница qty. Одна функция чинит и заказ дайнера, и handoff. Рядом — CHECK
   `price_cents >= 0` в миграцию.
6. **Handoff по своим местам**: `AcceptHandoff` → `pos/app`, `CreateHandoff` → `menu/app` (§1.4;
   шаги 6–7 плана §5). Попутно попробовать одну транзакцию на shared `*sql.DB` вместо
   consume/compensate (ponytail-долг, помеченный в самом коде).
7. **Удалить мёртвое**: `menudb`, `posdb`, легаси `menu/adapters/http` (шаг 2 плана §5). Три
   носителя энтропии, ноль пользователей.
8. **Переезд domain-пакетов внутрь контекстов** (шаги 3–5 плана §5) + фиксация правил импортов и
   решений §1 (владение restaurants, конвенция customer_id-без-FK, конформизм POS) в
   `CONTEXT-MAP.md`.
9. **`AssistantService` и лимит блюд → app** (шаги 8–9 плана §5) — последние крупные куски
   бизнес-логики в HTTP-адаптерах.
10. **События**: правки `docs/EVENTS.md` (дешёво, сразу — список в §4) и первое проводное событие с
    реальным потребителем (`AssistantActionsApplied` или `OrderPlaced`), через
    `sharedkernel.AggregateRoot` в транзакции команды. Паблишер — только с первым внешним
    консьюмером.

Чего **не** делать (консенсус отчётов): Money-VO с валютами, generic-репозитории, event bus поверх
пустого outbox, полный ACL в menubridge, вынос контекстов в отдельные сервисы/gRPC (ADR 0001: «не
раньше второго процесса»), физический перенос `CREATE TABLE restaurants`.
