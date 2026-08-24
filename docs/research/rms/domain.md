# RMS-домены: проектирование бизнес-логики (референс — iiko)

Проектный документ. Кода нет и не будет в этом PR — только доменные решения.
Kafka/события и офлайн здесь **не проектируются** (другие агенты); места, где
нужно событие, помечены `→ событие: <Имя>`.

## 0. Вводная

### Что уважаем из существующего

- **Деньги — integer cents** везде. Новые домены не вводят Money-VO.
- **Кросс-агрегатные уникальности держит БД** (partial unique индексы:
  одна открытая смена, один открытый тикет на стол и т.д.). Новые
  инварианты того же класса — тоже в БД, не в агрегатах.
- **Кросс-контекстные ссылки — без FK** (`tickets.customer_id` уже так).
  Новые контексты ссылаются на чужие ID голыми uuid-колонками.
- **Каждая tenant-таблица несёт `restaurant_id`**, каждый запрос фильтрует по нему.
- Миграции — по контекстам в `backend/migrations/<context>/`; порядок батчей
  фиксирован (menu → platform → pos → новые). `CREATE TABLE` между батчами
  не переносим.
- Правило интеграции из ddd-architecture.md §4: *нужен ответ другого
  контекста внутри use case — синхронный порт; другой лишь реагирует на
  факт — событие. Никогда оба.*

### Новая величина: количество

Склад не живёт в целых штуках — граммы, миллилитры, доли порции. Вводим
одно решение на все новые домены: **количества — `NUMERIC(12,3)` в базовой
единице позиции** (g / ml / pcs), в Go — миллиединицы `int64` («миллиграммы»)
по аналогии с cents. Деньги остаются integer cents; там, где себестоимость
партии делится на количество, храним **стоимость партии целиком** (cents) и
делим при расчёте с банковским округлением — цену за единицу не храним как
дробь.

### Карта контекстов после расширения

```
Platform (тенантность, auth, роли, лимиты)
  ├─ Menu ── POS ── Kitchen (внутри POS)
  ├─ Inventory (склад + закупки)   ← события продаж из POS
  ├─ Workforce (расписания, табель, мотивация)
  ├─ Loyalty (баллы, промо)        ↔ POS синхронно при оплате
  └─ Finance (P&L — read model поверх остальных + свои расходы)
```

Шесть требований — четыре новых контекста и два расширения существующих.
Обоснования — в разделах.

---

## 1. Склад и себестоимость — новый контекст `inventory`

### Почему отдельный bounded context

Язык другой: «номенклатура», «партия», «накладная», «списание», «выход» —
ни одно из этих слов не имеет смысла в menu/pos. Единственная связь с
menu — тех-карта ссылается на `menu_item_id` (проекция, без FK), с pos —
реакция на факт продажи. Жизненные циклы независимы: меню меняется
ежедневно, номенклатура — стабильна. Классический supporting-контекст,
downstream от menu (conformist: читает id и имена позиций) и от pos
(только события).

### Убиквитарный язык

| Термин | Значение |
|---|---|
| StockItem (номенклатура) | То, что лежит на складе: сырьё («мука»), полуфабрикат («тесто»), товар для перепродажи («кола 0.33») |
| TechCard (тех-карта) | Рецепт: чему в номенклатуре соответствует продажа menu_item — ингредиенты (брутто) и выход |
| Batch (партия) | Приход по одной цене; носитель FIFO-себестоимости |
| StockMovement | Строка книги движений: приход / списание / корректировка |
| GoodsReceipt | Приходная накладная от поставщика |
| WriteOffAct | Акт списания (порча, бой, питание персонала) |
| Stocktake | Инвентаризация: пересчёт → акт расхождений |

### Агрегаты

**StockItem** — корень: StockItem.
- Поля: sku, name, unit (`g`|`ml`|`pcs`), type (`ingredient`|`prepared`|`retail`), min_stock (для алертов), archived.
- Инварианты: unit неизменяем после первого движения (иначе вся книга
  движений теряет смысл); архивация вместо удаления, если были движения.
- Уникальность sku в ресторане — unique индекс `(restaurant_id, sku)` (БД).

**TechCard** — корень: TechCard; граница: TechCard + TechCardLine[].
- Привязка: `menu_item_id uuid` (без FK, чужой контекст) — «продажа этой
  позиции меню списывает вот это».
- Строки: stock_item_id, gross_qty (брутто), опционально net_qty. Yield
  (выход, г) — информационное поле для полуфабрикатов.
- Инварианты (в агрегате): ≥1 строки при активации; gross_qty > 0;
  один stock_item — одна строка (unique `(tech_card_id, stock_item_id)` — БД).
