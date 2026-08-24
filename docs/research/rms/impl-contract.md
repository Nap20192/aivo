# Инкремент-1: ledger-ядро + смена до D6 — реализуемый контракт

Рабочий контракт первого инкремента для команды backender / frontender / tester.
Один проход. Опирается на: reference.md (D1–D8, §15 refuted, §16 план),
architecture.md (R1), ddd-architecture.md (агрегаты, долги 1/3/8/10), PLATFORM.md
(конвенции API). Всё, что не описано здесь как «делаем», — вне инкремента.

Инкремент-1 = R1 «Деньги и живые события» + под него подложенное **настоящее
GL-ядро** (D1/D4/D7), потому что Z-отчёт и приёмка смены без реального ленджера —
это опять «вариансы полем», ровно тот анти-паттерн, что запрещает reference §7.

---

## 1. Скоуп и анти-скоуп

### В скоупе

1. **Ledger-ядро** (новый контекст `internal/ledger`) по D1/D4/D7:
   - append-only GL: проводки не редактируются и не удаляются; коррекция — только
     сторно-документ (reversal), датированный текущим днём (D1);
   - документ проводки с жизненным циклом **draft → posted → cancelled** и явным
     гейтом постинга (D4); до постинга GL не трогается, отмена — сторнирует;
   - **две даты** на каждом документе: `accounting_date` (бизнес-дата факта) +
     `recorded_at` (реальный таймстемп записи) — D7;
   - **фиксированный набор измерений**: `restaurant_id` (единица/локация) +
     `cost_center_id` (см. §2, решение по reference §16.1). Открытых измерений
     нет (анти-паттерн §14.1);
   - одностороннние строки (каждая строка — либо debit, либо credit, сумма
     дебетов = сумме кредитов по документу), авто-балансирующая строка на
     «unassigned/rounding» счёт как предохранитель (reference §2);
   - план счётов (seed) + конфиг-маппинг «назначение → счёт» как **точка
     настройки GL-семантики per-restaurant** (refuted §15.6).

2. **Оплаты/тендеры на тикете** (расширение `pos`): тендеры с
   **payment-group GL-семантикой** (cash/card/gift_card/comp/void/house_account),
   записываются при закрытии тикета.

3. **Кассовые движения**: pay-in / pay-out / drop внутри смены (reference §7 —
   необходимые примитивы, их отсутствие названо реальным пробелом).

4. **Смена до D6**: **Open → Closed → Accepted**.
   - `Close` — считает expected по новой формуле (float + нал-тендеры + pay-in −
     pay-out − drop), собирает **draft-журнал** приёмки в ledger;
   - **shift-acceptance документ** — бэк-офис построчно ревьюит draft, может
     переназначить счёт/cost-center на строке (override), затем `Accept` постит
     журнал → GL-факт (reference §5, §7 — human-in-the-loop контроль до постинга);
   - **вариансы кассы — проводка, а не поле** (declared − expected → строка на
     shortage/surplus счёт; журнал без неё не балансируется) — refuted §15.5.

5. Долги, гасящиеся здесь по необходимости: **долг 1** (атомарный CloseShift —
   без него нельзя ни payments, ни приёмку), **долг 3** (иммутабельность
   закрытого тикета — guard'ы `status='open'`), **долг 8, частично** (новый
   контекст `ledger` рождается сразу в целевой структуре `internal/ledger/{domain,
   app,ports,adapters}`, а не мигрирует потом).

### Анти-скоуп (жёстко не тащим)

- **Склад / repost / COGS / инвентаризация** — нет. GL-ядро готово принимать
  stock-проводки позже, но их источника в инкременте нет.
- **Закупки / AP / поставщики** — нет.
- **Рецепты / тех-карты / себестоимость** — нет.
- **Снапшот закрытия периода (D8)** — нет; только **заглушка-гейт**:
  функция `periodOpen(restaurantID, date) bool`, в инкременте всегда `true`.
  Точка расширения зафиксирована, снапшот-документ — позже.
- **Мультивалюта** — нет; single-currency (база компании). Все amount-колонки —
  `bigint` cents, помечены комментарием `-- single currency (company base);
  multicurrency deferred — reference §16.4`. Колонки валюты **не заводим**, но
  имена/типы готовы к её добавлению (§16.4).
- **Скидки на тикете / лояльность / gift-card эмиссия** — нет (это R7). Группа
  `gift_card`/`comp`/`house_account` в маппинге **объявлена** (чтобы GL-контракт
  был полным и refuted §15.6 тестировался), но эмиссия/баланс карт — не строим.
- **Bank reconciliation (deposit → undeposited funds)** — нет; счёт
  «undeposited funds» в плане есть как заготовка, чейн — позже.
- **6-ролевая матрица прав** — нет; приёмка/ledger гейтятся существующей ролью
  `manager+` (owner/manager) из platform-auth. Матрица 6 ролей — отдельный слайс.
