# Исследование bounded context PLATFORM

Дата: 2026-08-24. Область: `backend/internal/platform/{app,ports,adapters}`,
`backend/internal/domain/platform/`, `backend/internal/sharedkernel/`,
`backend/migrations/platform/`, контракт `docs/PLATFORM.md`, каталог событий
`docs/EVENTS.md`. Соседние контексты menu/pos не анализировались — только точки
связи (раздел 3.6).

---

## 1. Логика as-is

Слои: HTTP-адаптер (`adapters/http`) → приложение (`app.App`, один struct
методов-юзкейсов) → доменные типы (`internal/domain/platform`) → порт
`ports.Store` → Postgres-адаптер (`adapters/postgres`). Плюс порты
`BillingProvider` (fake), `ThemeGenerator` и `Assistant` (оба — `claude` CLI),
`ImageStore` (S3/MinIO).

### 1.1 Регистрация организации

`POST /api/v1/auth/register` (`http/auth.go:144`) → `App.Register`
(`app/app.go:134`) → `Store.CreateOrgWithOwner` (`postgres/postgres.go:38`).

- App валидирует: org_name/restaurant_name непустые, email через
  `net/mail.ParseAddress` (≤254), пароль ≥8 символов; слаг ресторана
  выводится `Slugify(restaurantName)` и проверяется `domain.ValidSlug`.
- Атомарно в одной транзакции создаются: организация, ресторан, **дефолтное
  меню чужого контекста** (`insertDefaultMenu`, `postgres.go:283` — прямой
  INSERT в `menus`), владелец (`role=owner`, `restaurant_id=NULL`), подписка
  `free/active`.
- При конфликте слага app повторяет один раз с случайным суффиксом
  (`app.go:166-169`) — детектирует «slug» по `strings.Contains(err.Error(), "slug")`,
  т.е. по тексту ошибки (хрупко).
- Затем `startSession`: токен 32 байта, в БД хранится SHA-256
  (`hashToken`), TTL 30 дней; куки `aivo_session` ставит хендлер.

### 1.2 Аутентификация и сессии

- `Login` (`app.go:204`): нижний регистр email, при неизвестном email
  сжигается bcrypt на `dummyBcryptHash` (защита от тайминг-атаки), плохой
  email и плохой пароль сливаются в один `ErrUnauthorized`.
- `UserByToken` (`app.go:229`) → `Store.SessionUser`: **истечение сессии
  проверяется в SQL** (`WHERE expires_at > now()`, `postgres.go:182`), не в
  домене.
- Middleware в адаптере: `auth` (кука→юзер), `manage` (роль
  owner/manager, `http/auth.go:32`), `restaurant(needManage, ...)`
  (`http/auth.go:46`) — тенант-скоуп: ресторан ищется
  `Store.Restaurant(orgID, id)` (чужой id → 404), затем
  `User.CanAccessRestaurant` (staff чужого ресторана → 403).
- Инвариант «роль ∈ {owner,manager,waiter}» — только валидация на входе
  (`AddStaff`), в типе `Role` он не защищён.

### 1.3 Подписки (state machine)

`POST /api/v1/org/subscription` → `App.ChangePlan` (`app.go:266`).

- Домен: `Subscription.Transition` (`domain/platform/domain.go:141`) —
  таблица переходов `trialing→{active,past_due,canceled}`,
  `active→{past_due,canceled}`, `past_due→{active,canceled}`, `canceled`
  терминальна.
- **Но** `ChangePlan` машиной пользуется лишь частично: при уходе на free и
  при подписке на платный план он **конструирует новый
  `Subscription{Status: ...}` с нуля** (`app.go:282`, `app.go:290`), не
  делая `Transition` от текущего статуса. Следствие: `canceled` фактически
  не терминален — смена плана «воскрешает» подписку в обход машины.
  Transition применяется только к прыжку `trialing→<статус от провайдера>`.
- Лимиты плана: `Plan.MaxRestaurants()` / `Plan.MaxMenuItems()` — данные в
  домене, enforcement размазан: лимит ресторанов — в app
  (`CreateRestaurant`, `app.go:333-341`), лимит блюд — **в HTTP-хендлере
  чужого (menu) ресурса** (`http/menuadmin.go:459-473` через
  `App.ItemLimitFor`).