- **≤1 активной тех-карты на menu_item** — partial unique
  `(restaurant_id, menu_item_id) WHERE active` (БД, как open shift).
- Версионирование (себестоимость «на дату продажи» по исторической карте) —
  НЕ в MVP: списание фиксирует снапшот количеств в movement, этого хватает.
- Полуфабрикаты (тех-карта производит stock_item, который сам ингредиент
  другой карты) — post-MVP; в MVP карта только «расшифровывает» продажу.

**Batch** — не самостоятельный агрегат, а строка внутри границы
StockLedger (см. ниже): создаётся приходом, уменьшается списаниями.
`qty_remaining >= 0` — CHECK в БД.

**GoodsReceipt** — корень: GoodsReceipt; граница: + GoodsReceiptLine[].
- Статусы: `draft → posted`. Проведение (post) атомарно создаёт партии
  и движения прихода; проведённая накладная иммутабельна (паттерн
  `Shift.Close` — лучший в системе, копируем: UPDATE ... WHERE status='draft'
  + RowsAffected). Сторно — отдельным документом, не правкой.
- Строки: stock_item_id, qty, **total_cost_cents** (за строку, не за
  единицу — см. §0), supplier_id?.
- → событие: `GoodsReceiptPosted` (для Finance: кредиторка/расход).

**WriteOffAct** — корень: WriteOffAct; граница: + строки.
- reason: `spoilage` | `staff_meal` | `loss` | `other` + note.
- Тоже `draft → posted`, иммутабелен после.
- → событие: `StockWrittenOff`.

**Stocktake** — корень: Stocktake; граница: + StocktakeLine[].
- Статусы: `draft → counting → posted`. **≤1 незакрытой инвентаризации на
  ресторан** — partial unique `(restaurant_id) WHERE status <> 'posted'` (БД).
- Строки: stock_item_id, counted_qty; expected_qty фиксируется в момент
  post (не при создании — иначе продажи во время пересчёта дают ложную
  недостачу; iiko делает так же «на момент проведения»).
- Post атомарно создаёт движения surplus/shortage на дельты.
- → событие: `StocktakePosted` (Finance: недостачи в P&L).

### FIFO-себестоимость

Механика — целиком в app-слое inventory, одна транзакция на списание:

1. Списание X единиц stock_item → выбрать партии
   `WHERE qty_remaining > 0 ORDER BY received_at, id` с `FOR UPDATE`.
2. Гасить партии по порядку; стоимость доли партии =
   `round(batch_cost_cents * consumed/initial)`; последняя доля партии
   получает остаток стоимости (чтобы сумма долей == стоимости партии,
   копейки не теряются).
3. Движение фиксирует `qty` и `cost_cents` — книга движений самодостаточна
   для food cost без пересчёта партий задним числом.

**Отрицательные остатки разрешены** (позиция iiko: продажу нельзя
блокировать складом — официант не должен видеть «нельзя продать борщ,
кладовщик не провёл накладную»). Когда партий не хватает: остаток
списывается по цене последней известной партии (или 0, если приходов не
было), движение помечается `estimated = true`. Следующий приход НЕ
пересчитывает прошлое (просто и предсказуемо); точность возвращает
инвентаризация. Пометить в отчёте food cost долю estimated-движений.

### Списание по продаже

Продажа — факт POS; складу не нужен ответ внутри use case оплаты ⇒
**только событие**, синхронного порта pos→inventory нет вообще.

- → событие: `TicketClosed` (уже в EVENTS.md; payload надо обогатить
  строками `[{menu_item_id, qty}]` либо консюмер перечитывает
  ticket_lines — по конвенции EVENTS.md «consumers re-read state»).
- Консюмер в inventory: для каждой строки найти активную тех-карту →
  FIFO-списание; позиция без тех-карты — пропуск (retail-товары получают
  тривиальную карту «1 шт себя самого» автоматически при связывании).
- Идемпотентность по `event_id` — движения несут `source_event_id`,
  unique индекс. (Механику доставки проектирует агент по событиям.)

### Use cases

| Use case | Актор | Суть |
|---|---|---|
| CreateStockItem / Archive | кладовщик+ | номенклатура |
| UpsertTechCard / Activate | менеджер | рецепт для позиции меню |
| CreateReceipt → PostReceipt | кладовщик | приход, создаёт партии |
| CreateWriteOff → Post | кладовщик | порча/питание персонала |
| StartStocktake → EnterCounts → Post | кладовщик+менеджер | инвентаризация |
| ConsumeForSale (внутренний) | событие TicketClosed | FIFO-списание по тех-карте |
| StockOnHand | менеджер | остатки = Σ движений (или Σ qty_remaining партий) |
| FoodCostReport | менеджер | по периоду: выручка позиции (из pos) vs Σ cost_cents списаний |