- **Kafka / relay** — нет; события (если пишем) — только в outbox in-process.

---

## 2. Контекст `internal/ledger`

Отдельный контекст обоснован: GL — самостоятельный bounded context с
собственными инвариантами (append-only, баланс, гейт), потребляемый pos через
синхронный порт. Живёт в целевой структуре сразу (долг 8):
`internal/ledger/{domain,app,ports,adapters/postgres}`.

### Решение по измерениям (reference §16.1)

Два фиксированных измерения на каждой GL-строке:
- **`restaurant_id`** — единица/локация; всегда = тенант проводки (в GL-строку
  кладём явно, не полагаемся на джойн);
- **`cost_center_id`** — плоский справочник per-restaurant, seed = один центр
  `main`. В инкременте существует только `main`; модель готова к split
  kitchen/bar/front **без изменения схемы** (просто добавятся строки в
  `cost_centers`). Дерева нет, allocation-движка нет (анти-паттерн §14.2,
  reference §10 — «shallow до именованного требования»).

### Агрегаты и инварианты

**Account** (справочник плана счётов; лёгкий агрегат/reference).
Поля словами: id, restaurant_id, code, name, type
(`asset|liability|revenue|expense|equity|statistical`), normal_side
(`debit|credit`), postable(bool), created_at.
Инварианты: код уникален в пределах ресторана; проводки принимает только
`postable` лист-счёт; `type`/`normal_side` **заморожены** после первой проводки
на счёт (проверка в app/store, не UI). Warehouse-счётом GL не подменяем
(reference §2).

**JournalDocument** (aggregate root — документ проводки).
Поля: id, restaurant_id, kind (`shift_acceptance|manual|reversal`), state
(`draft|posted|cancelled`), accounting_date (date, D7), recorded_at (timestamptz,
D7), posted_at, cancelled_at, source_kind (`shift|manual|null`), source_id,
reversal_of (uuid, nullable — на какой документ это сторно), created_by, lines
[]JournalLine.
Инварианты:
- **баланс**: Σ debit = Σ credit по строкам перед постингом; дисбаланс/округление
  добирается авто-строкой на счёт назначения `rounding_unassigned` (reference §2 —
  предохранитель, а не ошибка);
- **гейт D4**: править можно только `draft`; `post()` — единственный переход в
  `posted`; `posted` иммутабелен (ни edit, ни delete строк/шапки);
- **отмена = сторно** (D1): `cancel()` не мутирует оригинал, а порождает новый
  документ kind=`reversal`, reversal_of=orig.id, строки зеркальны (debit↔credit),
  `accounting_date` = **текущий день** (revalidate at current date — refuted §15.1),
  оригинал → state=`cancelled`;
- **две даты** обязательны (D7);
- **измерения** обязательны на каждой строке (restaurant_id + cost_center_id);
- **постинг проверяет `periodOpen(restaurantID, accounting_date)`** — в инкременте
  заглушка `true`; **сторно ревалидируется на текущей дате, не на дате оригинала**
  (refuted §15.1 — механизм закрытия не блокирует собственную отмену).

**JournalLine** (entity внутри документа).
Поля: id, document_id, account_id, side (`debit|credit`), amount_cents (bigint ≥ 0,
single-currency маркер §16.4), cost_center_id, memo, seq.
Инвариант: строго односторонняя (debit XOR credit, amount > 0). После постинга —
append-only, не меняется никогда (refuted §15.2/§15.3 — единственный путь
изменения posted-факта = сторно).

### Точка настройки GL-семантики (refuted §15.6)

**`ledger_account_map`**: restaurant_id, purpose (text), account_id. Seed на
провижининге ресторана, редактируется через API. `purpose` из фиксированного
набора: `sales_revenue`, `cash_drawer`, `cash_over_short`, `cash_movement`,
`rounding_unassigned`, и по группам тендеров — `tender:cash`, `tender:card`,
`tender:gift_card`, `tender:comp`, `tender:house_account`. Смена маппинга меняет
результат постинга — это и есть per-deployment конфиг GL-трактовки из §15.6
(в отличие от «фиксированного свойства системы»).

### Таблицы (имена + ключевые колонки словами; DDL — забота backender)

`migrations/ledger/0001_init.up.sql`:
- **accounts** — см. Account. Индексы: `UNIQUE(restaurant_id, code)`;
  `(restaurant_id)`.
- **cost_centers** — id, restaurant_id, code, name. `UNIQUE(restaurant_id, code)`.
  Seed `main`.
- **journal_documents** — см. JournalDocument. Индексы: `(restaurant_id,
  accounting_date)`; `(source_kind, source_id)`.
- **journal_lines** — см. JournalLine. Индекс `(document_id, seq)`; `(account_id)`.
- **ledger_account_map** — restaurant_id, purpose, account_id.
  `UNIQUE(restaurant_id, purpose)`.

