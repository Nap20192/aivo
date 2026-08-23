# Ресёрч bounded context MENU

Дата: 2026-08-24. Область: `backend/internal/menu/{app,ports,adapters}`, `backend/internal/domain/menu/`,
`backend/internal/sharedkernel/`, `backend/migrations/menu/`, `backend/pkg/session/`, контракт `docs/PLATFORM.md`,
каталог событий `docs/EVENTS.md`. Соседние контексты (platform, pos) рассматриваются только в точках стыка.

---

## 1. Логика as-is: сценарии end-to-end

### 1.0. Две HTTP-поверхности — важная вводная

У контекста Menu сейчас **две** HTTP-поверхности, и живая — не «своя»:

1. **Легаси-адаптер** `internal/menu/adapters/http/handlers.go` (`/api/landing/...`, `/api/menu/...`,
   `POST /api/orders`, `POST /api/service-requests`, `/api/qr/...`). Его `NewMux`
   (`handlers.go:34`) **нигде не смонтирован** в `cmd/aivo-server/main.go` — он живёт только в тестах.
2. **Актуальная поверхность v1** живёт в чужом адаптере: `internal/platform/adapters/http/diner.go` и
   `internal/platform/adapters/http/handoff.go` (`/api/v1/t/{token}/...`, `/api/v1/m/...`, `/api/v1/pos/handoff/...`).
   Она вызывает `menuapp.Application` для заказов/сервис-запросов, но для входа, handoff и админ-CRUD ходит
   напрямую в `menuports.Store`/`AdminStore`, минуя app-слой Menu.

Поведение двух поверхностей уже разъехалось (ключ кулдауна заказа — см. 1.2). Это главный источник
«домашней» энтропии контекста.

### 1.1. Вход дайнера по столу

`GET /api/v1/t/{token}` → `platform/adapters/http/diner.go:118` (`dinerEntry`):

- `MenuAdmin.TableByTokenGlobal` (`menu/adapters/postgres/admin.go:39`) — токен глобально уникален
  (миграция `0002_global_table_token.up.sql`), слуг не нужен; неизвестный/чужой токен → одинаковый 404.
- Далее адаптер сам агрегирует: ресторан+тема из platform, `Store.Menu` (все категории+позиции ресторана),
  `AdminStore.Menus` (список меню, default первым), `PendingServiceRequestsForTable` (открытые запросы стола).
- На success-path выдаётся анонимная сессионная кука (`session.IssueOrRefresh`, `pkg/session/cookie.go:27`,
  httpOnly, sliding TTL 5ч).

Инвариант тенант-изоляции «токен = учётные данные» enforced в запросе (`WHERE token = $1`, глобальная
уникальность в БД), а для пары (slug, token) — в `app/resolve.go:20` (`resolveTable`), единственной точке
резолва для команд Menu.

### 1.2. Заказ дайнера (Order)

`POST /api/v1/t/{token}/orders` → `diner.go` (`dinerOrder`) → `app.SubmitOrderHandler.Handle`
(`app/submit_order.go:62`):

1. `resolveTable` (slug+token, scoped) → 404 при несовпадении.
2. Кулдаун: `session.AllowOrder(cmd.SessionID)` — не чаще 1 заказа в 30с (`pkg/session/ratelimit.go:49`),
   in-memory map. **Ключ различается по поверхностям**: v1-адаптер передаёт `SessionID: table.Token`
   (`diner.go`, комментарий «cooldown key: unforgeable») — кулдаун фактически **на стол**, а не на сессию;
   легаси-адаптер передаёт cookie-сессию (`menu/adapters/http/handlers.go:299,309`). `CONTEXT.md` («Diner
   session»: max 1 per 30s **per session**) описывает легаси-поведение и уже неточен.
3. `store.Menu(restaurantID)` — грузится **весь** каталог ресторана, строится map по ID.
4. Для каждой строки — `domain.NewOrderLine(item, optionIDs, qty)` (`domain/menu/domain.go:179`):
   qty ≥ 1, item.Available, каждый option_id принадлежит item. Это единственное место конструирования
   снапшота строки (name/price/labels фиксируются на момент заказа).
5. `store.CreateOrder` (`menu/adapters/postgres/postgres.go:290`) — транзакция orders → order_lines →
   order_line_options; ID/CreatedAt генерирует store; пустой список строк отбивается store'ом (`:291`).
6. `notifyOrder` (`app/notify.go:18`) — best-effort Telegram: достать `NotificationChannel`, расшифровать
   bot token (AES-256-GCM, `pkg/crypto`), отправить; любая ошибка только логируется — заказ уже сохранён.

Где enforced инварианты заказа:

| Инвариант | Где |
|---|---|
| qty ≥ 1, item доступен, option принадлежит item | domain: `NewOrderLine` (`domain.go:179-209`) |
| ≥ 1 строки в заказе | HTTP-адаптеры (`handlers.go:287`, `diner.go`) + store (`postgres.go:291`, `memory.go:182`) — **не домен** |
| снапшот не переписывается задним числом | конструкция: снапшот-колонки `order_lines`/`order_line_options` (`0001_init.up.sql:89-108`), UPDATE меню их не трогает |
| кулдаун 30с | `pkg/session` in-memory (`ratelimit.go:49`) |
| тенант-изоляция | `resolveTable` + `WHERE restaurant_id` во всех запросах store |

**Дыра в инварианте опций**: `NewOrderLine` игнорирует структуру групп — `OptionGroup.Multi=false`
(«ровно один выбор») не проверяется, дубликаты option_id допускаются и считаются в цену дважды
(`domain.go:187-199`: все опции item сплющены в одну map, выбор валидируется только на принадлежность).
Верхней границы qty тоже нет.

### 1.3. Сервис-запрос (позвать официанта / счёт)

`POST /api/v1/t/{token}/requests` → `diner.go` (`dinerRequest`) → `app.SubmitServiceRequestHandler.Handle`
(`app/submit_service_request.go:41`):

1. `resolveTable`.
2. Дедуп «максимум один открытый запрос данного вида на стол»: `session.AllowServiceRequest(table.ID, kind)`
   (`ratelimit.go:66`) — **in-memory** map с TTL 2 минуты, успешный вызов сам помечает пару (table, kind)
   занятой.
3. `store.CreateServiceRequest` (`postgres.go:342`) — insert со status `pending`.
4. best-effort Telegram-уведомление.

Тот же инвариант продублирован в БД партиальным уникальным индексом
`service_requests_open_per_table_kind_idx` (`0001_init.up.sql:123-125`), и есть портовый метод
`Store.HasOpenServiceRequest` (`ports/store.go:70`), причём док-комментарий `ports/store.go:61` требует:
«Callers MUST check HasOpenServiceRequest first». **Фактически его не вызывает никто** (только реализации и
тесты) — app-слой использует in-memory `pkg/session`. Три несогласованные реализации одного инварианта дают
реальные баги:

- После рестарта процесса map пуст, а в БД висит pending-строка → insert падает об уникальный индекс →
  диенер получает **500**, а не 429/409 «already open».
- Обратная рассинхронизация: POS подтверждает запрос через `menubridge.AckServiceRequest`
  (`pos/adapters/menubridge/menubridge.go:52`) → `SetServiceRequestStatus` в БД, но
  `session.MarkAcknowledged` (`ratelimit.go:81`) **не вызывается нигде в проде** — после ack стол всё равно
  заблокирован на повторный запрос до истечения 2-минутного TTL in-memory записи.

Закрытие запроса: POS-поверхность `POST /api/v1/pos/requests/{id}/ack|dismiss` → menubridge →
`AdminStore.SetServiceRequestStatus` (`admin.go:339`) — переход хранится как сырая строка, никакой машины
состояний в домене нет (см. §3).

### 1.4. Cart handoff (корзина по коду для официанта)

Создание — `POST /api/v1/t/{token}/handoff` → `platform/adapters/http/handoff.go:28` (`dinerHandoff`).
Целый use case живёт прямо в HTTP-адаптере platform:

1. Резолв стола по токену; границы ввода 1..50 строк, note ≤ 500 (`handoff.go:49-55`) — магические числа
   в адаптере.
2. Валидация каждой строки через тот же `domain.NewOrderLine` против живого меню — **до** списания
   кулдаун-слота (осознанно: 422 не должен сжигать 30-секундное окно).
3. Кулдаун — общий слот с заказом: `session.AllowOrder(table.Token)` (`handoff.go:96`).
4. Привязка `customer_id`, если есть кука `aivo_customer` (+ `TouchGuest` в CRM platform).
5. Генерация кода: `domain.NewHandoffCode()` (`domain/menu/handoff.go:23`) — 6 символов из алфавита без
   0/O/1/I; `AdminStore.CreateHandoff` (`menu/adapters/postgres/handoff.go:16`) в транзакции удаляет
   предыдущий активный handoff этого стола («no stacking») и вставляет новый; коллизия кода → `ErrConflict`
   → ретрай генерации до 4 попыток в адаптере (`handoff.go:118-131`).
6. Ответ: `{code, qr_url, expires_at}`; TTL = `domain.HandoffTTL` (15 мин).