### Таблицы (`migrations/inventory/`)

```sql
stock_items      (id, restaurant_id, sku, name, unit, type, min_stock numeric(12,3),
                  archived bool; UNIQUE (restaurant_id, sku))
tech_cards       (id, restaurant_id, menu_item_id uuid /*без FK*/, yield_qty, active bool,
                  updated_at; PARTIAL UNIQUE (restaurant_id, menu_item_id) WHERE active)
tech_card_lines  (id, tech_card_id FK, stock_item_id FK, gross_qty numeric(12,3);
                  UNIQUE (tech_card_id, stock_item_id))
goods_receipts   (id, restaurant_id, supplier_id uuid NULL, status, note, posted_at, posted_by)
goods_receipt_lines (id, receipt_id FK, stock_item_id FK, qty, total_cost_cents int)
stock_batches    (id, restaurant_id, stock_item_id FK, receipt_line_id FK NULL,
                  received_at, qty_initial, qty_remaining CHECK (qty_remaining >= 0),
                  cost_cents int)  -- стоимость всей партии
stock_movements  (id, restaurant_id, stock_item_id FK, kind /*receipt|sale|writeoff|
                  stocktake_surplus|stocktake_shortage*/, qty signed, cost_cents int,
                  estimated bool, doc_id uuid, source_event_id uuid NULL UNIQUE, created_at)
write_off_acts   (id, restaurant_id, reason, note, status, posted_at, posted_by)
write_off_lines  (id, act_id FK, stock_item_id FK, qty)
stocktakes       (id, restaurant_id, status, started_at, posted_at, posted_by;
                  PARTIAL UNIQUE (restaurant_id) WHERE status <> 'posted')
stocktake_lines  (id, stocktake_id FK, stock_item_id FK, counted_qty,
                  expected_qty NULL /*фиксируется при post*/)
```

Остаток = `SUM(stock_movements.qty)` по позиции; денормализованный кэш
остатков — только если отчёт по остаткам станет медленным (не в MVP).

Мульти-склады (кухня/бар отдельно, как в iiko) — **не в MVP**: одна
колонка `storage_id` добавляется позже без ломки модели; пока весь
ресторан — один склад.

### MVP / потом

- **MVP:** номенклатура, тех-карты (без полуфабрикатов), приходные
  накладные, ручные списания, инвентаризация, FIFO-книга движений,
  списание по TicketClosed, отчёты «остатки» и «food cost».
- **Потом:** полуфабрикаты (вложенные тех-карты), версии тех-карт,
  мульти-склады и перемещения между ними, алерты по min_stock,
  авто-заявка поставщику от min_stock.

---

## 2. Закупки — внутри контекста `inventory`

### Почему не отдельный контекст

Язык общий (поставщик, накладная, цена за единицу номенклатуры), данные
общие (те же stock_items), актор тот же (кладовщик/менеджер). Отдельный
BC дал бы дублирование номенклатуры и ACL между «закупками» и «складом»
без единой причины: сложного тендерного workflow, отдельной команды или
интеграций с EDI у нас нет. iiko тоже держит это одним модулем
(iikoOffice «Товары и склады»). Выделим, только если появится
централизованная закупка на организацию (несколько ресторанов → один
закупщик) — тогда это org-scoped контекст. Пока — подпапка домена.

### Агрегаты

**Supplier** — корень: Supplier. Поля: name, contacts (jsonb, как
`restaurants.contacts`), payment_terms note, archived. Уникальность
имени в ресторане — unique `(restaurant_id, lower(name))` (БД).

**SupplierPrice** — не агрегат, а запись прайса: `(supplier_id,
stock_item_id, price_cents за базовую единицу, valid_from)`. История цен
append-only; актуальная цена = последняя по valid_from. Инвариант «одна
цена на дату» — unique `(supplier_id, stock_item_id, valid_from)` (БД).
Обновляется двумя путями: вручную и автоматически при post накладной
(цена строки / qty → новая запись прайса).

**PurchaseOrder (заявка)** — корень: PurchaseOrder; граница: + строки.
- Статусы: `draft → sent → partially_received → closed | cancelled`.
- Инварианты: строки редактируемы только в draft; закрытие — терминально.
- Приёмка: GoodsReceipt ссылается на `purchase_order_id NULL` — приход
  «по заявке» подставляет строки заявки как черновик накладной,
  расхождения (привезли меньше/дороже) остаются видимыми: заявка хранит
  ожидание, накладная — факт. Сверка = запрос по двум таблицам, отдельного
  агрегата «приёмка» не нужно.
- → событие: `PurchaseOrderSent` (потом: email/EDI поставщику — не сейчас).