### Partial unique индексы (ledger)

- `journal_documents`: **один «живой» документ на источник** —
  `UNIQUE(source_kind, source_id) WHERE state <> 'cancelled' AND source_kind =
  'shift'`. Гарантирует идемпотентность приёмки: смена постит не более одного
  журнала (refuted §15.2, контракт идемпотентности §16.5). Повторный accept →
  конфликт → 409.

---

## 3. Расширение контекста `pos` (что расширяем / что заменяем)

Совместимость с существующим pos — обязательна. Явно:

**РАСШИРЯЕМ:**
- `pos.Shift`: добавляем состояние **Accepted** — колонки `accepted_at`,
  `accepted_by`, `journal_document_id` (ссылка на draft→posted журнал). Индекс
  «одна открытая на ресторан» — сохраняем; **добавляем** «одна открытая на
  кассира»: `UNIQUE(restaurant_id, opened_by) WHERE closed_at IS NULL` (reference
  §5/§7 — one open shift per till *and per cashier*).
- `pos.Ticket`: добавляем явное **закрытие с тендерами** (`Close(tenders)`),
  колонку `closed_at`, и **guard'ы иммутабельности** (долг 3): `FireTicket`,
  `AppendTicketNote`, `LinkTicketCustomer`, `AddLines` получают `AND status =
  'open'` + проверку `RowsAffected`.
- `GET /api/v1/pos/state`: объект смены получает `state`
  (`open|closed|accepted`), `expected_cents` пересчитывается по новой формуле,
  добавляется разбивка тендеров для Z-отчёта (см. §4).

**ЗАМЕНЯЕМ:**
- **Формулу `Shift.Close`** (сейчас `domain/pos/domain.go:41`): было «expected =
  float + сумма тоталов тикетов, все продажи считаются налом». Стало:
  `expected_cash = opening_float + Σ cash-тендеров + Σ pay_in − Σ pay_out − Σ drop`.
  Карта/безнал в drawer не попадает. `variance = declared − expected_cash`.
  Поля `expected_cents/declared_cents/variance_cents` на строке смены —
  **остаются как display-кэш**, но источник истины варианса — GL-строка на
  `cash_over_short` (refuted §15.5).
- **App-флоу `CloseShift`** (`pos/app/app.go:127` — гонка потери денег, долг 1):
  было несколько store-вызовов вне транзакции. Стало — **одна транзакция**:
  lock смены → закрыть остаточные тикеты → собрать тендеры и кассовые движения →
  `Shift.Close` → построить **draft-журнал** через `ports.Ledger` → записать всё
  атомарно. Формула expected пишется **один раз** (в домене), не дублируется.

**СОХРАНЯЕМ без изменений:** схему `tickets`/`ticket_lines`, `AddLines`, `Fire`,
handoff, menubridge, seed. Роуты pos по-прежнему в
`internal/platform/adapters/http` (как сейчас) — новые pos/ledger хендлеры туда же,
это существующая конвенция размещения, не трогаем.

### Новые таблицы/колонки pos

`migrations/pos/0004_payments_acceptance.up.sql`:
- **payment_methods** — id, restaurant_id, code, name, payment_group
  (`cash|card|gift_card|comp|void|house_account`), active(bool).
  `UNIQUE(restaurant_id, code)`. Seed: `cash`, `card`.
- **ticket_payments** — id, ticket_id, restaurant_id, method_id, payment_group
  (снапшот группы на момент оплаты), amount_cents (bigint, §16.4), tip_cents,
  recorded_at, recorded_by. Индекс `(ticket_id)`.
- **cash_operations** — id, shift_id, restaurant_id, kind (`pay_in|pay_out|drop`),
  amount_cents (bigint, §16.4), reason, recorded_by, recorded_at. Индекс
  `(shift_id)`.
- **shifts** ALTER: `accepted_at timestamptz`, `accepted_by uuid`,
  `journal_document_id uuid` (без FK — кросс-контекст, как `tickets.customer_id`).
- **tickets** ALTER: `closed_at timestamptz`.

Partial unique (pos): «одна открытая смена на кассира» (см. выше);
«один открытый тикет на стол» — сохраняем.

### Порт `pos → ledger` (синхронный, D-правило «нужен ответ другого контекста»)

`pos/ports.Ledger`:
- `AccountForPurpose(ctx, restaurantID, purpose) (accountID, error)` — резолв
  маппинга при сборке draft;
- `BuildDraftShiftJournal(ctx, tx, restaurantID, draft) (docID, error)` — создать
  draft-документ приёмки в GL в **той же транзакции** (общий `*sql.Tx`, обе
  таблицы в одной Postgres — монолит, кросс-контекстная транзакция допустима и
  задокументирована здесь как разрешённая);
- `PostJournal(ctx, tx, restaurantID, docID) error` — постинг при Accept;
- `CancelJournal(ctx, restaurantID, docID) (reversalID, error)` — сторно.