QR кода — `GET .../handoff/qr?code=X` (`handoff.go:141`): lookup + проверка `handoff.TableID == table.ID`
прямо в адаптере.

Приём официантом — `GET /api/v1/pos/handoff/{code}` (превью: имя клиента без email/телефона, сумма через
`Handoff.TotalCents()`) и `POST /api/v1/pos/handoff/{code}/accept` (`handoff.go:219`):

1. `HandoffByCode(restaurantID, code)` — активность (не использован, не истёк, свой ресторан) зашита в SQL
   (`handoff.go pg:60-72`): unknown/expired/used/foreign неразличимы, все 404.
2. **Consume-then-compensate сага в адаптере**: сначала `MarkHandoffUsed` (single-use, отсекает
   конкурентный двойной accept — UPDATE с `used_at IS NULL AND expires_at > now()`), затем
   `posapp.AddLines` в тикет POS; при ошибке — компенсация `UnmarkHandoffUsed` на
   `context.WithoutCancel` (`handoff.go:241-255`, ponytail-коммент про «одну кросс-стор транзакцию»).
3. Линковка customer к тикету + `TouchGuest` — best-effort.

Инварианты handoff и где они enforced:

| Инвариант | Где |
|---|---|
| single-use | БД: `used_at` + условный UPDATE (`handoff.go pg:74-86`) |
| TTL 15 мин | домен: константа `HandoffTTL`, `Active()` (`domain/handoff.go:12,63`); фактическая проверка — в SQL (`expires_at > now()`) |
| ≤ 1 активного на стол | адаптер: DELETE перед INSERT (`handoff.go pg:28-31`) |
| уникальность кода среди активных | БД: партиальный индекс `cart_handoffs_code_active_idx` (`0004:31`) + ретрай в HTTP-адаптере |
| строки валидны против живого меню | домен `NewOrderLine`, вызывается из HTTP-адаптера platform |

### 1.5. Landing и QR стола

- Легаси `GET /api/landing/{slug}/{table_token}` → `app.GetLandingHandler` (`app/get_landing.go:35`):
  resolveTable + `Store.LandingBlocks` (jsonb `data`, порядок по position). В актуальной v1-поверхности
  landing-блоки **не используются** — вход дайнера собирает шапку из platform-полей ресторана
  (hours/address/contacts) и темы; таблица `landing_blocks` обслуживается только легаси-эндпоинтом,
  админ-CRUD для неё нет. Фактически полумёртвая ветка модели.
- QR: легаси `GET /api/qr/...` → `app.GetQRHandler` (`app/get_qr.go:30`) — рендер ссылки
  `{base}/{slug}/t/{token}` в PNG на лету, никогда не хранится. Админ-вариант — в platform-поверхности.

### 1.6. Админ-CRUD (меню/категории/позиции/столы)

Из platform-поверхности (`menuadmin.go`, не входит в разбор) через `ports.AdminStore` → Postgres:

- Мульти-меню: `menus` с `UNIQUE (restaurant_id, slug)` и партиальным индексом «ровно один default»
  (`0003:13-18`). `UpdateMenu` (`menus.go:89`) в транзакции: снять default с себя нельзя (надо продвинуть
  другой), продвижение атомарно снимает старый default. `DeleteMenu` (`menus.go:124`) считает меню/категории
  в транзакции и вызывает доменное правило `domain.CanDeleteMenu` (`domain.go:58`): не default, не последний,
  непустой — только с force; исторические ссылки строк заказов держат RESTRICT.
- Тенант-изоляция на запись сделана красиво: `CreateCategory` пишет `menu_id` через scoped-подзапрос
  (`admin.go:111-125`), `CreateMenuItem`/`UpdateMenuItem` — так же через категорию (`admin.go:155-217`),
  0 строк → `ErrNotFound`.
- `UpdateMenuItem` заменяет аллергены/группы/опции **целиком** (wholesale) — безопасно, потому что заказы
  снапшотят.
- `DeleteMenuItem`/`DeleteCategory` → `ErrItemReferenced` при FK 23503 (история заказов) — «пометьте
  недоступным вместо удаления».
- Столы: `CreateTable`, `RegenerateTableToken` (мгновенная инвалидация ссылки).

### 1.7. Rate limit по IP

Глобальная обёртка легаси-муксa `withIPLimit` (`handlers.go:47`): fixed window 20 req/min per IP
(`ratelimit.go:90`), IP — прямой peer без X-Forwarded-For (ponytail-коммент про доверенный прокси).
На v1-поверхности этой обёртки нет (platform-мукс её не ставит) — бэкстоп из `CONTEXT.md` реально
действует только на несмонтированном легаси.

---