### Use cases

CreateSupplier / ArchiveSupplier; UpsertSupplierPrice;
CreatePurchaseOrder → Send → закрытие вручную или авто при полной
приёмке; ReceiveAgainstOrder (черновик накладной из заявки);
PriceHistoryReport (динамика закупочных цен — источник алертов «поставщик
поднял цену», → событие: `SupplierPriceIncreased`, консюмер — AI-ассистент, позже).

### Таблицы

```sql
suppliers        (id, restaurant_id, name, contacts jsonb, note, archived;
                  UNIQUE (restaurant_id, lower(name)))
supplier_prices  (id, restaurant_id, supplier_id FK, stock_item_id FK,
                  price_cents int, valid_from date;
                  UNIQUE (supplier_id, stock_item_id, valid_from))
purchase_orders  (id, restaurant_id, supplier_id FK, status, note,
                  created_by, sent_at, closed_at)
purchase_order_lines (id, order_id FK, stock_item_id FK, qty, price_cents int)
-- goods_receipts.purchase_order_id uuid NULL — колонка из §1
```

### MVP / потом

- **MVP:** поставщики + supplier_id на накладной + автопрайс из накладных.
  Заявки (PurchaseOrder) — **не MVP**: ресторан первые месяцы принимает
  «что привезли», ценность заявок появляется с объёмом.
- **Потом:** заявки, сверка заявка/факт, алерты цен, авто-заявка от min_stock.

---

## 3. Персонал — расширение `platform` (роли) + новый контекст `workforce`

### Почему раскол на два

Роли и права — это **идентичность и доступ**: они нужны каждому контексту
через auth и живут там, где users, sessions и permission-checks — в
platform. Тащить их в новый контекст — значит, каждый запрос ходит в два
места за «кто ты и что можешь».

Расписания, табель и мотивация — **операционный учёт труда**: свой язык
(смена-в-графике ≠ кассовая смена pos!), свои акторы, связь с
platform-User — только ссылка на id. Это отдельный supporting-контекст
`workforce`, downstream от platform (id сотрудников) и pos (продажи для
мотивации — по событиям).

### 3a. Роли — расширение platform

`users.role` — text, расширение дёшево: добавить `cook`, `cashier`,
`storekeeper`. Все — restaurant-scoped (RestaurantID обязателен, как
waiter; инвариант фабрик `NewOwner`/`NewStaff` из ddd-architecture.md §3
не меняется).

Права — **статическая матрица в коде platform/app** (permission → роли),
не таблица: ролей 6, кастомных ролей никто не просил (YAGNI; iiko-style
конструктор прав — потом, если появится second tenant с особыми ролями).
Эскиз матрицы:

| Право | owner | manager | cashier | waiter | cook | storekeeper |
|---|---|---|---|---|---|---|
| POS: смена (открыть/закрыть), внесения/изъятия | ✓ | ✓ | ✓ | — | — | — |
| POS: тикеты, приём оплат | ✓ | ✓ | ✓ | ✓ | — | — |
| KDS: статусы блюд | ✓ | ✓ | — | — | ✓ | — |
| Inventory: документы | ✓ | ✓ | — | — | — | ✓ |
| Inventory: инвентаризация post | ✓ | ✓ | — | — | — | — |
| Workforce: график, табель | ✓ | ✓ | — | — | — | — |
| Меню, тех-карты, темы | ✓ | ✓ | — | — | — | — |
| Финансы: P&L, скидки задним числом | ✓ | ✓* | — | — | — | — |
| Биллинг, штат, орг-настройки | ✓ | — | — | — | — | — |

\* P&L менеджеру — настраиваемый флаг ресторана (потом), в MVP — да.

### 3b. Контекст `workforce`

#### Агрегаты

**ScheduledShift (плановая смена)** — корень: сам по себе (строка, не
недельный «график»-контейнер: правки идут посменно, контейнер дал бы
конфликты редактирования на пустом месте).
- Поля: user_id (без FK — platform), role_on_shift, starts_at, ends_at, note.
- Инвариант «сотрудник не в двух местах одновременно» — **EXCLUDE
  constraint в БД** (`btree_gist`: `EXCLUDE USING gist (user_id WITH =,
  tstzrange(starts_at, ends_at) WITH &&)`) — то же семейство решений, что
  partial unique для open shift; агрегат это проверить не может
  (кросс-агрегатное правило).
- ends_at > starts_at — CHECK.

**TimeEntry (табель, факт)** — корень: сам по себе.
- clock_in / clock_out (отметка на POS-планшете PIN-кодом или менеджером
  вручную), source: `pos_clock` | `manual`.