Реализация порта — адаптер `pos/adapters/ledgerbridge` поверх `ledger/app`.

---

## 4. API `/api/v1` (конвенции PLATFORM.md)

Деньги — integer cents. IDs — uuid strings. Ошибки —
`{"error":{"code","message"}}`, коды 401/403/404/409/422. Тенант выводится из
сессии, не из тела.

### POS (сессия, waiter+)

**POST `/api/v1/pos/tickets/{id}/close`**
Закрыть тикет с тендерами.
```
req:  {"payments":[{"method_id":uuid,"amount_cents":int,"tip_cents":int}]}
resp: {"ticket":{id,status:"closed",closed_at,total_cents,
                 payments:[{method_id,payment_group,amount_cents,tip_cents}]}}
```
422 `tenders_mismatch` если Σ amount ≠ total (кроме группы `void` — закрытие без
оплаты); 409 `ticket_closed` если уже закрыт; 409 `no_open_shift`.

**POST `/api/v1/pos/shifts/{id}/cash-operations`**
Внесение/изъятие/инкассация.
```
req:  {"kind":"pay_in|pay_out|drop","amount_cents":int,"reason":string}
resp: {"cash_operation":{id,kind,amount_cents,reason,recorded_at}}
```
422 `invalid_amount` (≤0); 409 `shift_not_open`.

**POST `/api/v1/pos/shifts/{id}/close`**  (заменяет текущую семантику)
```
req:  {"declared_cents":int}
resp: {"shift":{id,number,state:"closed",
                expected_cents,declared_cents,variance_cents,
                closed_at,journal_document_id}}
```
Атомарно: закрывает остаточные тикеты, считает expected, собирает **draft**-журнал.
409 `open_tickets_unpaid` если есть открытые тикеты со строками без оплаты
(официант обязан рассчитать; пустые тикеты авто-закрываются как void).

**GET `/api/v1/pos/shifts/{id}/z-report`**
Разбивка для кассира (display).
```
resp: {opening_float_cents, tenders:[{payment_group,amount_cents,tip_cents}],
       cash_operations:[{kind,amount_cents}], expected_cash_cents,
       declared_cents, variance_cents, state}
```

`GET /api/v1/pos/state` — объект `shift` дополняется полем `state`.

### Приёмка смены (сессия, manager+, restaurant-scoped)

**GET `/api/v1/restaurants/{id}/shifts?state=closed|accepted`** — список смен.
```
resp: [{id,number,cashier,opened_at,closed_at,accepted_at,state,
        expected_cents,declared_cents,variance_cents}]
```

**GET `/api/v1/restaurants/{id}/shifts/{shift_id}/acceptance`**
Draft-журнал приёмки для ревью.
```
resp: {shift:{...}, document:{id,state:"draft",accounting_date,recorded_at,
       lines:[{line_id,account_id,account_code,account_name,side,amount_cents,
               cost_center_id,memo,editable:bool}]},
       variance_cents, balanced:bool}
```

**PATCH `/api/v1/restaurants/{id}/shifts/{shift_id}/acceptance`**
Override назначений (только пока `draft`) — reference §5/§7, refuted §15.2
(контролируемый write-путь в «read-only» ленджер).
```
req:  {"lines":[{"line_id":uuid,"account_id":uuid?,"cost_center_id":uuid?}]}
resp: same shape as GET
```
409 `document_posted` если смена уже принята; 422 `account_not_postable`.

**POST `/api/v1/restaurants/{id}/shifts/{shift_id}/accept`**
Постинг draft → GL-факт, смена → Accepted.
```
resp: {shift:{...,state:"accepted",accepted_at}, document:{id,state:"posted",posted_at}}
```
409 `shift_not_closed` / `already_accepted`; 422 `unbalanced` (не должно
случаться — авто-строка добирает, но контракт явный).

### Ledger back-office (сессия, manager+, restaurant-scoped)

**GET `/api/v1/restaurants/{id}/ledger/accounts`** → план счётов.
```
resp: [{id,code,name,type,normal_side,postable}]
```

**GET/PUT `/api/v1/restaurants/{id}/ledger/account-map`** → конфиг GL-семантики
(§15.6).
```
GET  resp: [{purpose,account_id,account_code}]
PUT  req:  {"map":[{"purpose":string,"account_id":uuid}]}  → same shape
```
422 `unknown_purpose` / `account_not_postable`.

**GET `/api/v1/restaurants/{id}/ledger/journals?from=&to=&account=&source=`**
Список posted-документов, инкрементально по `accounting_date` (обязателен `from`).
```
resp: [{id,kind,state,accounting_date,recorded_at,source_kind,source_id,
        reversal_of,total_cents}]
```