## 2. Сущности, связи, анемичность

### Схема (владелец таблиц — menu, миграции `backend/migrations/menu/`)

```
restaurants 1─N tables
restaurants 1─N menus (ровно 1 default; UNIQUE(rest,slug))
menus       1─N categories 1─N menu_items
menu_items  1─N option_groups 1─N options
menu_items  1─N menu_item_allergens (EU-14, PK(item,allergen))
restaurants 1─N orders 1─N order_lines 1─N order_line_options   [снапшоты]
orders      N─1 tables;  orders.customer_id → platform.customers (FK добавляет platform/0003)
restaurants 1─N service_requests (partial unique: 1 pending на (table,kind))
restaurants 1─N cart_handoffs (jsonb lines; partial unique code WHERE used_at IS NULL)
restaurants 1─N landing_blocks (jsonb data)
restaurants 1─1 notification_channels (PK = restaurant_id, шифрованный bot token + key_version)
```

Точки стыка с соседями (не углубляясь):

- **restaurants — shared таблица**: базу создаёт menu (`0001`), platform-миграция `platform/0001` добавляет
  свои колонки (org_id, address, hours, contacts) и провиженит строки (`platform/adapters/postgres/postgres.go:50`).
  В Go-типе Menu `Restaurant` — только ID/Slug/Name/CreatedAt (`domain.go:17`), platform держит свою проекцию.
- **customer_id** на orders/cart_handoffs — идентификатор из platform (диенер-аккаунты), Menu его только
  проносит; CRM platform читает menu-таблицы `orders`/`order_lines`/`tables` прямым SQL
  (`platform/adapters/postgres/customers.go:133,193,335`) — граница контекстов на уровне БД проницаема.
- **handoff принимает POS**: accept конвертирует `HandoffLine.{MenuItemID,OptionIDs,Qty}` в
  `posapp.LineInput` и идёт обычным путём добавления строк тикета; сам тикет — вне Menu (ADR 0002).
- **POS читает меню** через `menubridge` (Go-интерфейсы in-process поверх `AdminStore`).

### Насколько всё анемично

Почти полностью. Доменное поведение сегодня исчерпывается пятью функциями:

- `NewOrderLine` (`domain.go:179`) — настоящая фабрика с инвариантами; лучший кусок домена.
- `CanDeleteMenu` (`domain.go:58`) — чистое правило, честно вызывается из адаптера в транзакции.
- `ValidAllergen` (`domain.go:103`) — закрытый enum.
- `Handoff.Active`, `Handoff.TotalCents`, `NewHandoffCode` (`handoff.go`).

Всё остальное — DTO-подобные структуры с публичными полями, которые собирают и мутируют app-слой,
HTTP-адаптеры и store'ы:

- `Order` — нет конструктора; инвариант «≥1 строки» размазан по HTTP и store'ам; нет `TotalCents()`
  (а у `Handoff` есть — асимметрия; сумма заказа нужна и Telegram-нотификации, и будущему `OrderPlaced`).
- `ServiceRequest.Status` — сырая `string` с константами рядом (`domain.go:234-238`); никакого
  guard'а переходов; `SetServiceRequestStatus` примет любую строку (`admin.go:339`).
- `Menu/Category/MenuItem/Table/LandingBlock` — записи; их правила живут в SQL и адаптерах.
- `sharedkernel` (`Entity`, `AggregateRoot`, `DomainEvent`) — **мёртвый код**: ни один тип Menu (и вообще
  ни один контекст) его не встраивает, `Raise` не вызывается нигде, кроме `sharedkernel_test.go`.
- Пакет `menudb` (sqlc, `adapters/postgres/menudb/`) сгенерирован и **не используется ни одним файлом** —
  третий, мёртвый, слой доступа к тем же таблицам.

---

## 3. DDD-разбор

### 3.1. Агрегаты

#### Order — корень: Order; граница: Order + OrderLine[] + OrderLineOption[]

- **Защищаемые инварианты**: ≥ 1 строки; каждая строка валидна на момент подачи (qty ≥ 1, item доступен,
  опции принадлежат item и уважают single/multi группы — последнее сейчас не enforced, см. §1.2);
  снапшот неизменяем после создания; принадлежность одному (restaurant, table).
- **Почему граница такая**: строки — не самостоятельные сущности, а *value-снапшоты*, у них нет жизни вне
  заказа (никто не адресует order_line по ID из кода); консистентность «заказ целиком или ничего» нужна в
  одной транзакции — так и сделано (`postgres.go:295-338`). `MenuItem` в агрегат не входит и не должен:
  заказ ссылается на него по ID и копирует данные, поэтому изменение меню не требует блокировки заказов —
  классическая причина резать границу именно здесь. Payment/fulfilment-состояний нет намеренно (ADR 0002:
  Order отвязан от till/shift/GL до ратификации POS-домена) — поэтому Order в Menu write-once, без
  lifecycle, и маленькая граница оправдана вдвойне.