- Инвариант: **≤1 открытой записи на сотрудника** — partial unique
  `(user_id) WHERE clock_out IS NULL` (БД). Закрытая запись правится
  только манагером (аудит-поле edited_by).
- Табель за период = запрос по time_entries, отдельного агрегата
  «месячный табель» нет — это отчёт.
- → событие: `TimeEntryClosed` (Finance: labor cost по факту).

**MotivationRule** — корень: сам по себе. Декларативное правило:
- scope: `personal` (продажи официанта) | `restaurant` (все продажи);
- basis: `percent_of_sales` (bps — сотые доли процента, integer, в духе
  cents) | `fixed_per_shift_cents`;
- role — к какой роли применяется; active_from/active_to.
- Начисления считаются **отчётом по требованию** (продажи из pos ×
  правило), не материализуются по каждому чеку: правило поменяли задним
  числом — отчёт честно пересчитался. Материализация — только при выплате
  (потом, вместе с payroll).
- **Prerequisite в pos:** у продаж нет автора — тикеты/строки не хранят
  официанта. Нужна колонка `ticket_lines.added_by uuid` (заполняется из
  сессии в существующем add-lines пути). Без неё возможна только
  restaurant-scope мотивация. Помечаю как единственное изменение pos ради
  workforce.

#### Use cases

PlanShift / EditShift / DeleteShift (менеджер); MySchedule (сотрудник);
ClockIn / ClockOut (PIN на POS); CorrectTimeEntry (менеджер);
TimesheetReport (часы план/факт за период);
UpsertMotivationRule; MotivationReport (заработано по правилам за период).

#### Таблицы (`migrations/workforce/`)

```sql
scheduled_shifts (id, restaurant_id, user_id uuid /*без FK*/, role_on_shift,
                  starts_at, ends_at CHECK (ends_at > starts_at), note,
                  EXCLUDE USING gist (user_id WITH =, tstzrange(starts_at, ends_at) WITH &&))
time_entries     (id, restaurant_id, user_id uuid, clock_in, clock_out NULL,
                  source, edited_by uuid NULL;
                  PARTIAL UNIQUE (user_id) WHERE clock_out IS NULL)
motivation_rules (id, restaurant_id, role, scope, basis, rate_bps int NULL,
                  fixed_cents int NULL, active_from date, active_to date NULL)
pos_pins         (user_id PK /*без FK, platform*/, restaurant_id, pin_hash bytea)
```

#### MVP / потом

- **MVP:** роли (3a) + плановые смены + clock in/out + табель-отчёт.
- **Потом:** мотивация (после `added_by` в pos), payroll-выплаты,
  переработки/ставки, интеграция табеля в P&L автоматом.

---

## 4. Финансы — расширение `pos` (деньги смены) + новый контекст `finance` (P&L)

### Почему раскол на два

Оплаты, внесения/изъятия и чаевые — часть **транзакционного инварианта
кассовой смены**: `expected = float + нал.оплаты + внесения − изъятия`
обязан считаться в той же транзакции, что и `Shift.Close` (топ-1 из
ddd-architecture.md — атомарный CloseShift; расширяем именно его, второй
источник правды о кассе недопустим). Это pos.

P&L — **отчётный** контекст: читает факты всех остальных (выручка pos,
COGS inventory, labor workforce), добавляет единственные свои данные —
прочие расходы (аренда, коммуналка). Никаких общих транзакций с pos.
Классический reporting/downstream BC.

### 4a. Оплаты и касса — расширение pos

Сейчас `TicketClosed` = «пометили closed», денег нет. Вводим:

**Payment** — entity внутри границы агрегата Ticket.
- method: `cash` | `card`; amount_cents; tip_cents (отдельно от amount —
  чаевые не выручка и не участвуют в food cost/P&L revenue).
- Инварианты Ticket (в агрегате, транзакционно):
  - оплата возможна только у `status='open'` (то же правило `AND
    status='open'` + RowsAffected, что чинит FireTicket);
  - закрытие требует `Σ payments.amount == Σ lines total` (сплит-оплата =
    несколько payments; частичная оплата без закрытия разрешена);
  - переплата наличными = сдача: платёж хранит `tendered_cents` и
    `change_cents`, в кассу ложится `amount = tendered − change`.
- Скидка на чек (нужна Loyalty, §5): `ticket_discounts` — строки-снапшоты
  `(kind, label, amount_cents, promo_id?/points?)`; тогда инвариант
  закрытия: `Σ payments == Σ lines − Σ discounts`.
- → событие: `TicketClosed` обогащается: total_cents, discount_cents,
  payments `[{method, amount, tip}]` (нужно Finance и Loyalty).

