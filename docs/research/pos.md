# Ресёрч: bounded context POS

Дата: 2026-08-24. Область: `backend/internal/pos/{app,ports,adapters}`, `backend/internal/domain/pos/`, `backend/internal/sharedkernel/`, `backend/migrations/pos/`, HTTP-адаптеры `backend/internal/platform/adapters/http/{pos.go,handoff.go}`, контракты `docs/PLATFORM.md` и `docs/EVENTS.md`. Соседние контексты (menu, platform) рассматриваются только в точках соприкосновения.

---

## 1. Логика as-is

### 1.1 Слои и маршрутизация

Схема слоёв стандартная гексагональная: HTTP-адаптер → `pos/app.App` → порты (`ports.Store`, `ports.Menu`) → адаптеры (`adapters/postgres.Store`, `adapters/menubridge.Bridge`). Доменные типы — `internal/domain/pos`.

Маршруты (все в `backend/internal/platform/adapters/http/http.go:135-141,157-158`):

| Маршрут | Обработчик |
|---|---|
| `GET  /api/v1/pos/state` | `posState` (pos.go:234) |
| `POST /api/v1/pos/shifts` | `posOpenShift` (pos.go:316) |
| `POST /api/v1/pos/shifts/{id}/close` | `posCloseShift` (pos.go:335) |
| `POST /api/v1/pos/tables/{tableID}/lines` | `posAddLines` (pos.go:366) |
| `POST /api/v1/pos/tickets/{id}/fire` | `posFire` (pos.go:394) |
| `POST /api/v1/pos/requests/{id}/ack\|dismiss` | `posAckRequest`/`posDismissRequest` (pos.go:407-413) |
| `GET  /api/v1/pos/handoff/{code}` | `posHandoffPreview` (handoff.go:206) |
| `POST /api/v1/pos/handoff/{code}/accept` | `posHandoffAccept` (handoff.go:219) |

Все POS-маршруты обёрнуты в middleware `h.pos` (pos.go:32-61): аутентификация сессией + разрешение ресторана. Сотрудник с `RestaurantID` действует только на своём ресторане; org-пользователь (владелец) — через `?restaurant_id=` (с проверкой принадлежности организации, pos.go:45) или первый ресторан организации. Handoff-маршруты дополнительно обёрнуты в `public(...)` — per-IP rate limit, чтобы 6-символьные коды нельзя было перебрать даже враждебной staff-сессией (http.go:155-158, комментарий там же).

### 1.2 Сценарий: `GET /pos/state` (hot path, поллинг ~5с)

`posState` (pos.go:234) → `App.State` (app.go:52):

1. `store.OpenTickets` — все открытые тикеты ресторана с позициями за **два** запроса: тикеты, затем `ticket_lines WHERE ticket_id = ANY($1)` (postgres.go:201-266, `attachLinesBatch`).
2. `store.OpenShiftFor` — открытая смена (partial-index-запрос `closed_at IS NULL`). Если есть:
   - `store.ShiftSequence` — порядковый номер «shift-N» через `count(*)` по `opened_at` (postgres.go:95-109);
   - `store.ShiftClosedSalesCents` — SQL-агрегат продаж по **закрытым** тикетам смены (postgres.go:270-286);
   - running expected = `OpeningFloatCents + closedSales + Σ TotalCents()` открытых тикетов этой смены (app.go:77-81).
3. `menu.Tables` и `menu.PendingServiceRequests` через menubridge.

Дальше хендлер (не app!) докидывает: `Platform.RestaurantPublic`, `Menu.Menu` (категории+позиции), `MenuAdmin.Menus`, собирает view (метки опций, `unit_price_cents` с дельтами, времена «HH:MM» в `posLocation` — глобальная TZ из `RESTAURANT_TZ`, pos.go:66-71) и считает **ETag**: `sha256` от готового JSON-тела, первые 16 байт в hex (pos.go:291-313). `If-None-Match` совпал → `304` без тела. Кэша нет — просто хэш на каждый запрос; для 5-секундного полла это осознанно дёшево.

### 1.3 Сценарий: открытие смены