**GET `/api/v1/restaurants/{id}/ledger/journals/{doc_id}`** → документ со строками.
```
resp: {id,kind,state,accounting_date,recorded_at,posted_at,cancelled_at,
       reversal_of,lines:[{account_id,account_code,side,amount_cents,
       cost_center_id,memo}]}
```

**POST `/api/v1/restaurants/{id}/ledger/journals?post=1`** → ручной журнал.
```
req:  {"accounting_date":"YYYY-MM-DD","memo":string,
       "lines":[{"account_id":uuid,"side":"debit|credit","amount_cents":int,
                 "cost_center_id":uuid?,"memo":string?}]}
resp: {document:{...}}   // draft, либо posted при ?post=1
```
422 `unbalanced` (если авто-строка запрещена для ручных — решение: для manual
дисбаланс = ошибка, а не авто-добор), `account_not_postable`, `line_side`.

**POST `/api/v1/restaurants/{id}/ledger/journals/{doc_id}/cancel`** → сторно.
```
resp: {reversal:{id,kind:"reversal",accounting_date:<сегодня>,...},
       original:{id,state:"cancelled"}}
```
409 `already_cancelled` / `not_posted`.

---

## 5. Карта работ

### Backender (Go)

**Ledger — новый контекст:**
- `internal/ledger/domain`: `Account`, `JournalDocument` (AggregateRoot),
  `JournalLine`; методы `NewDocument`, `AddLine`, `Balance()`/авто-unassigned,
  `Post()`, `Cancel()→reversal`, инварианты (баланс, односторонность, freeze
  type, две даты, гейт state-machine). Юнит-тесты домена.
- `internal/ledger/ports`: `Store` (accounts CRUD-read, cost_centers, documents
  draft/post/cancel/list/get, account_map get/put), `ErrNotFound/ErrConflict`.
- `internal/ledger/app`: `PostJournal`, `CancelJournal`, `ManualJournal`,
  `GetJournals/GetJournal`, `AccountMapGet/Put`, `AccountForPurpose`,
  `periodOpen()` **заглушка → true** (точка расширения D8).
- `internal/ledger/adapters/postgres` + sqlc `queries/ledger/ledger.sql`
  (генерат `ledgerdb`), `migrations/ledger/0001_init.*`.
- **Seed при провижининге ресторана**: план счётов (см. §6), `cost_centers.main`,
  `ledger_account_map` дефолты, `payment_methods` cash/card. Хук в
  platform-провижининге либо `ledger/app.SeedRestaurant(tx, restaurantID)`,
  вызываемый оттуда; продублировать в `cmd/aivo-seed`.

**Pos — расширение:**
- `domain/pos`: новая формула `Shift.Close` (float + нал-тендеры + pay_in −
  pay_out − drop); состояние Accepted (`Accept()`); `Ticket.Close(tenders)` +
  guard'ы иммутабельности; типы `Tender`, `CashOperation`. Юнит-тесты формулы и
  guard'ов.
- `pos/ports`: `Ledger` (см. §3); расширить `Store` методами
  `RecordTicketPayments`, `RecordCashOperation`, `TendersForShift`,
  `CashOperationsForShift`, `AcceptShift`, `InTx(ctx, fn)` (unit-of-work на общий
  `*sql.Tx`).
- `pos/app`: `CloseTicket`, `RecordCashOperation`, **`CloseShift` переписать
  атомарно** (долг 1) — одна транзакция, внутри строит draft через `ports.Ledger`
  в том же `tx`; `AcceptShift` (постит через `ports.Ledger.PostJournal` в tx +
  ставит `shifts.accepted_at`); `ZReport`.
- `pos/adapters/ledgerbridge`: реализация `pos/ports.Ledger` поверх `ledger/app`.
- `pos/adapters/postgres`: новые store-методы + guard'ы `AND status='open'` +
  RowsAffected (долг 3); `migrations/pos/0004_*`.

**HTTP (в `platform/adapters/http` — существующая конвенция):**
- pos-хендлеры: ticket close, cash-operation, shift close (новая форма), z-report.
- приёмка: list shifts, acceptance GET/PATCH, accept POST (гейт manager+).
- ledger: accounts, account-map GET/PUT, journals list/get/post, cancel
  (гейт manager+).

**Транзакции — где:**
- `CloseTicket` — 1 tx (payments + tickets.closed_at + guard).
- `CloseShift` — **1 tx** (lock shift → close tickets → aggregate tenders/cashops
  → Shift.Close → BuildDraftShiftJournal). Долг 1.
- `AcceptShift` — **1 tx** (PostJournal: вставка journal_lines + document.state →
  posted + shifts.accepted_at + journal_document_id). Идемпотентность —
  partial-unique «один живой документ на смену» ловит гонку двойного accept → 409.
- `CancelJournal` — 1 tx (reversal insert + original.state → cancelled).
- `ManualJournal(post=1)` — 1 tx.