- Биллинг: `BillingProvider` (fake, `adapters/billing/fake.go`) — всегда
  одобряет, возвращает `SubActive`.
- `renews_at` в ответе — фикция адаптера: `updated_at + 30 дней`
  (`http/org.go:32`).

### 1.4 Провижининг и настройки ресторана

- `App.CreateRestaurant` (`app.go:317`): имя/слаг валидируются, лимит плана
  проверяется **неатомарно** (count → insert; гонка двух запросов может
  превысить лимит; принятый компромисс).
- `App.UpdateRestaurant` (`app.go:365`): patch-семантика (nil = не менять),
  слаг через `ValidSlug`, hours ≤20 строк, phone/instagram кладутся в map
  `Contacts`. Кастомный домен — часть того же patch: `validHostname`
  (regex, ≤253) и `Store.SetCustomDomain`.
- `SetCustomDomain` (`postgres.go:425`): DELETE старой записи + INSERT
  новой **без транзакции**; `verified_at = now()` сразу (верификация DNS —
  документированная заглушка v1). Уникальность домена — PK таблицы
  (`ErrConflict` 409).
- Роутинг по домену: `customDomainMiddleware` в
  `cmd/aivo-server/main.go:195` — `RestaurantIDByDomain` (только
  `verified_at IS NOT NULL`) → 302 на канонический `/{slug}`, кэш 60 сек.

### 1.5 Темы (design.md + AI-генерация)

- `Theme` = jsonb `theme` + текст `design_md` на ресторан; строка создаётся
  лениво — отсутствие строки читается как пустая тема (`postgres.go:385-389`).