- **Сейчас**: корень анемичен; инварианты частично в domain-фабрике строк, частично в HTTP/store.

#### Handoff — корень: Handoff; граница: Handoff + HandoffLine[]

- **Защищаемые инварианты**: single-use (`UsedAt`); TTL (`ExpiresAt`, 15 мин); максимум один активный на
  стол; код уникален среди активных; строки валидированы против живого меню при создании.
- **Почему граница такая**: handoff — это *переносимая корзина с кодом-предъявителем*, её жизненный цикл
  (создан → предъявлен → использован/истёк) полностью самостоятелен и заканчивается ровно в момент, когда
  начинается жизнь POS-тикета. Тикет в границу не входит принципиально: accept — это **межконтекстная
  операция** (consume в Menu + AddLines в POS), одна транзакция невозможна по границе контекстов, поэтому
  и существует компенсация `UnmarkHandoffUsed`. Агрегат защищает только своё: код нельзя использовать
  дважды/после TTL/чужим рестораном. Строка хранит и снапшот (для превью официанту), и source-IDs (чтобы
  accept шёл обычным POS-путём с перевалидацией) — это осознанное дублирование, а не размытая граница.
- **Сейчас**: единственный агрегат с методами (`Active`, `TotalCents`), но весь протокол (создать-с-ретраем,
  consume/compensate) — в HTTP-адаптере platform.

#### MenuItem (каталог) — корень: MenuItem; граница: MenuItem + OptionGroup[] + Option[] + Allergen[]

Опции/группы редактируются только wholesale вместе с item (`admin.go:182-217`), извне адресуются только
для выбора (по ID внутри item) — это внутренности агрегата. `Category` и `Menu` — отдельные маленькие
агрегаты-справочники; инвариант «ровно один default-меню на ресторан» межагрегатный и правильно живёт в БД
(партиальный уникальный индекс + транзакция `UpdateMenu`). `CanDeleteMenu` — доменная политика удаления,
уже выделена.

#### ServiceRequest — корень: ServiceRequest (сам по себе)

Инвариант «≤ 1 pending на (table, kind)» — межагрегатный, его настоящий дом — БД-индекс (уже есть);
lifecycle `pending → acknowledged | dismissed` — внутриагрегатный и должен быть методом/guard'ом домена,
а не строкой в UPDATE. In-memory копия инварианта в `pkg/session` — лишний третий экземпляр (см. §1.3).

#### Restaurant, Table, NotificationChannel, LandingBlock

- `Restaurant` в Menu — по сути **reference/shared identity**, агрегатом Menu не является (владение
  жизненным циклом — у platform).
- `Table` — микро-агрегат с одним поведением (регенерация токена = инвалидация ссылки).
- `NotificationChannel` — конфигурационная запись 1:1 к ресторану; отдельного агрегата не заслуживает.
- `LandingBlock` — набор записей; закрытый каталог типов и правила repeatable/single-instance
  (`domain.go:265-276`) нигде не enforced (админ-CRUD отсутствует, так что пока это только доки).

### 3.2. Entity vs Value Object

Уже фактически VO (immutable, без identity-семантики): `OrderLine`, `OrderLineOption`, `HandoffLine`,
`Allergen` (образцовый: закрытый enum + `ValidAllergen`), `ServiceRequestKind`, `LandingBlockType`.
Entity: Restaurant, Table, Menu, Category, MenuItem, Order, ServiceRequest, Handoff, NotificationChannel.

**Деньги**. Везде голый `int` центов: `PriceCents`, `PriceDeltaCents`, `UnitPriceCents`, суммы
(`TotalCents`). Контракт (`PLATFORM.md`: «Money: integer cents everywhere») это фиксирует, и полноценный
Money-VO с валютой — YAGNI до мультивалютности. Но сейчас нет даже типовой защиты: центы, qty и position —
одинаковые `int`, их можно перепутать без единого warning'а, а `price_cents` нигде не проверяется на `>= 0`
(в БД нет CHECK, в домене нет фабрики; отрицательная цена пройдёт до заказа). Дешёвый шаг с реальной
пользой: `type Cents int` в домене + подсчёт сумм только методами (`OrderLine.TotalCents()`,
`Order.TotalCents()` — он всё равно нужен для `OrderPlaced.total_cents` и Telegram-текста) + CHECK
`price_cents >= 0` в миграции. Отдельный запах: `HandoffLine` несёт json-теги в доменном типе
(`handoff.go:37-44`), потому что адаптер маршалит доменную структуру прямо в jsonb-колонку **и** тот же
формат де-факто является wire-форматом — сериализация приклеена к домену.