`posOpenShift` (pos.go:316) → `App.OpenShift` (app.go:106): валидация `opening_float_cents >= 0` (`ErrInvalid` → 422), сборка `domain.Shift` в app-слое, `store.OpenShift` — голый `INSERT` (postgres.go:41-52). Инвариант «одна открытая смена на ресторан» держит **только БД**: partial unique index `shifts_open_per_restaurant_idx ON shifts (restaurant_id) WHERE closed_at IS NULL` (0001_init.up.sql); нарушение (`23505`) маппится в `ports.ErrConflict` → 409. После вставки — перечитывание `ShiftByID`, потому что `opened_at` проставляет `DEFAULT now()` в БД. `cashier` — display name, денормализованный на строку смены при открытии (миграция 0003), чтобы `pos/state` не делал per-poll lookup пользователя; для старых смен с пустым `cashier` хендлер делает fallback-lookup `Platform.User(s.OpenedBy)` (pos.go:83-90).

### 1.4 Сценарий: закрытие смены (и известная гонка — разбор в §3.2)

`posCloseShift` (pos.go:335) → `App.CloseShift` (app.go:127):

1. `store.ShiftByID` — чтение смены (без блокировки);
2. `store.TicketsForShift` — все тикеты смены с позициями, сумма `TotalCents()` в Go (v1: все продажи считаются наличными, способов оплаты нет);
3. `sh.Close(declared, sales, now)` — **доменный** метод (domain.go:41-55): `expected = float + sales`, `variance = declared − expected`; `ErrShiftClosed`, если уже закрыта; `ErrNegativeAmount` при отрицательных суммах;
4. `store.CloseShift` — `UPDATE ... WHERE closed_at IS NULL` (postgres.go:81-93): guard в WHERE и есть иммутабельность — повторное/конкурентное закрытие затрагивает 0 строк → `ErrConflict`;
5. `store.CloseTickets` — bulk `UPDATE tickets SET status='closed'` по смене (postgres.go:344-352).

Ответ — PostedShift-форма контракта (`number`, `expected/declared/variance_cents`, `posted_at`, фейковые `gl_lines: 2`) — pos.go:355-363.

### 1.5 Сценарий: добавление позиций (официант или handoff)

`posAddLines` (pos.go:366) → `App.AddLines` (app.go:177):

1. ≥1 строка, иначе `ErrInvalid`;
2. `menu.TableByID` — стол существует и принадлежит ресторану (tenant scope);
3. `store.OpenShiftFor` — без открытой смены → `ErrNoOpenShift` (409);
4. на каждую строку: `menu.MenuItemByID` → резолв опций-меток в ID (`optionIDByLabel`, app.go:248 — POS-клиент шлёт метки) → **переиспользование валидации menu-контекста** `menudomain.NewOrderLine` (qty, доступность, принадлежность опций) → снапшот в `domain.TicketLine` (имя, цена, опции копируются — последующие правки меню тикет не меняют);
5. get-or-create открытого тикета стола: `OpenTicketForTable`, при `ErrNotFound` — `CreateTicket` под текущей сменой. Инвариант «один открытый тикет на стол» держит partial unique index `tickets_open_per_table_idx ON tickets (table_id) WHERE status='open'` (0001_init.up.sql); гонка create-create отдаст второму `ErrConflict` → 409 клиенту (ретрая-перечитывания нет);
6. `store.AddLines` — INSERT позиций в транзакции (opts как jsonb) — **единственная явная транзакция во всём POS-сторе**;
7. непустой `note` (только путь handoff) — `AppendTicketNote`: newline-конкатенация в SQL (postgres.go:167-178);
8. перечитывание `TicketByID` → view.

### 1.6 Сценарий: fire

`App.Fire` (app.go:260): `TicketByID` (только для 404), затем `store.FireTicket` — `UPDATE ticket_lines SET fired_at = now() WHERE fired_at IS NULL AND ticket_id IN (...)` (postgres.go:300-311). Идемпотентно: повторный fire затрагивает 0 строк и это ок. Статус тикета **не проверяется** — fire проходит и по закрытому тикету (см. §3.5).

### 1.7 Сценарий: приём handoff-кода (мост из menu-контекста)

Создание handoff — целиком menu-контекст (`dinerHandoff`, handoff.go:28: валидация 1–50 строк, note ≤500, снапшот через тот же `NewOrderLine`, cooldown по токену стола, код 6 символов A-Z2-9 без 0/O/1/I, TTL 15 мин, single-use). POS только потребляет:

- `posHandoffPreview` (handoff.go:206): `MenuAdmin.HandoffByCode` (код upper-case'ится; неизвестный/просроченный/чужой/использованный — одинаковые 404), view со строками в форме тикет-линий; имя клиента — только имя, никогда email/телефон (handoff.go:182-187).
- `posHandoffAccept` (handoff.go:219): паттерн **consume-first + компенсация**:
  1. `MarkHandoffUsed` — потребить код первым (single-use, блокирует конкурентный двойной accept);
  2. `Pos.AddLines` обычным путём (стол — из handoff или переопределён `table_id` из тела; чужой `table_id` отсечётся в `AddLines` проверкой `TableByID`);
  3. при ошибке — `UnmarkHandoffUsed` на `context.WithoutCancel` (отмена запроса — ровно тот случай, когда unmark обязан пройти); там же `ponytail:`-комментарий: одна кросс-сторная транзакция убрала бы компенсацию;
  4. при `CustomerID` — `Pos.LinkTicketCustomer` (first-wins: `UPDATE ... WHERE customer_id IS NULL`, postgres.go:290-298) + `Platform.TouchGuest`; обе ошибки — только warn, не откат.

### 1.8 Инварианты: сводка «что и где enforced»

| Инвариант | Где enforced | Оценка |
|---|---|---|
| Одна открытая смена на ресторан | Только БД: partial unique index `shifts_open_per_restaurant_idx` | Надёжно, race-proof. App/домен правило не знают — узнают постфактум по `ErrConflict` |
| Один открытый тикет на стол | Только БД: partial unique index `tickets_open_per_table_idx` | Надёжно; но app при гонке get-or-create отдаёт 409 вместо ретрая (app.go:227-235) |
| Смена иммутабельна после закрытия | Два слоя: домен `Shift.Close` → `ErrShiftClosed` (domain.go:42-44) + стор `WHERE closed_at IS NULL` (postgres.go:83-91) | Лучший инвариант контекста — защищён и в домене, и в БД |
| Тикет иммутабелен после закрытия | **Частично, случайно**: `AddLines` не попадёт в закрытый тикет лишь потому, что `OpenTicketForTable` ищет только открытые; `FireTicket`, `AppendTicketNote`, `LinkTicketCustomer` статус **не проверяют** | Дыра (см. §3.5) |
| Деньги смены (expected = float + sales, variance = declared − expected) | Формула — в домене (`Shift.Close`); но сумма `sales` считается **вне** транзакции закрытия (app.go:133-140), а running expected в state и агрегат закрытых продаж дублируют `TotalCents()` в SQL (postgres.go:270-286) | Гонка read-compute-write, §3.2 |
| Tenant isolation | Каждый запрос стора фильтрует `restaurant_id`; исключение — `Store.AddLines(ctx, ticketID, lines)` без `restaurantID` (ports.go:56, postgres.go:143) — безопасно только потому, что app всегда резолвит `ticketID` scoped-запросом | Приемлемо, но сигнатура порта слабее остальных |
| Снапшот позиций (правки меню не трогают тикет) | Копирование полей в `TicketLine` при добавлении (app.go:217-224) + отсутствие UPDATE-путей для строк, кроме `fired_at` | Надёжно |
| Single-use handoff | Menu-стор `MarkHandoffUsed` + компенсация в хендлере | Работает; оркестрация живёт не в том слое (§3.6) |

---

## 2. Сущности, связи, анемичность

### 2.1 Схема (migrations/pos/0001–0003)

```
shifts    (id PK, restaurant_id FK→restaurants CASCADE, opened_by FK→users,
           cashier text DEFAULT '' [0003], opened_at DEFAULT now(),
           opening_float_cents, closed_at?, declared_cents?, expected_cents?, variance_cents?)
tickets   (id PK, restaurant_id FK→restaurants CASCADE, shift_id FK→shifts CASCADE,
           table_id FK→tables CASCADE, customer_id? БЕЗ FK [0003, cross-batch],
           status 'open'|'closed', note text DEFAULT '' [0002], created_at)
ticket_lines (id PK, ticket_id FK→tickets CASCADE, menu_item_id FK→menu_items,
           name, unit_price_cents, qty, options jsonb [{label, price_delta_cents}],
           fired_at?, created_at)
```

Плюс partial unique индексы из §1.8 и `tickets_restaurant_customer_idx` (CRM-выборки).

### 2.2 Связи с соседними контекстами (только точки касания)

- `shifts.opened_by → users(id)` — **FK через границу контекста** в platform. На уровне Go POS о платформенном User не знает (только `sharedkernel.ID`); связь используется единожды — fallback имени кассира в HTTP-адаптере (pos.go:87).
- `tickets.table_id → tables(id)`, `ticket_lines.menu_item_id → menu_items(id)` — FK в menu-контекст. На уровне кода — через `ports.Menu` / `menubridge`.
- `tickets.customer_id` — **сознательно без FK** (комментарий «cross-batch» в 0003): миграции контекстов накатываются пакетами, порядок не гарантирован. Единственная связь, оформленная как «мягкая ссылка» — так и должны выглядеть остальные кросс-контекстные ссылки, если контексты когда-нибудь разъедутся по БД.
- `menubridge.Bridge` (menubridge.go) — реализация `ports.Menu` поверх `menuports.AdminStore`: транслирует только **ошибки** (`menuports.ErrNotFound` → `ports.ErrNotFound`), но возвращает **типы menu-домена как есть** (`menudomain.MenuItem`, `Table`, `ServiceRequest`). Это тонкий ACL: по ошибкам граница есть, по типам — нет (POS app и даже `pos/ports` импортируют `menudomain`, ports.go:10).

### 2.3 Анемичность

- **`Shift`** — наполовину богатая: `Open()` и `Close()` инкапсулируют главный денежный инвариант, тест есть (domain_test.go:9-38). Но конструктора нет — app собирает структуру пополево (app.go:110-116), и «одна открытая на ресторан» для домена невидима.
- **`Ticket`** — анемичен: `Status string` с константами-строками, ни `Close()`, ни `Fire()`, ни `AddLine()`, ни проверки «менять можно только открытый». Все переходы состояния выполняются SQL-ом в сторе. Единственное поведение — `TotalCents()`.
- **`TicketLine`** — снапшот с `TotalCents()`; по сути ок, это value-подобная запись внутри тикета, хоть и с ID (нужен для fire по строкам в будущем).
- **`LineOption`** — чистый value object (label + delta, иммутабелен). Хорошо.
- **sharedkernel**: POS использует только `sharedkernel.ID` (alias `uuid.UUID`). `Entity`, `AggregateRoot`, `DomainEvent` не задействованы — событий контекст не поднимает вообще.
- Отдельно: `adapters/postgres/posdb/` — sqlc-генерат (2 запроса, `queries/pos/pos.sql`) **никем не импортируется**. Реальный стор — рукописный `adapters/postgres/postgres.go`. Мёртвый код с риском расхождения (sqlc въехал последним коммитом 7991531; видимо, задел на миграцию стора).

---

## 3. DDD-разбор

### 3.1 Агрегаты и их границы

**Shift — корень без детей.** Граница: сама смена и её денежное закрытие. Тикеты в агрегат **не входят** — и это правильно: тикеты живут своей жизнью (официанты параллельно добавляют позиции), затаскивать их в границу Shift означало бы сериализовать весь зал через одну версию агрегата. Но следствие: «expected по смене» — это **вычисление поверх чужих агрегатов**, и его консистентность нужно обеспечивать транзакцией чтения+записи, чего сейчас нет (§3.2). Инвариант «одна открытая на ресторан» — межагрегатный (уровня ресторана), и partial unique index — каноничное DDD-решение для такого (uniqueness через хранилище, не через агрегат). Тут придраться не к чему, стоит только оставить как есть.

**Ticket — корень с детьми TicketLine.** Защищаемые инварианты: (а) позиции добавляются только в открытый тикет; (б) закрытый тикет иммутабелен; (в) снапшот-семантика строк; (г) один открытый на стол — опять межагрегатный, отдан БД (правильно). Проблема: инварианты (а) и (б) в коде агрегата не выражены — они «получаются» из того, какие SQL-запросы написаны (§3.5). Строки добавляются мимо корня: `store.AddLines(ticketID, lines)` — корень как объект в операции не участвует, версии/оптимистичной блокировки нет. Для v1 это терпимо (append-only, конфликтов по строкам нет), но `fired_at` и `status` — уже конкурентные поля без какой-либо версионности.

### 3.2 Разбор гонки read-compute-write в CloseShift

Код: app.go:127-155. Последовательность: `ShiftByID` (без lock) → `TicketsForShift` (снимок продаж) → `Shift.Close` (домен считает expected/variance) → `store.CloseShift` (guard `closed_at IS NULL`) → `store.CloseTickets`. Пять запросов, **ни одной общей транзакции**.

Разложим по сценариям:

- **Двойное/конкурентное закрытие** — защищено: второй `UPDATE ... WHERE closed_at IS NULL` затронет 0 строк → `ErrConflict` → 409. Здесь guard работает как compare-and-swap, гонки нет.
- **Гонка №1 (потерянные продажи):** официант делает `AddLines`, коммит его строк ложится **между** `TicketsForShift` (шаг 2) и `CloseTickets` (шаг 5). Строки не попали в `sales` → `expected_cents` занижен, `variance_cents` завышен — а смена уже иммутабельна. Затем `CloseTickets` закрывает тикет вместе с неучтёнными строками — деньги молча выпали из кассовой сверки. Окно узкое (миллисекунды между двумя запросами), но это ровно та «money path», где полагаться на узость окна нельзя. Замечу: `AddLines` перед вставкой проверяет `OpenShiftFor` (app.go:187) — но эта проверка тоже читает без блокировки и успевает пройти до того, как смена закрыта, так что от гонки не спасает.
- **Гонка №2 (частичный отказ):** упали/оборвались между шагом 4 и 5 — смена закрыта, тикеты остались открытыми. Дальше цепочка: новые `AddLines` получают `ErrNoOpenShift` (открытой смены нет) — пока кто-то не откроет новую смену; после этого стол с «осиротевшим» открытым тикетом принимает строки в тикет **закрытой** смены (get-or-create найдёт его по `table_id`), а `State` эти суммы в running expected не включит (фильтр `t.ShiftID == shift.ID`, app.go:79) и закрытие новой смены их тоже не посчитает (`TicketsForShift` по `shift_id`). Деньги снова теряются, уже устойчиво.
- **Гонка №3 (симметричная №1, безобиднее):** `AddLines` создаёт тикет после `CloseTickets`, но до конца запроса — тикет просто останется открытым под закрытой сменой, дальше как №2.

**Корень всех трёх — один:** закрытие смены не атомарно и снимок продаж не согласован с переводом тикетов в closed. Правильная форма — одна транзакция стора:

```sql
BEGIN;
SELECT ... FROM shifts WHERE id=$1 AND restaurant_id=$2 FOR UPDATE;  -- и проверить closed_at IS NULL
UPDATE tickets SET status='closed' WHERE shift_id=$1 AND status='open';
-- sales: SQL-агрегат по ВСЕМ тикетам смены (теперь все closed, снимок согласован)
UPDATE shifts SET closed_at=..., declared_cents=..., expected_cents=..., variance_cents=... WHERE id=$1;
COMMIT;
```

`FOR UPDATE` на строке смены заодно сериализует закрытие против `AddLines`, если `AddLines` в своей транзакции тоже будет брать shared-блокировку смены (`FOR SHARE`) — либо проще: `AddLines`-транзакция после вставки перепроверяет `closed_at IS NULL`. Минимальный вариант, закрывающий №1 и №2 без переделки AddLines: перенести шаги «закрыть тикеты → посчитать → закрыть смену» в один транзакционный метод стора и считать sales SQL-ом **после** закрытия тикетов внутри той же транзакции. Домен при этом не страдает: `Shift.Close(declared, sales, at)` по-прежнему вычисляет expected/variance — транзакция лишь поставляет ему согласованный `sales` (паттерн «closure of operations»: домен чистый, атомарность — забота адаптера).

### 3.3 Entity vs Value Object: деньги

Сейчас деньги — голые `int` центы везде (контракт PLATFORM.md:219 «Money: integer cents everywhere»). Для v1 (одна валюта, одна страна) полноценный VO `Money{amount, currency}` — оверинжиниринг: он окупается при мультивалютности, которой нет и не заявлено. Что **действительно** просится в VO:

- **Тройка закрытия `DeclaredCents/ExpectedCents/VarianceCents *int`** (domain.go:24-26) — три nullable-указателя, валидные только все вместе и только при `ClosedAt != nil`. Это классический признак пропущенного VO: `type ShiftClosure struct { At time.Time; DeclaredCents, ExpectedCents, VarianceCents int }` и поле `Closure *ShiftClosure`. Инвариант «либо всё, либо ничего» становится невыразимым неправильно, разыменования `*sh.ExpectedCents` в хендлере (pos.go:357-360) перестают быть потенциальными nil-паниками.
- **`LineOption`** — уже VO, оставить.
- **`Ticket.Status string`** → определённый тип `type TicketStatus string` с константами — копеечная типобезопасность вместо сравнения строк.
- Опционально `type Cents = int` (alias для читаемости сигнатур) — но это вкусовое, можно не делать.

Entity: `Shift`, `Ticket`, `TicketLine` — корректно entity (идентичность + жизненный цикл). `TicketLine` пограничен: после создания меняется только `fired_at`, но ID нужен (адресация строк), так что entity внутри агрегата — верно.

### 3.4 Домен-события: сверка с docs/EVENTS.md

Факт: **ни одно событие не поднимается**. `sharedkernel.AggregateRoot` не встроен ни в один POS-тип, outbox-таблица `events` (platform/0004_events.up.sql) существует, писателей нет. EVENTS.md честно говорит «None are wired yet — this is the contract to implement against». Сверка каталога POS с реальным кодом:

| Событие каталога | Соответствие коду | Замечания / правки |
|---|---|---|
| `ShiftOpened` (shift_id, restaurant_id, opened_by, float_cents) | точка — `App.OpenShift` | **Добавить `cashier` в payload** — он денормализован именно ради потребителей без lookup'а; консьюмеру отчётов он нужен так же. Поле `float_cents` переименовать в `opening_float_cents` — консистентно с колонкой и остальными payload |
| `ShiftClosed` (declared/expected/variance_cents) | точка — транзакция закрытия (§3.2) | Ок; `restaurant_id` идёт колонкой outbox по конвенции. Стоит добавить `sales_cents` — иначе консьюмер вычисляет его вычитанием |
| `TicketOpened` (ticket_id, shift_id, table_id) | точка — create-ветка `App.AddLines` | Ок. Добавить `customer_id?` нельзя — линковка происходит позже; вместо этого см. ниже |
| `LinesFired` (ticket_id, line_ids) | точка — `Fire` | Уточнить в каталоге: fire идемпотентен, `line_ids` = **только реально прошитые этим вызовом** строки (сейчас стор даже не возвращает их — придётся `UPDATE ... RETURNING id`). Иначе повторный fire родит дубль-событие с теми же строками |
| `TicketClosed` (ticket_id, total_cents, customer_id?) | точка — bulk `CloseTickets` | **Главная нестыковка:** в v1 тикеты закрываются только пачкой при закрытии смены, отдельного «оплатить тикет» нет. Событие per-ticket, а переход — одним SQL UPDATE. Реализация обязана эмитить по событию на тикет внутри транзакции закрытия (данные уже прочитаны для sales — дёшево). Зафиксировать это в каталоге («raised when: shift close in v1; direct ticket payment later») |

Чего в каталоге **не хватает** (предложения):

- **`TicketLinesAdded` (ticket_id, line_ids, added_cents)** — сейчас путь официанта/handoff вообще не наблюдаем: `OrderPlaced` покрывает только заказ дайнера в menu-контексте, kitchen/аналитика/AI-прогнозирование (заявленное направление продукта) не увидят добавления позиций до самого `TicketClosed` в конце смены. Это самый частотный бизнес-факт контекста — и единственный несобытийный.
- **`TicketCustomerLinked` (ticket_id, customer_id)** — либо это, либо явная запись в каталоге, что CRM-спенд достаточно `customer_id?` в `TicketClosed` (текущая схема CRM читает таблицы напрямую, так что можно и не заводить — но решение стоит зафиксировать).
- `HandoffAccepted` (menu-контекст) содержит `ticket_id, accepted_by` — payload кросс-контекстный; это нормально для интеграционного события, но эмитить его логично из будущего use case `AcceptHandoff` в POS app (см. §3.6), иначе у события два хозяина.

Механика: embed `sharedkernel.AggregateRoot` в `Shift`/`Ticket`, `Raise()` внутри доменных методов (`Close`, будущих `Fire`/`AddLine`), app сливает `Events()` в outbox **той же транзакцией**, что и изменение агрегата — ровно как написано в шапке EVENTS.md. Это требует, чтобы у app появился транзакционный порт (сейчас каждая операция стора — своя мини-транзакция), что совпадает с фиксом §3.2.

### 3.5 Что должно жить в domain vs app vs adapters — конкретные нарушения

Бизнес-правила, утёкшие в SQL (adapters/postgres/postgres.go):

- **postgres.go:300-311, `FireTicket`** — правило «прошиваются только непрошитые строки» и *отсутствующее* правило «только у открытого тикета» живут в WHERE. Fire по закрытому тикету сейчас проходит (эндпоинт `posFire` доступен по любому ticket id ресторана). Доменного метода `Ticket.Fire(at)` нет.
- **postgres.go:344-352, `CloseTickets`** — переход состояния целого агрегата bulk-UPDATE'ом, домен не участвует; отсюда же невозможность поднять `TicketClosed` per-ticket.
- **postgres.go:270-286, `ShiftClosedSalesCents`** — формула `((unit + Σdelta) * qty)` **продублирована в SQL** относительно `TicketLine.TotalCents()` (domain.go:108-114). Две реализации одной денежной формулы разъедутся при первом изменении (скидки, сервисный сбор).
- **postgres.go:167-178, `AppendTicketNote`** — правило склейки заметок (newline-join) в CASE-выражении SQL; статус тикета не проверяется.
- **postgres.go:290-298, `LinkTicketCustomer`** — правило «first link wins» в WHERE; помимо прочего молча «успешно» проходит и для несуществующего тикета (RowsAffected не проверяется), и для закрытого.

Логика use case, утёкшая в HTTP-адаптер:

- **handoff.go:219-269, `posHandoffAccept`** — полноценный сценарий приложения (consume → add lines → компенсация → link customer → touch guest), живущий в хендлере и дёргающий напрямую сторы **двух** контекстов (`h.MenuAdmin`, `h.Platform`) плюс `h.Pos`. Это app-слой по всем признакам; в адаптере ему не место — не тестируется без HTTP, дублирует знание о компенсации там, где живут json-декодеры. Должен стать `posapp.AcceptHandoff` с портом на handoff-операции menu-контекста (расширение `ports.Menu` или отдельный маленький порт + реализация в menubridge).
- **pos.go:234-314, `posState`** — композиция «floor view» размазана: половина в `App.State`, половина (ресторан, полное меню, список меню, ETag) в хендлере с прямыми вызовами `h.Menu`, `h.MenuAdmin`, `h.Platform`. Меню в state — часть POS-контракта (PLATFORM.md:199-208), значит, его сборка (`posMenu`, pos.go:188-230 — фильтрация available, «первая single-select группа как mods», префиксация имени меню) — прикладная/презентационная логика, которую стоит хотя бы собрать в одном слое. ETag — легитимно адаптерный.
- **pos.go:83-90** — fallback-lookup кассира в адаптере: приемлемо (чисто презентационная заплатка на старые данные), но помечу как кандидата на удаление после бэкфилла `cashier`.

Сборка агрегатов в app вместо домена:

- **app.go:110-116** (`OpenShift`) и **app.go:229** (`AddLines`, создание тикета) — структуры собираются полями. Нет `domain.NewShift(...)` / `domain.NewTicket(...)`, которые были бы естественным местом валидации (`float >= 0` сейчас в app, app.go:107) и `Raise(ShiftOpened)` / `Raise(TicketOpened)`.

Что расположено **правильно**:

- Снапшот-валидация позиций через `menudomain.NewOrderLine` (app.go:209) — переиспользование доменного правила соседнего контекста вместо копипасты; осознанный компромисс границы (тип `OrderLine` menu-домена используется как DTO). Для in-process монолита по ADR 0001 — адекватно.
- `ports.Store`/`ports.Menu` как Go-интерфейсы, ошибки-сентинели, маппинг ошибок в menubridge — чистая гексагональность.
- Partial unique индексы для межагрегатных uniqueness-инвариантов — образцово.
- Иммутабельность смены в двух слоях (домен + WHERE-guard).

### 3.6 Sharedkernel

Минималистичен и правилен (ID-alias, Entity, AggregateRoot с буфером событий, DomainEvent + EventBase), не импортирует контексты. Проблема одна: он декоративен — `AggregateRoot`/`DomainEvent` не использует никто, а `Entity` не embed'ит даже сам POS (у `Shift`/`Ticket` свои ID/CreatedAt поля, что comment в entity.go:26-28 явно разрешает). Риск: «библиотека на вырост», которая устареет до первого использования. Лечится пунктом про события (§4, шаг 5), не удалением.

---

## 4. Рекомендации по рефакторингу (по приоритету, маленькими шагами)

Порядок — по риску для денег и данных, каждый шаг самодостаточен и коммитится отдельно.

**Шаг 1 — атомарное закрытие смены (money-critical, маленький diff).**
Один транзакционный метод стора `CloseShift(ctx, restaurantID, shiftID, declaredCents)`-вида: `SELECT ... FOR UPDATE` смены → `UPDATE tickets SET status='closed'` → SQL-агрегат sales по уже закрытым тикетам → `Shift.Close` → `UPDATE shifts`. App-метод сжимается до вызова + маппинга ошибок. Закрывает гонки №1 и №2 из §3.2 разом. Тест: конкурентный `AddLines` во время закрытия либо попадает в sales, либо получает отказ — но не теряется.

**Шаг 2 — иммутабельность закрытого тикета (три WHERE-условия).**
Добавить `AND status='open'` (через подзапрос/JOIN на tickets) в `FireTicket`, `AppendTicketNote`, `LinkTicketCustomer`; в `LinkTicketCustomer` начать различать «0 строк» (сейчас молча ок). Плюс в `AddLines`-путь app — защита от осиротевших тикетов: если `OpenTicketForTable` вернул тикет с `ShiftID != текущая смена` — отказ 409 (данные уже неконсистентны, чинить руками, но не усугублять). После шага 1 такие тикеты перестанут возникать.

**Шаг 3 — обработать гонку get-or-create тикета (пять строк).**
В `App.AddLines` при `ErrConflict` от `CreateTicket` — перечитать `OpenTicketForTable` и продолжить, вместо 409 клиенту. Два официанта, жмущих «добавить» на пустом столе одновременно, — не конфликт, а норма зала.

**Шаг 4 — вытащить `AcceptHandoff` из HTTP-адаптера в pos/app.**
Перенести оркестрацию consume/компенсация/link из `posHandoffAccept` в `posapp.AcceptHandoff`; операции с handoff добавить в `ports.Menu` (`HandoffByCode`, `MarkUsed`, `UnmarkUsed`) и реализовать в menubridge. Хендлер остаётся декодером+вьюхой. Побочный выигрыш: сценарий тестируется юнитово, и будущее `HandoffAccepted`-событие обретает одного хозяина.

**Шаг 5 — события + богатый домен, одной связкой, инкрементально.**
5a: конструкторы `domain.NewShift`, `domain.NewTicket` (валидация переезжает из app), embed `sharedkernel.AggregateRoot` в оба корня. 5b: `Ticket.Fire(at)`, `Ticket.Close()`, `Shift` уже умеет `Close` — методы поднимают события каталога. 5c: outbox-писатель в транзакциях стора (таблица `events` готова с platform/0004). 5d: правки EVENTS.md из §3.4 — `cashier` и `opening_float_cents` в `ShiftOpened`, `sales_cents` в `ShiftClosed`, семантика идемпотентного `LinesFired`, точка эмиссии `TicketClosed` = закрытие смены в v1, новое `TicketLinesAdded`. Не пытаться сделать всё сразу — каждое под-изменение живёт отдельно.

**Шаг 6 — убрать дубль денежной формулы.**
После шага 1 `ShiftClosedSalesCents` остаётся только для running expected в `State`. Варианты: (а) оставить и повесить тест-сверку SQL против `TotalCents()` на одних данных; (б) считать running expected в Go из уже загруженных закрытых тикетов — но это лишние строки в hot path. Рекомендация: (а) — дешевле, дубль фиксируется тестом.

**Шаг 7 — решить судьбу sqlc (`posdb`).**
Пакет `adapters/postgres/posdb` не импортируется никем. Либо мигрировать рукописный стор на sqlc-запросы (по одному методу за PR), либо удалить генерат и `queries/pos/pos.sql` до реальной миграции. Держать обе версии запросов — гарантированное расхождение.

**Шаг 8 (низкий приоритет) — мелкая типизация домена.**
`ShiftClosure` VO вместо тройки `*int` (убирает разыменования в pos.go:357-360), `type TicketStatus string`. `Money`-VO **не заводить** — одна валюта, YAGNI; пересмотреть при появлении мультивалютности или скидок.

Отдельно зафиксированные и осознанные упрощения, которые трогать не нужно: глобальная `posLocation` (помечена `ponytail:`, per-restaurant TZ — когда появятся мульти-регионные тенанты), `attachLines` по одному запросу на тикет вне hot path (помечен там же), ETag без кэша, `till`=1/`other_till_shift`=null (v1: одна касса).