**CashOperation (внесение/изъятие)** — entity внутри границы Shift.
- kind: `pay_in` | `pay_out`; amount_cents > 0; reason (текст, обязателен);
  created_by. Только при открытой смене (`shift.closed_at IS NULL` в
  транзакции вставки — SELECT ... FOR UPDATE смены).
- → событие: `CashOperationRecorded`.

**Shift.Close** меняет формулу (сигнатура Close(declared, cashSales,
payIns, payOuts)):
`expected = opening_float + Σ cash payments (за вычетом сдачи) + Σ pay_in − Σ pay_out`.
Карта в expected не входит (в ящике её нет); сверка эквайринга — потом.
Чаевые: наличные чаевые в ящик не считаем (официант забирает), картой —
копятся на смене информационно (`tips_card_cents` в отчёте закрытия).

**X/Z-отчёт**: Z = существующий PostedShift + разбивка по методам оплаты
и операциям — запрос, не новый агрегат.

### 4b. Контекст `finance` — P&L

#### Агрегаты

**ExpenseEntry (прочий расход)** — корень: сам по себе.
- category — фиксированный enum: `rent`, `utilities`, `marketing`,
  `maintenance`, `other` (план счетов — YAGNI, вводим когда попросят
  бухгалтерскую выгрузку); amount_cents, incurred_on (date), note, created_by.
- Инварианты тривиальны (amount > 0); ценность — единое место ввода.

**PnL — не агрегат, а отчёт** (это важно: не materialized таблица «строки
P&L», которую надо инвалидировать). За период:

| Строка | Источник |
|---|---|
| Выручка | pos: Σ закрытых тикетов (lines − discounts), без чаевых |
| − COGS (food cost) | inventory: Σ cost_cents движений kind='sale' |
| = Валовая прибыль | |
| − Labor | workforce: часы табеля × ставка (ставок пока нет ⇒ MVP: ручной ExpenseEntry category='labor') |
| − Списания/недостачи | inventory: движения writeoff + stocktake_shortage |
| − Прочие расходы | finance: expense_entries |
| = Операционная прибыль | |

Чтение чужих данных: finance-app читает **через порты-читалки соседних
контекстов** (`ports.SalesReader`, `ports.CogsReader` — реализации в
bridge-адаптерах поверх чужих sqlc-запросов), не голым SQL по чужим
таблицам — тот же урок, что с CRM в ddd-architecture.md §4. Пересчёт
онлайн по запросу; кэш дневных агрегатов — когда станет медленно.

#### Use cases

RecordExpense / EditExpense (owner/manager); PnLReport(period);
DailySummary (выручка/средний чек/food cost % за день — витрина админки).

#### Таблицы

```sql
-- pos (расширение):
payments          (id, ticket_id FK, method, amount_cents, tip_cents,
                   tendered_cents NULL, change_cents NULL, created_by, created_at)
ticket_discounts  (id, ticket_id FK, kind /*promo|points|manual*/, label,
                   amount_cents, promo_id uuid NULL, created_by)
cash_operations   (id, shift_id FK, kind, amount_cents CHECK (> 0), reason,
                   created_by, created_at)
-- finance (migrations/finance/):
expense_entries   (id, restaurant_id, category, amount_cents CHECK (> 0),
                   incurred_on date, note, created_by, created_at)
```

#### MVP / потом

- **MVP (4a почти весь — это money path):** payments (нал/карта, сдача,
  чаевые), cash_operations, новая формула Close, Z-отчёт.
  Из 4b: expense_entries + PnL-отчёт (labor — ручной строкой).
- **Потом:** сверка эквайринга, labor из табеля автоматом, дневные
  агрегаты, экспорт бухгалтерии, мульти-валюта — никогда (пока не попросят).

---

## 5. Лояльность и маркетинг — новый контекст `loyalty`

### Почему отдельный bounded context

Баланс баллов — **org-scoped** (Customer платформенный, гость копит в
сети ресторанов организации — модель iikoCard), а pos и menu —
restaurant-scoped: уже поэтому не расширение pos. Свой язык (начисление,
сгорание, промо-механика), свой темп изменений (маркетинг крутит правила
еженедельно). Platform-Customer — upstream (id гостя), pos — синхронный
потребитель при оплате.

Интеграция по правилу §0: применение скидки/списание баллов требует
**ответа** (сколько можно списать?) ⇒ синхронный порт `pos →
ports.Loyalty` (реализация — bridge-адаптер, в процессе). Начисление —
реакция на факт ⇒ → событие: `TicketClosed` (консюмер loyalty).

### Агрегаты

**LoyaltyProgram** — корень: сам по себе; **одна на организацию** —
`org_id PRIMARY KEY` (та же техника, что `subscriptions.org_id PK`).
- Параметры: accrual_bps (сколько баллов за 100 центов, в bps),
  redeem_limit_bps (макс. доля чека баллами, дефолт 5000 = 50%),
  points_expire_days NULL, active.