**События (events-этап 0, опционально в инкременте):** при постинге писать в
outbox `events` в той же tx `JournalPosted` / `ShiftAccepted` (плоский payload).
Паблишер не поднимаем. Если не успеваем — не блокер; но колонку/запись outbox не
ломаем. Правки `EVENTS.md` — только для реально проводимых событий (долг 10).

### Frontender

**`frontend/pos`:**
- Экран **закрытия тикета**: ввод тендеров (cash/card + tip), «сдача» = Σ cash −
  total, кнопка close.
- Модалка **cash in/out** (pay_in/pay_out/drop).
- Экран **закрытия смены**: подсчёт наличных → `declared_cents`, превью варианса.
- **Z-отчёт**: разбивка по группам тендеров, кассовые движения, expected/variance.
- Типы (`frontend/pos/src/types.ts`): `Tender`, `PaymentGroup`, `CashOperation`,
  `ShiftClose`, `ZReport`; `Shift.state`.

**`frontend/admin`:**
- **Очередь приёмки смен** (список closed).
- **Детальная приёмка**: строки draft-журнала, дропдауны override
  account/cost-center, вариант подсвечен, кнопка Accept.
- **Ledger**: план счётов (read), редактор account-map, список журналов +
  детальный документ, форма ручного журнала, cancel/reversal.
- Типы (`frontend/admin/src/api/types.ts`): `Account`, `JournalDocument`,
  `JournalLine`, `AccountMapEntry`, `ShiftAcceptance`, `CostCenter`.

### Tester (интеграционные тесты)

**Шесть refuted assumptions (reference §15) как тест-кейсы под скоуп:**
1. **§15.1 — закрытие не блокирует свою же отмену; сторно ревалидируется на
   текущей дате.** Запостить журнал с прошлой `accounting_date`; отменить →
   reversal с `accounting_date = сегодня`, оригинал `cancelled`. (Гейт-заглушка
   не мешает сторно.)
2. **§15.2 — «read-only» ленджер имеет узкий контролируемый write-путь.**
   Override строк доступен **только на draft** (PATCH acceptance); после Accept
   PATCH → 409; posted-строки не меняются. Повторный Accept → 409 (одна живая
   проводка на смену).
3. **§15.3 — ни один путь не мутирует posted-факт.** Попытка edit posted-документа
   → отказ; коррекция только через reversal (append-only держится).
4. **§15.4 — дата консолидации детерминирована, не берётся «от последнего».**
   `accounting_date` журнала смены = бизнес-дата закрытия смены, а **не**
   таймстемп последнего тикета/оплаты.
5. **§15.5 — вариант — обязательная проводка, а не мягкое поле.** Close с
   declared ≠ expected ⇒ в журнале есть строка на `cash_over_short`; без неё
   журнал не балансируется. При declared = expected такой строки нет.
6. **§15.6 — GL-трактовка тендера — конфиг, не фиксированное свойство.** Сменить
   `account-map` для `tender:cash` ⇒ тот же нал постится на новый счёт. Два
   ресторана с разным маппингом ⇒ нал уходит на разные счета.

**Базовые инварианты:**
- Баланс: Σ debit = Σ credit на каждом posted-документе; авто-unassigned добирает
  дисбаланс (для shift-journal), для manual дисбаланс = 422.
- Односторонность строки (debit XOR credit, amount > 0).
- State-machine draft→posted→cancelled; нет edit после post.
- Две даты присутствуют на каждом документе.
- **CloseShift атомарен** (долг 1): конкурентное закрытие не даёт двойной постинг;
  смена иммутабельна после Accept.
- **Иммутабельность закрытого тикета** (долг 3): fire/add-lines/note/link на
  closed → отказ (RowsAffected=0 → ошибка).
- Формула expected = float + нал-тендеры + pay_in − pay_out − drop (карта в drawer
  не идёт).
- Тенант-изоляция: ресторан A не читает журналы/счета/смены ресторана B.

---

## 6. Seed плана счётов (дефолт per-restaurant)

Минимальный план (code — name — type — normal_side — postable):
- `1000` Cash on hand (drawer) — asset — debit — yes
- `1010` Card clearing — asset — debit — yes
- `1020` Undeposited funds — asset — debit — yes *(заготовка под bank rec, не
  используется в инкременте)*
- `1100` House account receivable — asset — debit — yes
- `2000` Gift card liability — liability — credit — yes *(заготовка под R7)*
- `4000` Sales revenue — revenue — credit — yes
- `4900` Comps / contra-revenue — revenue — debit — yes
- `5900` Cash over/short — expense — debit — yes
- `6000` Cash movements (pay in/out) — expense — debit — yes
- `9999` Unassigned / rounding — expense — debit — yes

`ledger_account_map` дефолты: `sales_revenue→4000`, `cash_drawer→1000`,
`cash_over_short→5900`, `cash_movement→6000`, `rounding_unassigned→9999`,
`tender:cash→1000`, `tender:card→1010`, `tender:gift_card→2000`,
`tender:comp→4900`, `tender:house_account→1100`. `void` — маппинга нет (проводок
не создаёт).