- **Единая точка записи** `App.SaveTheme` (`app.go:447`): размерные лимиты
  (64KB/256KB), accent из enum, banner_url http(s) ≤2048, ≤40 css_vars,
  каждая через `domain.ValidCSSVar` — CSS-инъекционный guard (имя
  `--lowercase`, запрет `url(`, `;`, `{`, `}`, `\`). Через неё идут все три
  писателя: админский PUT, применение AI-предложения, действие
  `update_theme` ассистента.
- Генерация: `POST .../theme/generate` → `App.GenerateTheme` (`app.go:63`)
  — требует непустой `design_md` (409 `no_design_md`), зовёт
  `ThemeGenerator` (adapters/claudecli), **никогда не сохраняет** —
  применение остаётся явным PUT (правило AGENTS.md «AI не управляет
  молча»); предложение логируется. Адаптер `claudecli.parseAndValidate`
  (`claudecli/claudecli.go:122`) строго валидирует вывод модели и **всегда
  сохраняет текущий banner_url** (модельному не доверяет).

### 1.6 Персонал

`App.AddStaff` (`app.go:496`): только manager/waiter (owner — только через
Register), email-валидация, пустой пароль = «invited» со случайным паролем
(инвайт-флоу не существует). Статус «invited» **не хранится** — адаптер
выводит его из того, был ли пароль в запросе (`http/org.go:321-324`), при
листинге все всегда «active» (`org.go:303`).

### 1.7 AI-ассистент админа

Маршруты `.../assistant/*` (manager+). Практически вся логика — в
HTTP-адаптере `http/assistant.go`:

- **Send** (`assistant.go:70`): multipart text+files (≤8 файлов; картинки —
  в S3, текст ≤64KB инлайнится в промпт; тип контента сниффается, декларации
  не доверяют). Хендлер сам собирает промпт со снапшотом
  меню/темы/ресторана и историей (`assistantPrompt`, `assistant.go:205`),
  зовёт `ports.Assistant.Chat`, валидирует действия.
- Валидация действий двухступенчатая, в домене:
  `ValidateAction` (allowlist типов + форма полей,
  `domain/platform/assistant.go:123`) — выполняется в CLI-адаптере
  (`claudecli/assistant.go:69`: одно невалидное действие ⇒ весь список
  отбрасывается, reply остаётся); `ValidateActionRefs`
  (`assistant.go:219`) — тенант-скоуп id и префикс image_url — в хендлере
  (`http/assistant.go:180-186`), та же семантика «всё или ничего».
- **Apply** (`http/assistant.go:310`): decide-once — сообщение должно быть
  ассистентским, с действиями и `action_status IS NULL`
  (`assistantDecidableMessage`); повторное решение → 409. Индексы
  валидируются до исполнения. Исполнение последовательное,
  stop-on-first-failure, **не в транзакции** (ponytail-компромисс);
  `applied` ставится только если хоть что-то удалось, иначе сообщение
  остаётся pending. Сам маппинг действий на команды menu-контекста —
  `applyAction` (`http/assistant.go:441-564`), включая ре-валидацию,
  вычисление position нового меню и сохранение темы.
- Decide-once дополнительно защищён на уровне SQL:
  `UPDATE ... WHERE action_status IS NULL` (`postgres/assistant.go:119-136`).
- Тред — один на ресторан, лениво (upsert по unique index,
  `postgres/assistant.go:16`).
- Аудит — только `slog.Info` («assistant proposal», «actions applied») —
  не в outbox.

### 1.8 Customers и guest CRM

- Аккаунты динеров платформенно-глобальные, полностью отдельные от staff:
  свои таблицы (`customers`, `customer_sessions`), своя кука
  `aivo_customer`, TTL 90 дней. `RegisterCustomer/LoginCustomer`
  (`app/customers.go`) — зеркало staff-аутентификации (bcrypt, dummy-hash,
  SHA-256 токена).
- `GET /customer/me` — своя кросс-ресторанная история заказов
  (`Store.CustomerOrders` читает `orders` menu-контекста напрямую,
  `postgres/customers.go:127`).
- **Приватность CRM = существование строки `guest_profiles`**: ресторан
  видит только клиентов, у которых есть строка (restaurant_id,
  customer_id), и только свои заказы. Строка создаётся лениво:
  `TouchGuestProfile` (upsert, `postgres/customers.go:173`) вызывается из
  адаптера при заказе залогиненного динера (`http/diner.go:339`), при
  создании handoff (`http/handoff.go:107`) и при приёме handoff
  (`http/handoff.go:262`).
- `Guests/GuestDetail/UpdateGuest`: visits/spend считаются одним
  агрегатным SQL по **чужим таблицам** `orders`+`order_lines` UNION
  `tickets`+`ticket_lines` (`guestTotalsSQL`, `postgres/customers.go:188`).
  Notes ≤10000, ≤20 тегов ≤40 символов — валидация в app
  (`customers.go:136-153`). Официанты видят имя клиента только в превью
  handoff (`http/handoff.go:182-187` — «name only, never email/phone»).

### 1.9 Events outbox

Таблица `events` (`migrations/platform/0004_events.up.sql`): outbox общий
для всех контекстов, `published_at IS NULL` = pending, частичный индекс.
sqlc-запросы сгенерированы (`platformdb/platform.sql.go`:
InsertEvent/PendingEvents/MarkEventPublished), sharedkernel даёт
`DomainEvent`/`AggregateRoot.Raise/Events`. **Ничего не подключено: ни один
код не пишет в outbox, ни один агрегат не Raise-ит, паблишера нет.**
`docs/EVENTS.md` — контракт «to implement against».

### 1.10 Сводка инвариантов и где они enforced

| Инвариант | Где сейчас |
|---|---|
| email уникален (staff / customers) | UNIQUE в БД → `ErrConflict` |
| слаг ресторана валиден и уникален | `domain.ValidSlug` (app) + UNIQUE в БД |
| роль ∈ enum; owner org-wide, staff — per-restaurant | валидация в `AddStaff` (app); скоуп — `User.CanAccessRestaurant` (домен) + middleware (адаптер) |
| одна подписка на org | PK `subscriptions(org_id)` |
| переходы статусов подписки | `Subscription.Transition` (домен), **частично обходится в `ChangePlan`** |
| лимит ресторанов плана | app (`CreateRestaurant`), неатомарно |
| лимит блюд плана | HTTP-хендлер `menuadmin.go:459` |
| тема: accent enum, css-guard, размеры | app `SaveTheme` (единая точка) + `domain.ValidCSSVar` |
| домен уникален, только verified роутится | PK БД + SQL-фильтр `verified_at IS NOT NULL` |
| сессия не истекла | SQL `expires_at > now()` |
| действия ассистента: allowlist/форма | домен `ValidateAction` (вызывает CLI-адаптер) |
| действия ассистента: тенант-скоуп id | домен `ValidateActionRefs` (вызывает HTTP-адаптер) |
| decide-once для действий | HTTP-адаптер + SQL `WHERE action_status IS NULL` |
| приватность CRM (строка = видимость) | схема БД + SQL-джойны |
| один тред ассистента на ресторан | UNIQUE index + upsert |

---

## 2. Сущности и связи; что anemic

```
Organization 1—1 Subscription
Organization 1—N User (owner: restaurant_id NULL; manager/waiter: scoped)
User 1—N Session
Organization 1—N Restaurant
Restaurant 1—0..1 Theme (лениво)
Restaurant 1—0..1 CustomDomain (уникален глобально)
Restaurant 1—1 AssistantThread 1—N AssistantMessage
Customer 1—N CustomerSession
Restaurant N—N Customer через GuestProfile (композитный PK)
Customer 1—N orders/tickets (чужие контексты, customer_id)
```

Оценка поведения:

- **Organization** — полностью anemic (id, name, created_at; ни одного метода).
- **User** — почти anemic; есть `CanAccessRestaurant`, `CanManage`. Правило
  «валидная роль» и «owner не может быть restaurant-scoped» не в типе.
- **Session** — anemic DTO; ни `IsExpired`, ни фабрики (TTL/хеширование — в
  app, истечение — в SQL).
- **Subscription** — единственный тип с настоящим поведением
  (`Transition`), но оно обходится (см. 1.3).
- **Plan** — value object с поведением (`MaxRestaurants`, `MaxMenuItems`) —
  хороший образец.
- **Restaurant** — anemic; вся patch-логика в app, `ValidSlug` — свободная
  функция рядом.
- **Theme** — anemic (json.RawMessage + строка); вся валидация в
  `App.SaveTheme` и продублирована в адаптерах.
- **CustomDomain** — anemic, причём тип вообще не используется кодом
  (store оперирует строкой host).
- **AssistantAction / ThemePayload** — value objects с богатой валидацией
  (`ValidateAction`, `ValidateActionRefs`, `ValidCSSVar`) — лучшая
  доменная часть контекста, но это свободные функции над плоским структом,
  а не конструктор, гарантирующий валидность.
- **AssistantMessage** — anemic; правила «решить можно один раз», «решаемы
  только ассистентские сообщения с действиями» живут в хендлере и SQL.
- **Customer / GuestProfile / GuestSummary / CustomerOrder** — anemic;
  Guest* — фактически read-модели (visits/spend вычисляются SQL).
- **Sharedkernel** (`Entity`, `AggregateRoot`, `DomainEvent`) — заготовки
  есть, в platform-домене **не используются нигде**.

---

## 3. DDD-разбор

### 3.1 Предлагаемые агрегаты

**Organization** (корень: Organization).
- Граница: только сама организация (name). Subscription и User —
  отдельно.
- Инвариант: валидное имя. Границу шире делать незачем: подписка меняется
  биллингом независимо от переименования org, а пользователей может быть
  много — грузить их всех ради rename бессмысленно.
- События: `OrganizationRegistered` (фактически поднимается сценарием
  регистрации, см. 3.4).

**Subscription** (корень: Subscription, identity = OrgID).
- Граница: план + статус.
- Инвариант: state machine переходов и правило «canceled терминален»;
  правило «free всегда active». Именно потому отдельный агрегат: жизненный
  цикл управляется биллинг-событиями, конкурентно с любыми действиями
  org/ресторанов; замыкание его в Organization создало бы ложную
  конкуренцию за одну версию агрегата.
- Требуемая доработка: `ChangePlan(plan, providerStatus)` как метод
  агрегата, внутри — только через `Transition`; сейчас app собирает
  структуры руками (`app/app.go:282,290`).
- События: `SubscriptionChanged` (payload old→new — как в EVENTS.md).
- Кросс-агрегатное правило «план ограничивает число ресторанов/блюд» —
  принципиально не инвариант одного агрегата; оно остаётся политикой
  app-слоя (как сейчас), но обе проверки должны жить в app, а не одна в
  app, другая в хендлере.

**User** (корень: User).
- Граница: учётка + роль + скоуп.
- Инварианты: валидный email, роль из enum, «owner ⇒ RestaurantID == nil,
  manager/waiter ⇒ RestaurantID != nil». Сейчас последнее нигде не
  выражено типом — только конвенцией `Register`/`AddStaff`.
- Session в агрегат не включать: сессии создаются/умирают массово и
  независимо; это отдельная маленькая сущность (или вообще технический
  реестр в app/adapters — допустимо оставить как есть, добавив
  `Session.IsExpired(now)` для симметрии с SQL).
- События: предлагается добавить `StaffUserAdded` (см. 3.4).

**Restaurant** (корень: Restaurant).
- Граница: ресторан + его настройки (address, hours, contacts) + claim
  кастомного домена как ссылка/VO.
- Инварианты: валидный slug, ≤20 строк hours, валидный hostname домена.
  Глобальная уникальность slug и domain — инвариант БД (правильно, это
  межагрегатная уникальность).
- Theme в агрегат **не** включать: тема меняется AI/админом часто и
  независимо от реквизитов; 1:1-таблица с ленивой строкой — фактически уже
  отдельный агрегат.
- События: `RestaurantProvisioned`; предлагается `CustomDomainClaimed` /
  `CustomDomainReleased` (см. 3.4).

**RestaurantTheme** (корень: Theme, identity = RestaurantID).
- Граница: theme JSON + design_md.
- Инвариант: валидность структуры темы (accent enum, css-guard, лимиты
  размеров) — сейчас это `App.SaveTheme`; по-хорошему это конструктор/метод
  `Theme.Apply(payload)` в домене, app остаётся оркестрация
  load→mutate→save.
- События: `ThemeApplied{source}`.

**AssistantThread** (корень: Thread; AssistantMessage — вложенные сущности).
- Граница: тред + его сообщения. Инвариант, ради которого граница именно
  такая: **decide-once** — решение (applied/discarded) по сообщению
  принимается один раз, и решаемы только ассистентские сообщения с
  непустыми действиями. Это правило связывает сообщение с его статусом и
  сейчас размазано между хендлером (`http/assistant.go:417-436`) и SQL
  (`postgres/assistant.go:119`). Метод
  `Thread.DecideMessage(id, decision)` выразил бы его в одном месте;
  оптимистическая защита от гонки остаётся условием в SQL (это нормально —
  БД как последний рубеж).
- Сообщения грузить постранично — агрегат «тред целиком» в память не
  тянуть; практично держать корнем тред, но работать через
  `Thread.AppendMessage` / `DecideMessage` с загрузкой одного сообщения.
- События: предлагается `AssistantActionsProposed`,
  `AssistantActionsApplied`, `AssistantActionsDiscarded` (см. 3.4) —
  сейчас этот аудит существует только как slog.

**Customer** (корень: Customer).
- Граница: аккаунт динера. Инварианты: email валиден/уникален, name ≤100.
- CustomerSession — как staff Session, вне агрегата.
- События: `CustomerRegistered`.

**GuestProfile** (корень: GuestProfile, identity = (RestaurantID, CustomerID)).
- Граница: notes + tags + first/last_seen одной пары
  ресторан-клиент. Инвариант: лимиты notes/tags и само правило приватности
  «нет строки — нет видимости» (последнее — структурный инвариант схемы,
  агрегат его лишь документирует).
- Отдельный агрегат, а не часть Customer: ресторан правит свой профиль
  гостя, не трогая аккаунт клиента; и не часть Restaurant — профилей
  много.
- `GuestSummary`, `GuestOrder`, `CustomerOrder` — не сущности, а
  **read-модели** (проекции по чужим таблицам); их правильно так и
  оставить query-стороной.

### 3.2 Entity vs Value Object

Value objects (сейчас — примитивы или анонимные структуры):
- `Role`, `Plan`, `SubscriptionStatus` — уже defined types с
  enum-семантикой; недостаёт запрета конструирования невалидных значений
  на границе (ValidRole/ValidPlan вызываются точечно).
- `Slug` — сейчас `string` + свободная `ValidSlug`; кандидат на VO
  (`NewSlug`), т.к. используется в трёх местах (регистрация, ресторан,
  меню ассистента) с дублированием проверки.
- `Email` — валидация только в app (`validEmail`), тип отсутствует.
- `Hostname` (custom domain) — regex в app (`app.go:418`).
- `HoursRow`, `Contacts` (map) — VO без валидации содержимого (label/open
  /close не проверяются вовсе — принятая дырка).
- `ThemePayload`, `AssistantAction`, `Attachment` — VO с валидацией
  свободными функциями.
- `TokenHash` — []byte, семантика размазана (hashToken в app).
- Money — int cents по всему контракту (осознанно, ок).

Entities: Organization, User, Session, Restaurant, Theme,
AssistantThread/Message, Customer, GuestProfile, Subscription.

### 3.3 Что должно жить в domain / app / adapters

- **domain**: state machine подписки (полностью, включая политику
  free/paid); валидации Slug/Email/Hostname/Theme/Action как
  конструкторы VO; правила ролей и скоупа User; decide-once треда;
  лимиты плана (данные — уже там).
- **app**: оркестрация юзкейсов (load → метод агрегата → save → события в
  outbox), кросс-агрегатные политики (лимиты плана при создании
  ресторана/блюда), интеграция с портами (billing, generator, assistant,
  images), сессии/токены/bcrypt (техническая аутентификация — уместно в
  app), сборка промпта ассистента и применение его действий.
- **adapters/http**: только парсинг/куки/коды ответов/view-модели;
  adapters/postgres: только SQL и маппинг + constraint'ы БД как последний
  рубеж уникальности/гонок.

### 3.4 Домен-события: сверка с docs/EVENTS.md и правки

Заявлено в EVENTS.md (platform): `OrganizationRegistered`,
`SubscriptionChanged`, `RestaurantProvisioned`, `ThemeApplied`,
`CustomerRegistered`. Всё соответствует найденным сценариям; ничего из
этого пока не поднимается (outbox мёртв).

Предлагаемые правки каталога:

1. **`ThemeApplied.source`** — enum расширить: сейчас `manual | ai_proposal`,
   но писателей темы три: PUT админа, применение AI-генерации (тоже PUT —
   сервер их не различает!) и действие ассистента `update_theme`. Либо
   добавить `assistant`, либо честно оставить два значения и признать, что
   manual/ai_proposal сервер различить не может, пока apply идёт тем же
   PUT (стоит добавить признак в запрос или отдельный endpoint apply).
2. **Добавить `StaffUserAdded`** (aggregate: user; payload: user_id, org_id,
   restaurant_id, role) — создание staff-аккаунта — значимое событие
   безопасности; сейчас нет даже лога.
3. **Добавить `CustomDomainClaimed` / `CustomDomainReleased`** (aggregate:
   restaurant; payload: restaurant_id, domain) — влияет на роутинг и
   будущую cert-автоматизацию.
4. **Добавить `AssistantActionsApplied` / `AssistantActionsDiscarded`**
   (aggregate: assistant_thread; payload: message_id, restaurant_id,
   selected indexes, succeeded count) — правило AGENTS.md «логировать
   AI-рекомендации» сейчас исполнено slog'ом; outbox дал бы устойчивый
   аудит. Опционально `AssistantActionsProposed`.
5. **`GuestProfile`**: событие первого визита
   (`GuestFirstSeen`/`GuestProfileCreated`) полезно для будущих
   CRM-автоматизаций; «touch» на каждый заказ в outbox не писать (шум,
   к тому же `OrderPlaced`/`TicketClosed` из menu/pos уже несут
   customer_id — консьюмер CRM может строиться на них).
6. Конвенция «restaurant_id NULL только для org-level» — под ней надо
   явно перечислить и `CustomerRegistered` (customer платформенно-глобален,
   restaurant_id тоже NULL) — сейчас упомянуты только
   OrganizationRegistered/SubscriptionChanged.

### 3.5 Конкретные нарушения сейчас (бизнес-логика не на своём месте)

Адаптеры (HTTP) содержат use-case-логику:

- `http/assistant.go:441-564` — `applyAction`: полный маппинг действий
  ассистента на команды menu-контекста, включая вычисление position меню
  (531-536, продублировано из `menuadmin.go:68-77`) и слияние темы. Это
  application service, живущий в хендлере.
- `http/assistant.go:310-385` — батч-семантика apply
  (stop-on-first-failure, refresh refs после create-действий, правило
  «applied только если succeeded>0») — политика юзкейса в адаптере.
- `http/assistant.go:205-307` — сборка промпта и снапшота состояния — тоже
  app-обязанность (порт `Assistant.Chat(prompt)` слишком низкоуровневый:
  адаптер вынужден знать, как готовить промпт).
- `http/handoff.go:241-265` — сага accept: consume → компенсация
  unmark при ошибке → link ticket customer → touch guest. Оркестрация
  процесса в хендлере.
- `http/menuadmin.go:459-473` — enforcement лимита блюд плана в хендлере
  (парный лимит ресторанов — в app: несимметрично).
- `http/diner.go:338-342`, `http/handoff.go:107` — политика «залогиненный
  динер ⇒ TouchGuest» — доменная политика CRM в адаптере.
- `http/org.go:197-203` — `validAccent` дублирует `domain.ValidAccent`
  (одно из двух надо удалить); `org.go:247` валидирует accent в хендлере,
  хотя `SaveTheme` проверит снова.
- `http/org.go:32` — вычисление `renews_at` (30 дней) — бизнес-фикция в
  view-маппере.

App-слой обходит домен:

- `app/app.go:282,290` — `ChangePlan` собирает `Subscription` руками, в
  обход `Transition` (терминальность canceled не работает).
- `app/app.go:166-169` — retry слага по `strings.Contains(err.Error(), "slug")`
  — сопоставление по тексту ошибки вместо типизированного конфликта.

Store содержит бизнес-правила/чужие данные:

- `postgres/postgres.go:283-291` — `insertDefaultMenu`: platform-store
  пишет в таблицу `menus` menu-контекста (осознанная атомарность
  провижининга, но связь недокументирована портом).
- `postgres/postgres.go:425-443` — delete+insert домена без транзакции.
- `postgres/customers.go:188-210, 332-338` — SQL по чужим таблицам
  `orders/order_lines/tickets/ticket_lines` (read-модель CRM, допустимо,
  но это точка связности, которая сломается молча при миграциях menu/pos).
- `postgres/assistant.go:119-136` — правило decide-once выражено только в
  SQL-предикате.
- `postgres/postgres.go:182` — истечение сессии только в SQL.

### 3.6 Точки связи с соседними контекстами (фиксация, без анализа)

- Общая таблица `restaurants`: создаётся в menu-миграциях, platform
  добавляет колонки `org_id/address/hours/contacts`
  (`migrations/platform/0001` — ALTER TABLE). Menu-контекст держит свою
  узкую проекцию (id, slug, name).
- `orders.customer_id` (menu/0004) — FK на `customers` добавляется в
  platform/0003; `tickets.customer_id` (pos/0003) и
  `cart_handoffs.customer_id` (menu/0004) — без FK.
- `insertDefaultMenu` — platform-store пишет в `menus`.
- CRM-агрегаты (`guestTotalsSQL`) читают `orders/tickets` напрямую.
- HTTP-адаптер platform хостит хендлеры menu-админки, POS и динера
  (композиция через `Deps.Menu/MenuAdmin/MenuApp/Pos`) — единая точка
  session-auth и error-mapping.

---

## 4. Рекомендации по рефакторингу (по приоритету, маленькими шагами)

Приоритет 1 — корректность инвариантов (маленькие, самодостаточные шаги):

1. **Провести `ChangePlan` через state machine.** Метод домена
   `Subscription.ChangePlan(newPlan, providerStatus) error`, внутри —
   только `Transition`; решить и зафиксировать политику реактивации
   canceled (либо явный переход `canceled→trialing` в таблице, либо 422).
   Один файл домена + упрощение `app.go:266-303`; тест на «canceled
   нельзя молча воскресить».
2. **Убрать retry слага по тексту ошибки**: научить store возвращать
   типизированный конфликт (`ErrSlugTaken` или `ErrConflict` с
   errors.Is-обёрткой поля) — правка `postgres.go:53` и `app.go:166`.
3. **Транзакция в `SetCustomDomain`** (delete+insert) — три строки.
4. **Симметрия лимитов плана**: перенести проверку лимита блюд из
   `menuadmin.go:459-473` в app (метод `App.EnsureCanCreateItem(orgID,
   restaurantID)` рядом с логикой `CreateRestaurant`), хендлер зовёт одну
   функцию.

Приоритет 2 — вытащить юзкейсы из HTTP-адаптера (по одному, без изменения
поведения):

5. **`AssistantService` в app**: перенести `assistantPrompt`, `applyAction`
   и батч-семантику apply из `http/assistant.go` в app-слой (методы
   `SendMessage`, `ApplyActions`, `DiscardActions`); хендлеры остаются
   парсингом multipart и кодами ответов. Порт menu-команд у app уже есть
   прецедентом (`ItemLimitFor`); понадобится узкий интерфейс на
   MenuAdmin-операции. Заодно исчезнет дублирование «position нового меню».
6. **Handoff-accept как app-сценарий**: сага consume/compensate/link/touch
   → метод в app (platform или отдельный «process» в http остаётся, но
   одним вызовом). Ponytail-комментарий про единую транзакцию — сюда же.
7. **TouchGuest как политика**: единственная точка «заказ/handoff
   залогиненного динера ⇒ touch» (сейчас три вызова в двух файлах).
8. Удалить дубликат `validAccent` в `http/org.go:197` (использовать
   `domain.ValidAccent`) и лишнюю раннюю проверку accent в `putTheme`.

Приоритет 3 — доменная модель (по мере надобности, не разом):

9. **`Theme.Apply`/`NewTheme` в домене**: перенести валидацию из
   `App.SaveTheme` в доменный конструктор; app остаётся load→save. Убирает
   и тройное знание структуры theme JSON (app, org.go view, diner.go).
10. **VO `Slug`, `Email`, `Hostname`** — по одному, начиная со Slug (три
    места использования).
11. **Инвариант User в типе**: фабрики `NewOwner(org, email, hash)` /
    `NewStaff(org, restaurant, role, ...)`, чтобы «owner без ресторана,
    staff с рестораном» не был конвенцией.
12. **`Thread.DecideMessage`**: правило decide-once в домене; SQL-предикат
    остаётся защитой от гонки.

Приоритет 4 — события (только когда появится первый консьюмер; до того —
YAGNI):

13. Обновить `docs/EVENTS.md` правками из 3.4 (дешёво, можно сразу).
14. Начать писать outbox с одного события, у которого есть реальный
    потребитель (кандидат: `AssistantActionsApplied` — заменяет slog-аудит
    устойчивым журналом; или `OrderPlaced` в menu для CRM). Вставка в ту же
    транзакцию, что и изменение агрегата; `AggregateRoot.Raise` + сбор
    событий в app после commit — каркас уже есть в sharedkernel.
15. Публишер (poll `events_pending` → deliver → `MarkEventPublished`) —
    только вместе с первым внешним потребителем.

Отдельно зафиксировать (не чинить сейчас): невыполнимость различения
manual/ai_proposal у `ThemeApplied` при текущем «apply = тот же PUT»;
статус «invited» персонала не хранится (сломается при листинге);
неатомарность проверки лимита ресторанов (гонка допустима для v1).