- 1 балл = 1 цент при списании (просто, объяснимо гостю; курс — потом).

**LoyaltyAccount** — корень: LoyaltyAccount; граница: + PointsTransaction[].
- Идентичность: `(org_id, customer_id)` — unique (БД). Создаётся лениво
  первым начислением (как guest_profile).
- balance_points int — денормализован, `CHECK (balance_points >= 0)` (БД
  добивает инвариант «не уйти в минус» при гонке двух списаний; UPDATE
  ... SET balance = balance − X WHERE balance >= X + RowsAffected).
- Транзакции append-only: `accrual | redemption | expiry | manual_adjust`,
  points signed, ticket_id/source_event_id для идемпотентности (unique).
- Резервирование баллов на время «тикет открыт со скидкой» — НЕ делаем:
  списание происходит в момент закрытия тикета (синхронный вызов внутри
  транзакции оплаты); между «показали скидку» и «закрыли» баланс мог
  упасть — тогда закрытие вернёт 409, официант уберёт скидку. Редкий
  случай, резервирование не окупается.

**Promotion** — корень: сам по себе. Декларативное правило скидки:
- kind: `percent_off_ticket` | `fixed_off_ticket` | `percent_off_items`
  (по списку menu_item_id — снапшот-массив без FK) | `happy_hours`
  (weekday/чч:мм-интервал + процент);
- restaurant_id (промо — ресторанные, в отличие от баллов) , active_from/to, archived.
- Применение: pos спрашивает `ApplicablePromos(restaurant, time)` и
  считает скидку **на своей стороне**, кладя снапшот в ticket_discounts —
  промо потом можно менять/удалять, чеки не поедут.
- Комбо/сеты, купоны-коды, «N-й кофе бесплатно» — потом (каждый — своя
  механика; каркас kind+params jsonb это уже вмещает).

### Use cases

ConfigureProgram (owner); гость в ЛК видит баланс (`GET /customer/me`
дополняется balance);
UpsertPromotion / ArchivePromotion (менеджер);
ApplicableAtTicket (pos, синхронно); RedeemAtClose (pos, синхронно, в
транзакции закрытия тикета — списание + скидка);
AccrueOnTicketClosed (событие, идемпотентно);
ExpirePoints (периодическая задача — потом);
→ событие: `PointsAccrued`, `PointsRedeemed` (маркетинговая аналитика, потом).

Обязательное требование AGENTS.md (AI/данные across tenants) здесь
превращается в: баллы — org-scoped, но **аналитика по гостю не пересекает
организации**; guest CRM ресторана видит только свои визиты (уже так).

### Таблицы (`migrations/loyalty/`)

```sql
loyalty_programs   (org_id PK /*без FK — platform*/, accrual_bps int,
                    redeem_limit_bps int, points_expire_days int NULL, active bool)
loyalty_accounts   (id, org_id, customer_id uuid /*без FK*/,
                    balance_points int CHECK (balance_points >= 0);
                    UNIQUE (org_id, customer_id))
points_transactions (id, account_id FK, kind, points int /*signed*/,
                    ticket_id uuid NULL, source_event_id uuid NULL UNIQUE,
                    note, created_at)
promotions         (id, restaurant_id, kind, params jsonb, name,
                    active_from, active_to NULL, archived)
```

### MVP / потом

- **MVP:** программа + счета + начисление по TicketClosed + списание при
  закрытии + один вид промо (`percent_off_ticket` вручную официантом с
  правом манагера). Гостю — баланс в ЛК.
- **Потом:** happy hours, item-промо, купоны, сгорание, сегменты гостей и
  рассылки (последнее — вообще отдельный разговор + каналы доставки).

---

## 6. Кухня (KDS) — расширение контекста `pos`

### Почему расширение, а не новый контекст

KDS оперирует **теми же ticket_lines** и тем же жизненным циклом fire —
у kitchen нет ни одной собственной сущности с независимым lifecycle,
кроме справочника станций. Отдельный BC означал бы проекцию строк тикета
наружу и обратную связь «готово» через границу — двойная запись правды о
статусе строки. iiko делает так же: KDS — экран iikoFront, не отдельная
система. Выделять в отдельный контекст стоит, только когда KDS станет
отдельным процессом/устройством с офлайном (не наша забота — офлайн
проектирует другой агент).

Внутри pos это подпакет `pos/kitchen` (screen отдельный — `/kds`, право
роли `cook`).

### Модель