Пример постинга журнала смены (нал 10000, карта 5000, вариант −200 недостача):
```
debit  1000 Cash drawer         9800   (ожидаемый нал после варианса)
debit  1010 Card clearing       5000
debit  5900 Cash over/short       200   (недостача — обязательная строка §15.5)
credit 4000 Sales revenue      15000
```

---

## 7. Definition of Done

- `migrations/ledger/0001` + `migrations/pos/0004` применяются; `go build -C
  backend ./... && go vet -C backend ./... && go test -C backend ./...` — зелёные.
- `cmd/aivo-seed` создаёт демо-тенанту план счётов + account-map + cost-center
  `main` + payment_methods.
- **POS**: закрытие тикета с тендерами; pay-in/out/drop; закрытие смены →
  draft-журнал + Z-отчёт.
- **Admin**: очередь приёмки; override account/cost-center на draft; Accept →
  **posted журнал, который балансируется**; список/детали журналов; ручной журнал;
  cancel через reversal.
- Каждый posted-журнал **балансируется**, имеет **две даты** и **измерения**
  (restaurant_id + cost_center_id) на каждой строке.
- Вариант — всегда проводка (`cash_over_short`); маппинг тендеров — конфиг.
- **Атомарность**: CloseShift и Accept — по одной транзакции; двойной accept → 409.
- **Иммутабельность**: закрытый тикет и posted-факт не мутируются (только сторно).
- Все шесть refuted-тестов + инвариант-тесты зелёные.
- Анти-скоуп соблюдён: нет склада/repost/закупок/рецептов/снапшота-периода/
  мультивалюты; period-гейт — заглушка `true`; amount-колонки помечены §16.4.
- Обновлены: `PLATFORM.md` (новые эндпоинты), `EVENTS.md` (если события проведены),
  `CONTEXT.md` (новый контекст ledger + решения по измерениям/маппингу).

---

## Отклонения (backender, инкремент-1)

Где реализация минимально отступила от буквы контракта — и почему.

1. **Размещение домена.** §2 говорит `internal/ledger/domain`, но фактическая
   конвенция репо (и комментарий `sharedkernel`) — домен в `internal/domain/<ctx>`.
   Домен ledger лежит в `internal/domain/ledger`, а `app/ports/adapters` — в
   `internal/ledger/*` (как у pos/menu/platform). Совместимость с pos обязательна,
   поэтому следовал реальной структуре.

2. **sqlc vs ручной SQL.** Блок `ledger` в `sqlc.yaml` + `queries/ledger/ledger.sql`
   добавлены и генерируют `ledgerdb` (как просили). Но адаптеры написаны вручную на
   `database/sql` — это настоящая конвенция репо: `menudb`/`posdb`/`platformdb` тоже
   сгенерированы, но адаптеры их **не используют**. Ручной SQL к тому же делает общий
   кросс-контекстный `*sql.Tx` тривиальным. `ledgerdb` — сгенерирован-но-не-используется,
   ровно как остальные.

3. **Кассовые движения не проводятся в GL.** pay_in/pay_out/drop записываются и
   участвуют в формуле expected и Z-отчёте, но **не** постятся отдельными GL-строками
   в инкременте (маппинг `cash_movement→6000` засеян на будущее). Журнал смены =
   строки тендеров (нал скорректирован вариансом) + `cash_over_short` + кредит
   `sales_revenue` + авто-строка. Баланс и все шесть refuted-тестов сохранены; точность
   отражения движений в цифре кассы — задокументированный потолок.

4. **Чаевые не проводятся.** `ticket_payments.tip_cents` хранится и показывается в
   Z-отчёте, но не постится в GL (нет счёта tip-liability в инкременте).

5. **Freeze типа/стороны счёта.** Инвариант «type/normal_side заморожены после первой
   проводки» держится **по отсутствию пути мутации**: в §4 нет эндпоинта редактирования
   счёта, менять нечему. Метод `AccountHasPostings` убран как мёртвый код — вернуть
   вместе с эндпоинтом правки счёта.

6. **Порт pos→ledger сужен.** `AccountForPurpose`/`CancelJournal` из списка §3 — это
   back-office операции ledger (не часть pos-транзакции); они вызываются напрямую у
   `ledger/app` из HTTP-слоя. Порт `pos/ports.Ledger` несёт только
   `BuildDraftShiftJournal` + `PostJournal` (то, что реально нужно в транзакциях pos).
   Добавлен `ledger` метод `LiveDocumentBySource`/`LiveDocumentForShift`, чтобы хендлеры
   приёмки находили черновик журнала смены.