### 3.3. Домен-события: сверка с docs/EVENTS.md

Сейчас **не поднимается ни одно событие**: outbox-таблица `events` создана (`platform/0004_events.up.sql`),
`sharedkernel.AggregateRoot/DomainEvent` есть, но `Raise` не вызывается ниоткуда. Каталог `docs/EVENTS.md`
(строки 21-28) для Menu:

| Событие | Оценка | Предлагаемые правки |
|---|---|---|
| `OrderPlaced` (order_id, restaurant_id, table_id, total_cents, customer_id?) | ок, поднимать в `SubmitOrderHandler` в одной транзакции с `CreateOrder` | для total_cents нужен `Order.TotalCents()` — сейчас его нет нигде |
| `ServiceRequested` (request_id, table_id, kind) | ок | добавить `restaurant_id` в payload — конвенция самого файла требует flat-payload с ID, а колонка `events.restaurant_id` не отменяет самодостаточность payload'а; сейчас Menu — единственный контекст, где его нет в payload |
| `HandoffCreated` (handoff_id, table_id, code, **total_cents**) | поднимать при `CreateHandoff` | **убрать `code` из payload**: код — предъявительский секрет (единственная «аутентификация» handoff'а), а outbox читается доставщиком/консюмерами и живёт дольше 15-минутного TTL; консюмеру код не нужен — есть handoff_id |
| `HandoffAccepted` (handoff_id, ticket_id, accepted_by) | ок; поднимать в accept-флоу после успешного `AddLines` | payload содержит POS-данные (ticket_id) — нормально для интеграционного события, но фиксирует, что raise-точка живёт в оркестраторе accept'а, не внутри агрегата Handoff |
| — | **дырка в каталоге**: закрытие сервис-запроса | добавить `ServiceRequestClosed` (request_id, table_id, kind, outcome: acknowledged\|dismissed) — это ровно тот сигнал, по которому нужно чистить дедуп-состояние (сейчас его никто не чистит, §1.3) и который нужен аналитике времени реакции |
| — | кандидат: `TableTokenRegenerated` (table_id) | security-audit событие; дёшево, но можно отложить (YAGNI, отметить в каталоге как «не поднимаем осознанно») |
| — | кандидат: `MenuItemAvailabilityChanged` (86'd) | понадобится живому POS-меню/аналитике; сейчас не поднимать, но занести в каталог как future |

`HandoffExpired` заводить не стоит: истечение пассивное (нет джоба), событие потребовало бы шедулер ради
факта, который консюмер может вычислить из `HandoffCreated.occurred_at + TTL`.

### 3.4. Что должно жить где (domain / app / adapters) и конкретные нарушения

Целевое распределение: **domain** — инварианты, фабрики, переходы состояний, политика (`CanDeleteMenu`),
расчёт денег; **app** — оркестрация use case'ов (резолв, транзакция+outbox, нотификация), маппинг доменных
ошибок; **adapters** — парсинг/сериализация, SQL, Telegram, куки.

Нарушения сейчас (file:line):

1. **`pkg/session` — «pkg», вросший в домен.** `pkg/session/ratelimit.go:7` импортирует
   `aivo/internal/domain/menu`; `AllowServiceRequest`/`MarkAcknowledged` (`ratelimit.go:66,81`) — это
   доменный инвариант дедупа сервис-запросов, продублированный в process-local map поверх уже существующего
   БД-индекса, с расхождениями при рестарте и при ack (см. §1.3). Известный запах подтверждён; сам пакет
   честно признаётся в этом в шапке `cookie.go:1-7`.
2. **Порт с ложным контрактом.** `ports/store.go:61` («Callers MUST check HasOpenServiceRequest first»)
   никем не выполняется — `app/submit_service_request.go:49` использует `session.AllowServiceRequest`;
   `HasOpenServiceRequest` — мёртвый метод порта в проде.
3. **Use case в чужом адаптере.** Весь handoff-флоу — валидация, лимиты 1..50/500, кулдаун, ретрай кода,
   линковка клиента, сага consume/compensate — в `platform/adapters/http/handoff.go:28-138` и `:219-269`
   вместо `menu/app` (командам `CreateHandoff`/`AcceptHandoff` там самое место; ownership тоже странный:
   логика Menu-агрегата лежит в HTTP-слое platform).
4. **Вход дайнера мимо app-слоя.** `dinerEntry`/`dinerBrowseMenu` (`diner.go:118,231`) дергают
   `Store`/`AdminStore` напрямую из адаптера — чтение можно счесть допустимым CQRS-шорткатом, но комментарий
   `menu/adapters/http/handlers.go:33` («this adapter never talks to ports.Store directly») декларирует
   правило, которое действующая поверхность нарушает.
5. **Семантика SessionID сломана молча.** `diner.go` передаёт `SessionID: table.Token` в команду, чьё поле
   документировано как «диенер-сессия» (`app/submit_order.go:31-39`); кулдаун превратился из per-session в
   per-table, `CONTEXT.md` («Diner session») не обновлён. Либо честно переименовать в `CooldownKey`, либо
   вернуть per-session.
6. **Инвариант Order без дома.** Нет `domain.NewOrder`; «≥1 строки» проверяют HTTP (`handlers.go:287`),
   store Postgres (`postgres.go:291`) и store memory (`memory.go:182`) — три сторожа, ни один не доменный.
7. **Идентичность назначает адаптер.** `postgres.go:301-302` (`order.ID = uuid.New()`),
   `postgres.go:343-345` (то же + Status для ServiceRequest) — генерация ID/CreatedAt/начального статуса в
   store вместо доменной фабрики; memory-адаптер дублирует (`memory.go:185-186,195-197`).
8. **Машина состояний ServiceRequest отсутствует.** Статус — строка (`domain.go:248`), переход — любой
   UPDATE (`admin.go:339-350`); dismissed вообще не упомянут в типе поля (комментарий говорит только про
   pending/acknowledged).
9. **События не поднимаются, sharedkernel мёртв.** `sharedkernel/aggregate.go:6` — ноль встраиваний;
   outbox `events` пуст by design, хотя каталог объявлен контрактом «to implement against».
10. **ADR 0003 нарушен буквой.** ADR: plaintext-токен «never held as a Go string (only []byte, zeroed after
    use)»; `app/notify.go:53` делает `string(plaintext)` и передаёт строку через порт `Notifier`
    (`ports/notifier.go:17`); telegram-адаптер конкатенирует его в URL (`telegram.go:86`). Зануления нет.
11. **Дубликат/мультивыбор опций не проверяется** — `domain.go:187-199` (см. §1.2): нарушение
    `OptionGroup.Multi`-семантики, двойной счёт цены за дубли option_id.
12. **Легаси-поверхность и мёртвый sqlc-слой.** `menu/adapters/http` не смонтирован (расходится поведением с
    v1: кулдаун по куке, IP-limit есть только тут); `adapters/postgres/menudb/` не используется никем.
    Оба — чистый носитель энтропии.
13. **CONTEXT.md разошёлся с реальностью.** «Cart … never persisted server-side» — `cart_handoffs`
    ровно это и делает; «Order submits are debounced per session» — теперь per table; Landing page описана
    как действующая фича, а живая поверхность её не рендерит.
14. **Кросс-контекстные SQL-чтения.** `platform/adapters/postgres/customers.go:133,193,335` читают
    menu-таблицы напрямую (CRM-агрегации). Для модульного монолита терпимо, но это скрытая связность,
    которую стоит хотя бы задокументировать как осознанную (сейчас нигде не отмечено).

---

## 4. Рекомендации по рефакторингу (по приоритету, маленькими шагами)

Принцип: сначала убрать реальные баги и двойные источники правды, потом стянуть логику в домен, события —
только когда появится первый консюмер. Каждый шаг самостоятелен и мал.

### P0 — баги и двойная правда (низкий риск, высокая отдача)

1. **Дедуп сервис-запросов: оставить одну правду — БД.** Убрать `AllowServiceRequest`/`MarkAcknowledged`
   из `pkg/session`; в `SubmitServiceRequestHandler` ловить unique violation от
   `service_requests_open_per_table_kind_idx` (или звать уже существующий `HasOpenServiceRequest`) и
   маппить в `ErrServiceRequestAlreadyOpen`. Это одним махом чинит 500 после рестарта, «стол заблокирован
   после ack» и убирает импорт домена из pkg. In-memory 2-минутный TTL больше не нужен: pending-строка
   снимается ack/dismiss'ом из POS.
2. **Развязать `pkg/session` от домена.** После шага 1 в пакете остаются кука, `AllowOrder`, `AllowIP` —
   generic-строковые ключи, импорт `internal/domain/menu` удаляется. Либо переместить остаток в
   `internal/menu/...` (это Menu-plumbing, как сам пакет и признаёт), либо оставить в pkg уже без
   доменных типов.
3. **Опции: enforce single/multi и запрет дублей в `NewOrderLine`.** Валидировать выбор по группам:
   дубликат option_id — ошибка; >1 опции в группе с `Multi=false` — ошибка. Одна функция, один тест;
   автоматически чинит и заказ, и handoff (оба идут через `NewOrderLine`). Заодно — верхняя граница qty
   (например ≤ 99) рядом с `ErrInvalidQty`.
4. **Удалить мёртвое:** `adapters/postgres/menudb/` + `queries/menu/` (sqlc для Menu никем не используется),
   и решить судьбу легаси `menu/adapters/http`: либо смонтировать, либо (лучше) удалить вместе с тестами,
   перенеся ценные тест-сценарии на v1-поверхность. Ремонтировать две расходящиеся поверхности дороже,
   чем убить одну.

### P1 — стянуть инварианты в домен (маленькие чистые шаги)

5. **`domain.NewOrder(restaurant, table, customerID, lines, comment)`** — фабрика с «≥1 строки», генерацией
   ID/CreatedAt; store перестаёт назначать identity, тройная проверка пустых строк схлопывается в одну.
   Симметрично — минимальная фабрика для `ServiceRequest` (ID, pending, CreatedAt).
6. **`Order.TotalCents()` (+ `OrderLine.TotalCents()`).** Нужен Telegram-тексту (сейчас сумма вообще не
   показывается официанту) и обязателен для `OrderPlaced.total_cents`. Симметрия с `Handoff.TotalCents`.
7. **`type Cents int` + CHECK `price_cents >= 0`** (миграция) + валидация цены на границе админ-CRUD.
   Money-VO с валютой не делать (YAGNI до мультивалютности).
8. **Статусы ServiceRequest: typed + guard.** `type ServiceRequestStatus string`, метод
   `(*ServiceRequest).Close(outcome)` с проверкой `pending → acknowledged|dismissed`; `SetServiceRequestStatus`
   принимает типизированный статус. Три строки домена, убирает «любой UPDATE любой строкой».
9. **Переименовать `SubmitOrder.SessionID` → `CooldownKey`** (или вернуть per-session семантику —
   решение продукта), и синхронизировать `CONTEXT.md`: cooldown per table, cart handoff = персистентная
   корзина, статус Landing page. Дока-долг дешевле всего платить сразу.

### P2 — переезд use case'ов на свои места

10. **Команды `CreateHandoff` / `AcceptHandoff` в `menu/app`.** Перенести из
    `platform/adapters/http/handoff.go` валидацию, кулдаун, генерацию кода с ретраем (create) и сагу
    consume → AddLines → compensate (accept; POS-вызов — через маленький порт-колбек). Адаптер platform
    остаётся тонким маршрутизатором. Лимиты 1..50/≤500 — константы рядом с `HandoffTTL` в домене.
11. **Ponytail-долг саги**: попробовать одну транзакцию на shared `*sql.DB` (оба store уже сидят на одном
    пуле — `NewPostgresStoreFromDB`) вместо compensate; если получается дёшево — компенсация и
    `UnmarkHandoffUsed` удаляются.
12. **`notify.go` ближе к ADR 0003**: сменить сигнатуру `Notifier` на `[]byte`-токен, занулять после
    отправки; или перенести дешифровку внутрь telegram-адаптера (тогда app вообще не видит plaintext).

### P3 — события (только вместе с первым консюмером/паблишером)

13. **Outbox-запись в транзакциях команд**: `OrderPlaced` в `CreateOrder`-транзакции, `ServiceRequested`,
    `HandoffCreated` (без `code` в payload), `HandoffAccepted`, новый `ServiceRequestClosed`. Использовать
    существующий `sharedkernel` (`Raise` в фабриках, app пишет `Events()` в `events` перед commit) — либо,
    если embed-модель не приживается, честно удалить `AggregateRoot` и писать outbox-строки прямо в app.
    Обновить `docs/EVENTS.md`: `restaurant_id` в payload `ServiceRequested`, убрать `code`, добавить
    `ServiceRequestClosed`, отметить `TableTokenRegenerated`/`MenuItemAvailabilityChanged` как осознанно
    отложенные.
14. **Решить судьбу `landing_blocks`**: либо админ-CRUD + рендер в v1-входе, либо удалить таблицу/тип и
    оставить platform-поля ресторана как источник landing-контента (сейчас — вторая реальность де-факто).

Чего **не** делать: generic-репозитории, event bus поверх пустого outbox'а, Money с валютами, выделение
Menu в отдельный сервис/gRPC (ADR и PLATFORM.md прямо говорят «не раньше второго процесса»), репозиторный
слой поверх и так тонких store'ов.