**Station (станция)** — справочник: `(id, restaurant_id, name, position)`.
Маршрутизация: `station_categories (station_id, category_id uuid /*без
FK — menu*/)` — станция подписана на категории меню. Позиция без станции
падает на дефолтную «Кухня». Маршрутизация по категориям, не по позициям:
дешевле в настройке, покрывает 95% («Бар» = категории напитков). Инвариант
«категория максимум на одной станции» — unique `(restaurant_id,
category_id)` (БД).

**Жизненный цикл строки** — расширение TicketLine (не новая сущность:
статус готовки — свойство той же строки, двух правд не заводим):

```
fired_at (есть) → started_at → ready_at → served_at
```

- Переходы только вперёд, только у строки открытого тикета (тот же
  `AND status='open'` guard). Bump на KDS = ready; официант в pos видит
  «готово» и отмечает served.
- Строка привязывается к станции **в момент fire** (снапшот
  `station_id`, без FK на category — правила маршрутизации меняются, чеки
  на кухне не должны перепрыгивать).
- Курсы (закуски → горячее, «отдать вместе») — потом: поле course int на
  строке уже закладываем (default 0), логику придержки не строим.
- → событие: `LineReady` (пуш официанту; пока поллинг pos/state — просто
  новые поля в ответе), `LineServed`.

**Времена отдачи**: target_prep_minutes — на menu_item (колонка в menu
контексте, менеджер задаёт в админке; nullable = без норматива).
KDS красит чек: elapsed = now − fired_at vs max(target по строкам).
Отчёт «среднее время отдачи по позиции/станции/часу» — запрос по
timestamp-колонкам, отдельного хранилища не нужно.

### Use cases

UpsertStation / AssignCategories (менеджер);
KdsQueue (повар: строки fired, не served, своей станции, сгруппированные
по тикету, отсортированные по fired_at);
StartLine / BumpReady (повар); MarkServed (официант, из pos-экрана);
RecallLine (повар вернул ready → started — ошибся; только пока не served);
PrepTimeReport (менеджер).

### Таблицы (pos-миграции)

```sql
stations           (id, restaurant_id, name, position;
                    UNIQUE (restaurant_id, lower(name)))
station_categories (restaurant_id, station_id FK, category_id uuid /*без FK*/,
                    PRIMARY KEY (station_id, category_id),
                    UNIQUE (restaurant_id, category_id))
-- ticket_lines +: station_id uuid NULL, course int DEFAULT 0,
--                 started_at, ready_at, served_at (timestamptz NULL)
-- menu_items +:  target_prep_minutes int NULL  (миграция в batch menu)
```

### MVP / потом

- **MVP:** одна дефолтная станция, статусы fired → ready → served, экран
  KDS поверх существующего поллинга, target_prep_minutes + подсветка.
- **Потом:** мульти-станции с маршрутизацией, курсы/придержка, RecallLine,
  отчёт времён, SSE вместо поллинга (общий с pos переход).

---

## 7. Сводка: порядок внедрения и зависимости

Пересечение с топ-10 существующего рефакторинга: **пункт 4a (оплаты)
строится прямо на «атомарном CloseShift» (топ-1)** — делать их вместе,
иначе формула expected перепишется дважды.

| Волна | Что | Почему сначала |
|---|---|---|
| 1 | Оплаты/касса в pos (4a) + роли (3a) | money path; всё остальное (P&L, лояльность, мотивация) читает payments; роли нужны каждому новому экрану |
| 2 | Inventory MVP (§1) + поставщики (§2 MVP) | главный запрос «уровня iiko»; зависит только от TicketClosed |
| 3 | KDS MVP (§6) | мал, изолирован, виден клиенту |
| 4 | Workforce MVP (§3b) | независим; мотивация ждёт `added_by` |
| 5 | Finance P&L (§4b) | отчёт поверх волн 1–2 |
| 6 | Loyalty MVP (§5) | требует payments + ticket_discounts из волны 1 |

Новые события для EVENTS.md (сводно): обогащённый `TicketClosed`
(строки, payments, discounts), `GoodsReceiptPosted`, `StockWrittenOff`,
`StocktakePosted`, `CashOperationRecorded`, `TimeEntryClosed`,
`PointsAccrued`, `PointsRedeemed`, `LineReady`, `LineServed`,
`PurchaseOrderSent`, `SupplierPriceIncreased`. Механика доставки — вне
этого документа.

Новые синхронные порты (единственные): `pos → ports.Loyalty`
(промо/списание), `finance → ports.SalesReader/CogsReader/...`
(отчётные читалки). Всё остальное межконтекстное — события.

Что сознательно НЕ проектируем: план счетов, мульти-валюту, кастомные
роли, резервирование баллов, версии тех-карт, мульти-склады, payroll —
у каждого выше отмечено условие, при котором оно появляется.