7. **Хук провижининга.** ~~Не подключён в живой само-регистрации.~~ **Исправлено в
   QA-волне (M3/BUG-1):** `platform` store получил nil-safe хук `OnProvisionRestaurant`,
   вызываемый внутри той же транзакции, что и вставка ресторана (в `CreateOrgWithOwner`
   и `CreateRestaurant`, рядом с `insertDefaultMenu`). Хук
   (`internal/provisioning.RestaurantProvisioner`) сеет план счётов + cost-center + map
   (`ledger.SeedRestaurantTx` на общий `*sql.Tx`) и payment_methods cash/card
   (`pospg.SeedDefaultPaymentMethods`). Подключён в `cmd/aivo-server` и `cmd/aivo-seed`
   (дубль-seed из aivo-seed убран). Само-регистрированный ресторан теперь полностью
   провижинится атомарно.

8. **Код ошибки «нет открытой смены».** §4 для закрытия тикета указывает
   409 `no_open_shift`; введён отдельный `ErrShiftNotOpen` → 409 `shift_not_open` и для
   закрытия тикета, и для кассовых операций (единый 409), а не переиспользован
   существующий 422 `no_open_shift` от add-lines. Мелкая разница в строке кода.

9. **`GET /pos/state`.** Объект смены теперь считает `expected_cents` по новой формуле
   (нал-тендеры + движения) и получил поле `state`; убран старый «все продажи — нал»
   running-total и ставший ненужным `ShiftClosedSalesCents`.

10. **События/outbox не реализованы.** §5 помечает их опциональными («если не успеваем —
    не блокер»). Не делал; `EVENTS.md` не трогал — по инструкции контракта править его
    только для реально проводимых событий.

Проверка: `go build/vet/test ./...` зелёные; юнит-тесты домена (ledger: баланс, reversal,
авто-баланс, гейт постинга, state-machine; pos: формула Close, Accept-гейты, Ticket.Close).
Дополнительно прогнан временный интеграционный smoke против живого Postgres (миграции +
seed + пример §6 «нал 10000/карта 5000/вариант −200» → строки 1000 +9800, 1010 +5000,
5900 +200, 4000 −15000, balanced; post; идемпотентность повторной приёмки → конфликт;
cancel→reversal сегодняшней датой + оригинал cancelled; manual balanced/unbalanced;
ремап `tender:cash` §15.6; полный pos-флоу open→ticket→tender→cash-op→close→accept,
двойной accept → конфликт). Smoke удалён после проверки — интеграционные тесты за тестером.

### QA-волна фиксов (после ревью QA + тестера)

Закрыты все находки QA (B1,B2,M1,M2,M3,m1,m2,m3) и e2e (BUG-1..3):

- **B1** — append-only при гонке override×accept. `ReplaceDraftLines` и
  `PostShiftJournal` теперь берут `journal_documents … FOR UPDATE` и перечитывают state
  в одной tx → сериализуются на одной строке. Побочно найден и починен латентный
  self-deadlock: `Store.WithTx` оставлял `pool` ненулевым, из-за чего `ReplaceDraftLines`
  на tx-связанном сторе открывал **вторую** транзакцию (INSERT journal_lines → FK-lock на
  строке, которую первая tx держит FOR UPDATE). `WithTx` теперь возвращает `{q: tx}` (pool
  nil) → DELETE+INSERT идут инлайн по tx вызывающего. Конкурентный тест:
  `TestOverrideRacesAcceptStaysAppendOnly` (25 итераций override vs post, исход всегда
  когерентный, posted-строки не переписываются).
- **B2** — `/pos/state` отдаёт `payment_methods` (активные, из `store.PaymentMethods`).
- **M1** — `acceptanceView.shift` = полный ShiftRow (number/cashier/opened/closed/expected/
  declared/variance) через общий `shiftRowView`.
- **M2** — `GET /restaurants/{id}/ledger/cost-centers`; `OverrideDraftLines` валидирует
  принадлежность cost_center ресторану (422 вместо FK-500). Тест
  `TestOverrideRejectsForeignCostCenter`.
- **M3/BUG-1** — провижининг (см. отклонение №7).
- **m1** — `number` в списке смен = строка `"shift-N"`.
- **m2** — `CloseTicket` сверяет `ticket.ShiftID` с текущей открытой сменой →
  `ErrShiftNotOpen`. Тест `TestCloseTicketRejectsForeignShift`.
- **m3** — `PostedJournals.total_cents` = сумма дебетов = магнитуда сбалансированного
  документа (все posted-документы сбалансированы); соответствует контракту, оставлено.
- **BUG-2** — `TicketByID` выбирает `closed_at`.
- **BUG-3** — `RecordCashOperation` заполняет `RecordedAt` (пишется явно в INSERT).

Проверка волны: `go build/vet/test -C backend ./...` зелёные, включая интеграционные с
`DATABASE_URL` (ledger + pos адаптеры). `npm run build` зелёный в `frontend/admin` и
`frontend/pos`.
