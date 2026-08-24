# Инкремент-2: склад и тех-карты — реализуемый контракт

Рабочий контракт второго инкремента для команды backender / frontender / tester.
Один проход. Опирается на: `reference.md` (D1–D8; Domain 2 «Stock, Valuation &
Costing»; Domain 3 «Nomenclature & Recipes»; Domain 4 — приход; §14 анти-паттерны;
§15 refuted 2/3), `impl-contract.md` (что уже построено в инкременте-1: контекст
`ledger` с append-only журналом, приёмка смены, тендеры, provisioning-хук),
`domain.md` §1–2 (ранний дизайн склада) и `ddd-architecture.md` (правила импортов).

Всё, что не описано здесь как «делаем», — вне инкремента.

Инкремент-2 = **«Склад и тех-карты»**: перманентный складской ленджер (D2),
номенклатура и calendar-versioned тех-карты (D3/D5), складские документы с
GL-эффектом через существующий `ledger` (D4), и списание себестоимости по продаже
(COGS) в момент закрытия тикета (Domain 5).

> **Отношение к `domain.md` §1–2.** Ранний дизайн выбрал FIFO, версии тех-карт «не
> в MVP» и списание по продаже **через событие**. Настоящий контракт эти три
> решения **пересматривает** по требованию reference: взвешенная средняя вместо
> FIFO (D2), calendar-versioned карты (D5), синхронное списание в транзакции
> `CloseTicket` (Domain 5; async-шины у нас нет — outbox мёртв, ddd §4). Термины и
> таблицы `domain.md` переиспользуются, где не противоречат.

---

## 1. Скоуп и анти-скоуп

### В скоупе

1. **Номенклатура** (`inventory.Product`): единый product-entity с закрытым type
   enum `goods | dish | prepared | modifier` (Domain 3), единицы измерения с
   конверсией (kg/g/l/ml/pcs), связь `dish → menu_items` через `menu_item_id`
   (без FK, конформизм к menu). §3.
2. **Тех-карты** calendar-versioned по D5: интервалы валидности, ≤1 версия на день,
   backdated-создание закрывает предыдущую в полночь, рецепт-циклы запрещены,
   себестоимость — **append-only time series** (не мутируемое поле), consumption
   strategy (`assemble | deplete_finished`) на карте. §4.
3. **Склад perpetual** по D2: `stock_moves` append-only (qty + cost, две даты D7),
   **взвешенная средняя** себестоимость, материализованный `stock_on_hand`. §5.
4. **Складские документы** по D4 (`draft → posted → cancelled=reversal`): приходная
   накладная (+ минимальный справочник поставщиков), акт списания, инвентаризация
   server-computed с **dry-run** эндпоинтом и авто-проводкой излишков/недостач. §6.
5. **COGS по продаже**: в транзакции `CloseTicket` (инкремент-1) — расход
   ингредиентов по активной на бизнес-дату тех-карте, отдельный GL-документ
   `kind='cogs'`. §7.
6. **Food cost отчёт**: теоретический (Σ себестоимости рецептов проданного) vs
   фактический (Σ `cost_cents` sale-движений), минимально. §8.
7. **Расширение seed плана счётов** (`ledger`): счета Inventory / Accounts payable /
   COGS / Shrinkage / Surplus + map-purposes. §9.

### Анти-скоуп (жёстко не тащим)

- **Полный repost** («пересчёт от даты X») — **нет**. Вместо него — **запрет
  backdate-постинга склада раньше последнего проведённого движения по позиции**
  (§5.4). Задокументировано как временное упрощение с путём апгрейда до repost.
- **FIFO / LIFO / lot-costing** — **нет**. Только взвешенная средняя (moving
  average). Причина и upgrade-path — §5.3.
- **Закупочные заявки (PurchaseOrder) / EDI / прайс-листы поставщиков** — нет
  (это следующая волна; `domain.md` §2 «Потом»). Поставщик — минимальный
  справочник только ради `supplier_id` на накладной.
- **Landed cost** (распределение фрахта/пошлин по строкам) — нет.
- **Batch / expiry / серийный учёт** — нет (партий как агрегата нет; средняя не
  требует lot-tracking).
- **Мультивалюта** — нет; single currency (база компании), как в инкременте-1.
  Все amount-колонки — `bigint` cents с маркером `-- single currency; multicurrency
  deferred — reference §16.4`.
- **Мульти-склады / перемещения** — нет; весь ресторан = один склад (колонка
  `storage_id` добавится позже без ломки схемы).
- **Полуфабрикаты с рекурсивным разворачиванием при продаже** — списание при
  продаже разворачивает тех-карту **на один уровень** (§7.3). Рекурсивная
  теоретическая себестоимость для отчёта — да (§4.4), рекурсивное списание — нет.
- **Received-not-billed цепочка / отдельная фактура / оплата поставщику (AP
  settlement)** — нет. Приход кредитует **Accounts Payable напрямую** (§6.2,
  обоснование по Domain 4). Счёт `received_not_billed` засеян как заготовка.
- **6-ролевая матрица прав** — нет; гейтим существующими ролями (§10).
- **Kafka / relay / async-события** — нет; списание по продаже — синхронный порт
  (§7), не событие.

---

## 2. Размещение кода — новый контекст `internal/inventory`

**Решение: отдельный bounded context `inventory`, а НЕ расширение `ledger`.**

Обоснование (Domain 3 + `domain.md` §1): свой убиквитарный язык («номенклатура»,
«накладная», «списание», «выход», «себестоимость»), независимый жизненный цикл
(меню меняется ежедневно, номенклатура стабильна), downstream от menu (conformist:
читает `menu_item_id`) и от ledger (постит GL через порт — ровно то же отношение,
что у pos). `ledger` — это generic движок проводок; `inventory` — источник
документов, которые в него постят. Расширять `ledger` инвентарём значило бы смешать
два языка в одном контексте.

### Структура (по факту-конвенции репо, как ledger/pos)

```
internal/domain/inventory/        # чистый домен: импортирует только sharedkernel+stdlib
  product.go        Product, Unit (+ конверсия), ProductType
  techcard.go       TechCard, TechCardLine, RecipeCosting, ConsumptionStrategy
  stock.go          StockMove, OnHand, moving-average расчёт
  documents.go      GoodsReceipt(+Line), WriteOff(+Line), Stocktake(+Line), Supplier
internal/inventory/
  app/              use-cases (см. §11)
  ports/            Store, Ledger (порт в ledger), ErrNotFound/ErrConflict/…
  adapters/postgres/  ручной database/sql (конвенция репо) + inventorydb (sqlc-генерат, как ledgerdb)
  adapters/ledgerbridge/  реализация inventory/ports.Ledger поверх ledger/app
migrations/inventory/0001_init.up.sql / .down.sql
queries/inventory/inventory.sql   # sqlc block `inventory`
```

### Правила импортов (ddd-architecture §5) — соблюсти

- `inventory/domain` → только `sharedkernel` + stdlib. Деньги — `int64` cents,
  количества — `int64` милли-единицы базовой единицы (аналог cents, `domain.md` §0).
- `inventory/app` → свой `domain` + `ports`; чужой контекст (`ledger`) — **только
  через bridge-адаптер** `ledgerbridge`, не прямым импортом `ledger/app` в app.
- `pos → inventory` — легально (pos уже downstream): `pos/adapters/inventorybridge`
  реализует `pos/ports.Inventory` поверх `inventory/app`. Направление
  `pos → inventory → ledger`, циклов нет. `inventory → pos` и `inventory → menu`
  (кодом) **запрещены** — `menu_item_id` хранится голым uuid.
- `migrations` порядок батчей: `menu → platform → pos → inventory` (новый — в
  конец). `CREATE TABLE` между батчами не переносим.
- sqlc: новый блок `inventory` в `backend/sqlc.yaml`,
  `schema: ["migrations/menu","migrations/platform","migrations/ledger","migrations/pos","migrations/inventory"]`,
  `queries: ["queries/inventory"]`, package `inventorydb`. Адаптеры пишем вручную
  (как ledger/pos), `inventorydb` — сгенерирован-но-не-используется (конвенция).

### Порт `inventory → ledger` (расширение ledger/app — единственное)

`ledger/app` получает **один** новый метод для проводки инвентарных документов на
общей транзакции (аналог `BuildDraftShiftJournal`, но постит сразу — инвентарные
проводки механические, ревью человеком не требуют):

```
PostInventoryJournal(ctx, tx *sql.Tx, in InventoryJournalInput) (docID uuid, err)
  InventoryJournalInput{ RestaurantID, SourceKind, SourceID, AccountingDate,
                         Lines []{ Purpose string, Side, AmountCents } }
```

Резолвит `Purpose → account_id` через существующий `ledger_account_map` (та же
точка настройки §15.6), строит `JournalDocument` (source_kind = `inventory_receipt`
| `inventory_writeoff` | `inventory_stocktake` | `cogs`), добавляет строки,
`AutoBalance` на `rounding_unassigned`, `Post()` немедленно. Append-only сохранён:
коррекция — только через существующий `CancelJournal` (reversal). `inventory/ports.
Ledger` оборачивает этот метод + `CancelJournal` + `AccountForPurpose`.

Никакого draft-состояния у инвентарных GL-документов: складской документ имеет свой
gate (`draft → posted`), а GL постится атомарно в момент его `post` — второй gate
избыточен (анти-паттерн §14.6 «уровень церемонии не на каждую сущность»).

---

## 3. Номенклатура — `Product`

### Агрегат `Product` (корень)

Поля словами: `id`, `restaurant_id`, `sku`, `name`, `type`
(`goods|dish|prepared|modifier`), `stock_unit` (`g|ml|pcs` — **базовая** единица
складского учёта), `menu_item_id` (uuid, nullable, без FK — только для `dish`),
`min_stock` (milli-units, для алертов; nullable), `archived`, `created_at`.

**Инварианты:**
- `sku` уникален в ресторане — `UNIQUE(restaurant_id, sku)` (БД).
- `stock_unit` **неизменяем после первого движения** по позиции (иначе книга
  движений теряет смысл; `domain.md` §1). Проверка в app/store, не UI.
- `menu_item_id` заполняется **только** у `type='dish'`; `≤1 dish на menu_item` —
  partial unique `(restaurant_id, menu_item_id) WHERE menu_item_id IS NOT NULL`
  (БД, как open-shift). menu-контекст не трогаем: ссылка голым uuid, миграций в
  `migrations/menu` нет.
- Удаление запрещено при наличии движений — `archived=true` (Domain 3
  анти-паттерн «deactivation-by-rename» → реальный статус-флаг).
- `modifier` — товар-модификатор (сироп, доп-сыр); складского движения по продаже
  сам по себе не даёт в MVP (может быть строкой чужой тех-карты). `prepared` —
  полуфабрикат (тесто, соус): имеет собственную тех-карту (как он собирается) и
  собственный `stock_on_hand`.

### Единицы измерения и конверсия

Единицы — **закрытый enum со статической таблицей конверсии в коде** (физические
константы, не runtime-сущность — YAGNI, ponytail):

| unit | dimension | factor → base |
|---|---|---|
| `g`  | mass   | 1 (base)  |
| `kg` | mass   | 1000      |
| `ml` | volume | 1 (base)  |
| `l`  | volume | 1000      |
| `pcs`| count  | 1 (base)  |

Базовых единиц три: `g`, `ml`, `pcs`. `Product.stock_unit ∈ {g, ml, pcs}` — в ней
живут `stock_moves`, `stock_on_hand`, `tech_card_lines.qty`. Любой ввод (строка
накладной, строка рецепта) может быть в **совместимой display-единице** (`kg` для
g-позиции, `l` для ml-позиции) и конвертируется в базовую на границе
(`qty_base = qty_input × factor`). Кросс-размерность (`kg` для `pcs`-позиции) →
`422 unit_incompatible`. Отдельного эндпоинта единиц нет — таблица статична, фронт
знает её из типов.

Количества: домен — `int64` милли-единицы базовой единицы; БД — `NUMERIC(12,3)` в
базовой единице (`domain.md` §0). Стоимость — целые cents; храним стоимость строки
/ движения целиком, делим при расчёте средней с банковским округлением.

---

## 4. Тех-карты — calendar-versioned (D5)

### Агрегат `TechCard` (корень; граница = TechCard + TechCardLine[])

Тех-карта принадлежит продукту (`product_id` типа `dish` или `prepared`) и
описывает, во что разворачивается его производство/продажа. Поля версии:
`id`, `restaurant_id`, `product_id`, `valid_from` (date), `valid_to` (date, NULL =
открытая/текущая), `consumption` (`assemble | deplete_finished`), `yield_qty`
(milli-units выхода, информ.), `created_by`, `created_at`.

Строки `TechCardLine`: `id`, `tech_card_id`, `ingredient_product_id`, `qty`
(milli-units брутто в базовой единице ингредиента), `seq`.

### Инварианты (calendar-versioned, D5)

- **Интервал валидности** `[valid_from, valid_to)`. Текущая версия — открытая
  (`valid_to IS NULL`); искусственного срока у текущей нет.
- **≤1 версия на день на продукт**: создание версии с `valid_from = D`:
  - находит версию, активную на `D` (её `valid_from ≤ D` и `valid_to` NULL или `> D`);
  - **закрывает её в полночь `D`**: `valid_to = D` (backdated-создание = обычное
    создание интервала, не спец-случай — D5);
  - если на `D` уже начинается версия (`valid_from = D`) — `409 version_exists`
    (ровно одна на день).
- **Активная на дату X** — first-class запрос:
  `WHERE product_id=? AND valid_from ≤ X AND (valid_to IS NULL OR valid_to > X)`.
  Partial unique для «не более одной открытой на продукт»:
  `(restaurant_id, product_id) WHERE valid_to IS NULL` (БД).
- **≥1 строки** при создании; `qty > 0`; один ингредиент — одна строка
  `UNIQUE(tech_card_id, ingredient_product_id)` (БД).
- **Рецепт-циклы запрещены**: при создании версии — DFS по графу
  `product → активная техкарта → ingredient_product → …`; если новый набор строк
  замыкает цикл (продукт достижим сам из себя) → `422 recipe_cycle`.
  `// ponytail: DFS по активным картам на valid_from, O(V+E); достаточно, пока
  номенклатура одного ресторана в память влезает.`
- Версии **иммутабельны по интервалу**: `valid_from` проведённой версии не
  правится; единственная правка — создать новую версию (которая закроет текущую).
  Строки версии правятся только пока версия текущая и по ней ещё нет sale-движений
  на её интервале (иначе — новая версия).

### Себестоимость рецепта — append-only time series (Domain 3, ключевой fix)

Себестоимость **НЕ поле документа**, а отдельный append-only ряд `recipe_costings`:
`id`, `tech_card_id`, `cost_cents`, `method` (`weighted_avg`), `computed_at`,
`computed_by`. Текущая себестоимость версии = **последняя** строка по `computed_at`.
Никогда UPDATE (анти-паттерн Domain 3 «мутировать cost поле submitted-документа
in place»).

Расчёт себестоимости версии (§4.4): `cost_cents = Σ по строкам (line.qty ×
unit_cost(ingredient))`, где `unit_cost`:
- `goods` — текущая взвешенная средняя из `stock_on_hand` (`value/qty`, или
  `last_avg_cents` если `qty ≤ 0`, или 0 если приходов не было);
- `prepared` / `dish` (вложенный) — **рекурсивно** себестоимость его активной
  тех-карты на ту же дату (цикл-guard уже гарантировал ацикличность).

Округление: банковское; последняя строка добирает остаток, суммы копеек сходятся.

**Триггер пересчёта** (reference: recompute gated to explicit/approve, не
непрерывно, не молчаливо): (а) при создании/активации версии; (б) по явному вызову
`POST …/tech-cards/{id}/recost`. Массового scheduled-пересчёта в инкременте нет.

### Consumption strategy (Domain 3 — стратегия на рецепте)

`consumption` на тех-карте, применяется при продаже (§7):
- **`assemble`** (дефолт): продажа списывает **продукты-ингредиенты**, перечисленные
  в строках карты (один уровень — §7.3).
- **`deplete_finished`**: продажа списывает сам продукт (`dish`/`prepared`) как
  готовый со своего `stock_on_hand` (retail-стиль — «кола 0.33», готовый десерт).

Это закрывает обе дыры Domain 3: «теоретическое» списание без карты и «карта не
участвует в продаже».

---

## 5. Склад — perpetual weighted average (D2)

### `StockMove` — append-only книга движений (D2)

Поля: `id`, `restaurant_id`, `product_id`, `kind`
(`receipt | sale | writeoff | stocktake_surplus | stocktake_shortage | reversal`),
`qty` (milli-units, **signed**: приход +, расход −), `cost_cents` (signed,
магнитуда стоимости этого движения), `estimated` (bool — оценочная стоимость при
недостатке остатка), `business_date` (date, D7 — бизнес-дата факта), `recorded_at`
(timestamptz, D7 — реальная запись), `doc_kind`, `doc_id` (источник),
`source_event_id` (uuid, nullable, `UNIQUE` — идемпотентность списания по продаже),
`created_at`.

**Инварианты:**
- **append-only**: движение не редактируется и не удаляется никогда (D1/D2).
  Коррекция — только reversal-движение (зеркальные `qty`/`cost`, дата — сегодня).
- **две даты** обязательны (D7).
- книга **самодостаточна**: `cost_cents` проставлен в момент движения — food cost
  считается без пересчёта задним числом.

### Взвешенная средняя + материализованный `stock_on_hand`

**Решение: материализованный `stock_on_hand` (перезаписываемый кэш), НЕ view.**

`stock_on_hand`: `restaurant_id`, `product_id` (PK-пара), `qty` (milli-units,
signed), `value_cents` (int64, стоимость остатка), `last_avg_cents` (int64 — средняя
за последнюю положительную единицу × 1000, для оценки при `qty ≤ 0`), `updated_at`.

Средняя за базовую единицу = `value_cents / (qty/1000)` при `qty > 0`.

Почему материализация, а не view/фолд по всей истории: взвешенная средняя
**порядко-зависима** — стоимость каждого расхода зависит от running-состояния на
момент расхода. Фолд всей истории на каждое закрытие тикета — O(n) на продажу
(анти-паттерн «пересчёт как штатный путь», D2). `stock_on_hand` — кэш, каждое
движение обновляет его в **той же транзакции** под `SELECT … FOR UPDATE` по
`(restaurant_id, product_id)`. Источник истины — `stock_moves`; `stock_on_hand`
обязан быть равен фолду (инвариант-тест: `Σ moves.qty = on_hand.qty`,
`Σ signed(cost) = on_hand.value_cents`).

**Механика применения движения** (одна tx на документ):
- **приход** (+q, +c): `qty += q`, `value += c`; если после `qty > 0` —
  `last_avg = round(value × 1000 / qty)`.
- **расход** (−q): `unit = (qty>0 ? value×1000/qty : last_avg)`;
  `cost = round(unit × q / 1000)`; `value -= cost`, `qty -= q`. Движению
  проставляем `cost_cents = cost`.
- **недостаток остатка** (`q > qty` или `qty ≤ 0`): расход не блокируем (продажу
  нельзя останавливать складом — Domain 5); дефицит оценивается по `last_avg`
  (или 0), движение `estimated = true`, `qty` уходит в минус.
  `// ponytail: отрицательный остаток оценивается по last_avg; инвентаризация
  корректирует. Точный repest — upgrade-path.`
- **reversal**: берёт `qty`/`cost` **исходного** движения (а не текущую среднюю) —
  на общий `*sql.Tx`; `qty -= orig.qty`, `value -= orig.cost`. Точная отмена без
  искажения средней.

### FIFO — анти-скоуп (зафиксировано)

FIFO/LIFO требуют lot-tracking (`stock_batches` с `qty_remaining`) и выбора партий
при расходе с `FOR UPDATE` по каждой партии (`domain.md` §1 так и проектировал).
Взвешенная средняя требует лишь пары `(qty, value)` на позицию, не хранит партий и
достаточна для single-currency / single-location ресторана — reference D2 явно
называет moving average приемлемым дефолтом. **Не делаем FIFO.**
Upgrade-path: добавить `stock_batches` + `valuation_method` на продукт; книга
движений уже несёт `cost_cents` по-движению, миграция аддитивна.

### Backdate — анти-скоуп полного repost, вместо него запрет

Полного repost нет. Вместо него: **при постинге складского документа его
`business_date` должна быть ≥ максимальной `business_date` уже проведённых движений
по каждой затронутой позиции** (иначе `422 backdated_before_last_move`, с указанием
позиции). Это сохраняет хронологический порядок расходов на позиции → взвешенная
средняя остаётся корректной без пересчёта (обобщение «standard-cost exemption» D2 —
«запретить backdate вместо repost»).
`// ponytail: запрет backdate раньше последнего поста по позиции — временно;
upgrade-path — формальный repost-job с checkpoints/идемпотентностью (D2).`
Списание по продаже (`sale`) под это правило не попадает по построению (тикет
закрывается текущим днём смены).

---

## 6. Складские документы (D4: draft → posted → cancelled = reversal)

Общий паттерн (лучший инвариант системы, копируем из `Shift.Close`): проведение —
`UPDATE … WHERE status='draft'` + проверка `RowsAffected`; проведённый документ
иммутабелен; отмена — **отдельным reversal-документом**, не правкой. Каждое
проведение атомарно: (а) вставка `stock_moves`, (б) обновление `stock_on_hand`,
(в) GL-проводка через `inventory/ports.Ledger.PostInventoryJournal` — всё в **одной
транзакции**.

### 6.1. Поставщик — минимальный справочник

`Supplier`: `id`, `restaurant_id`, `name`, `contacts` (jsonb, как
`restaurants.contacts`), `note`, `archived`. `UNIQUE(restaurant_id, lower(name))`.
Больше ничего (прайсы/заявки — анти-скоуп). Удаление при наличии накладных
запрещено (archived).

### 6.2. Приходная накладная `GoodsReceipt`

Корень + `GoodsReceiptLine[]`. Поля: `id`, `restaurant_id`, `supplier_id` (nullable),
`status` (`draft|posted|cancelled`), `business_date`, `note`, `posted_at`,
`posted_by`, `reversal_of` (nullable). Строки: `id`, `receipt_id`, `product_id`,
`qty_input` + `input_unit` → `qty_base`, `unit_price_cents` (**цена за единицу
ввода**), `line_cost_cents = round(unit_price × qty_input)`, `seq`.

**Post** (в одной tx): по каждой строке — `receipt`-движение (`+qty_base`,
`+line_cost_cents`), обновление `stock_on_hand`; GL-проводка.

**GL-решение (Domain 4).** У нас **нет** отдельной фактуры и цепочки AP-оплаты в
инкременте. `received_not_billed` (кредит-при-приёмке, дебет-при-фактуре) оставил
бы висящий счёт, который нечем закрыть. Поэтому приход — **однодокументная модель**
(receipt = receipt+invoice, «restaurant-friendly default merge» Domain 4 §Decision-1):

```
debit  Inventory (1200)         Σ line_cost
credit Accounts payable (2100)  Σ line_cost
```

AP — реальная стоящая кредиторка, накапливается; её погашение (оплата поставщику) —
следующий инкремент. Счёт `received_not_billed (2110)` **засеян как заготовка** под
будущий split receipt/invoice (upgrade-path), но проводками не используется.
`→ (позже) событие GoodsReceiptPosted`.

**Автопрайс поставщика** (`domain.md` §2) — **анти-скоуп** (нет `supplier_prices`).

### 6.3. Акт списания `WriteOff`

Корень + строки. `reason` (`spoilage | expiry | staff_meal | loss | other`), `note`,
`status`, `business_date`, `posted_at`, `posted_by`, `reversal_of`. Строки:
`product_id`, `qty_input`+`unit` → `qty_base`.

**Post**: `writeoff`-движение (`−qty_base`, `−cost` по текущей средней) + on_hand; GL:

```
debit  Inventory shrinkage / write-off (5910)   Σ cost
credit Inventory (1200)                          Σ cost
```

### 6.4. Инвентаризация `Stocktake` — server-computed + dry-run (D2 §4)

Корень + `StocktakeLine[]`. Статусы `draft → posted` (+ `cancelled`=reversal).
**≤1 незакрытой на ресторан** — partial unique `(restaurant_id) WHERE status='draft'`
(БД). Строки: `product_id`, `counted_qty` (milli-units), `expected_qty` (nullable —
**фиксируется в момент post**, не при создании: продажи во время пересчёта иначе
дают ложную недостачу, `domain.md` §1), `variance_qty`, `variance_cost_cents`.

- **Dry-run** (`POST …/dry-run`) — server считает `expected_qty` (= текущий
  `stock_on_hand.qty`) и `variance = counted − expected` по каждой строке и
  возвращает **без сохранения** движений/проводок (устраняет класс клиентских
  багов расчёта — D2 §4). Идемпотентен, состояние не меняет (refuted §15.2 — read
  без side-effect).
- **Post** — фиксирует `expected_qty` на текущий момент, по каждой ненулевой дельте
  создаёт движение:
  - излишек (`counted > expected`): `stocktake_surplus` (`+Δ`, `+cost` по средней);
  - недостача (`counted < expected`): `stocktake_shortage` (`−Δ`, `−cost`);
  - GL авто-проводка одним документом (агрегат по знаку):
    ```
    недостача:  debit  Inventory shrinkage (5910)  / credit Inventory (1200)
    излишек:    debit  Inventory (1200)            / credit Inventory surplus (4910)
    ```

### Отмена любого документа

`POST …/cancel` — создаёт reversal-документ (status исходного → `cancelled`,
`reversal_of` проставлен), зеркальные `stock_moves` (по исходным `qty`/`cost`),
on_hand откатывается точно, GL — через `CancelJournal` (reversal сегодняшней датой,
D1). Оригинал не мутируется (D4, Domain 4 «void-as-mirror»).

---

## 7. Списание себестоимости по продаже (COGS)

### 7.1. Точка входа и транзакция

Хук — в **существующем `pos/app.CloseTicket`** (инкремент-1,
`internal/pos/app/app.go:256`). Транзакция `CloseTicket` уже атомарна (payments +
`tickets.closed_at`). Внутрь той же tx добавляется вызов `pos/ports.Inventory.
ConsumeForSale(ctx, tx, restaurantID, businessDate, lines)`, где `lines` —
`[]{ menu_item_id, qty }` из `ticket_lines` (колонка `menu_item_id` уже есть —
`migrations/pos/0001`, доп. префикс pos не нужен).

`businessDate` = бизнес-дата открытой смены тикета (не таймстемп последнего тикета —
refuted §15.4, детерминированная дата).

### 7.2. Решение: отдельный GL-документ `kind='cogs'` на тикет (обосновано)

COGS постится **отдельным журналом** (`source_kind='cogs'`, `source_id=ticket_id`),
проведённым немедленно в tx `CloseTicket`, **а не** внутри shift-acceptance журнала.

Обоснование:
- shift-acceptance журнал — **человеко-ревьюируемый кассовый** документ (собирается
  на CloseShift, постится на Accept). COGS — **механический производный** факт на
  тикет; ждать акцепта смены он не должен, и мешать его строки с кассовым ревью
  нельзя (человек ревьюит кассу, не себестоимость).
- Себестоимость **фиксируется на тикете** (cost at the check), а не откладывается
  на консолидацию — Domain 5 анти-паттерн «deferring valuation to consolidation»
  и refuted §15.4.
- COGS не дискреционен → авто-постинг без gate уместен (Domain 5 «deplete at the
  moment of sale by the recipe's own consumption strategy»).

Проводка на закрытие тикета:

```
debit  Cost of goods sold (5000)   Σ cost списанных движений
credit Inventory (1200)            Σ cost
```

### 7.3. Разворачивание (один уровень)

Для каждой строки тикета: `menu_item_id → Product(type='dish', menu_item_id=…)`;
нет продукта или нет активной карты → строка пропускается (retail/непрослеженное).
Активная тех-карта на `businessDate` (D5-запрос) → по `consumption`:
- `assemble`: для каждой `TechCardLine` списать `line.qty × ticket_line.qty` с
  `ingredient_product_id` (взвешенная средняя, §5). **Один уровень**: если ингредиент
  сам `prepared` — списываем его `stock_on_hand` напрямую, рекурсивно карту НЕ
  разворачиваем (анти-скоуп; теоретическая себестоимость для отчёта — рекурсивна,
  фактическое списание — нет).
- `deplete_finished`: списать сам `dish/prepared` продукт (`ticket_line.qty × 1`).

Идемпотентность: каждое sale-движение несёт `source_event_id` (детерминированный по
`ticket_line_id`) с `UNIQUE`-индексом; повторный `CloseTicket` того же тикета не
задваивает списание. (Тикет и так иммутабелен после close — долг 3; это второй
пояс.)

> **Отклонение от `domain.md` §1 (event-only).** `domain.md` планировал
> `pos → inventory` только событием. Здесь — **синхронный порт в общей tx**:
> async-шины нет (outbox мёртв, ddd §4), а COGS-факт обязан лечь атомарно с
> продажей, иначе крэш оставит продажу без себестоимости. Правило ddd §4 «нужен
> факт атомарно → синхронно». Upgrade-path: вынести в событие с идемпотентностью
> по `source_event_id` (поле уже заложено), когда появится проводная доставка.

---

## 8. Food cost отчёт (минимально)

`GET …/inventory/reports/food-cost?from=&to=` за период по позициям/итого:
- **Фактический COGS** = `Σ |cost_cents|` движений `kind='sale'` за период.
- **Теоретический COGS** = `Σ` по проданным `dish` (из pos:
  `menu_item_id → qty` за период) × себестоимость активной на дату продажи
  тех-карты (из `recipe_costings`).
- **Доля estimated** = доля sale-движений с `estimated=true` (честность средней при
  отрицательных остатках).
- Выручка позиции — из pos (порт-читалка `inventory/ports.SalesReader` поверх
  posdb-запроса, не голым SQL по чужим таблицам — урок ddd §4). Food cost % =
  фактический COGS / выручка.

Materialized-витрины нет (пересчёт по запросу; кэш — когда станет медленно).

---

## 9. GL: расширение seed (§ ledger)

### Новые счета (добавить в `ledger/app/seed.go` defaultAccounts)

| code | name | type | normal_side | postable |
|---|---|---|---|---|
| `1200` | Inventory | asset | debit | yes |
| `2100` | Accounts payable | liability | credit | yes |
| `2110` | Received not billed | liability | credit | yes *(заготовка, не используется)* |
| `5000` | Cost of goods sold | expense | debit | yes |
| `5910` | Inventory shrinkage / write-off | expense | debit | yes |
| `4910` | Inventory surplus | revenue | credit | yes |

### Новые map-purposes (defaultMap)

`inventory→1200`, `accounts_payable→2100`, `received_not_billed→2110`,
`cogs→5000`, `inventory_shrinkage→5910`, `inventory_surplus→4910`.

Смена маппинга меняет счёт проводки (та же per-restaurant точка §15.6).

### Backfill существующих ресторанов

Новые рестораны получают счета через `provisioning.RestaurantProvisioner` (код-seed
уже вызывает `ledger.SeedRestaurantTx`; просто расширяется список). **Существующие**
(демо-тенант из инкремента-1) — data-миграция `migrations/ledger/0002_inventory_
accounts.up.sql`: `INSERT … SELECT` счетов и map-строк по всем текущим `restaurants`
(idempotent через `ON CONFLICT DO NOTHING` по `(restaurant_id, code)` /
`(restaurant_id, purpose)`). `provisioning` для inventory дополнительно не нужен —
складские таблицы стартуют пустыми, seed номенклатуры не требуется.

### Пример: продажа борща (мука 200г @ ср.6c/г) + приход муки 5000г @ 30000c

```
Приход (документ inventory_receipt, business_date D):
  debit  1200 Inventory          30000
  credit 2100 Accounts payable   30000
  on_hand[мука]: qty +5000g, value +30000, last_avg=6000 (×1000)

Продажа (документ cogs, ticket close, business_date D):
  debit  5000 COGS               1200
  credit 1200 Inventory          1200
  движение sale: мука −200g, cost 1200, estimated=false
```

---

## 10. API `/api/v1` (конвенции инкремента-1)

Деньги — integer cents; количества — number в display-единице + код единицы, сервер
конвертирует; IDs — uuid; ошибки `{"error":{"code","message"}}` (401/403/404/409/
422); тенант — из сессии. Роли (матрица `domain.md` §3a): **storekeeper+** —
номенклатура/накладные/списания/counts; **manager+** — тех-карты, инвентаризация
post, отчёты, редактирование маппинга. Все — restaurant-scoped.

### Номенклатура (storekeeper+, чтение — staff+)

```
GET   /restaurants/{id}/inventory/products                 → [{id,sku,name,type,stock_unit,menu_item_id,min_stock,archived}]
POST  /restaurants/{id}/inventory/products                 {sku,name,type,stock_unit,menu_item_id?,min_stock?}
GET   /restaurants/{id}/inventory/products/{pid}           → product + on_hand{qty,unit,value_cents,avg_cents}
PATCH /restaurants/{id}/inventory/products/{pid}           {name?,min_stock?,menu_item_id?,archived?}   // stock_unit — только пока нет движений
```
422 `sku_taken` / `unit_incompatible` / `unit_locked` (движения есть);
409 `menu_item_taken` (≥1 dish на позицию меню).

### Тех-карты (чтение storekeeper+, запись manager+)

```
GET   /restaurants/{id}/inventory/products/{pid}/tech-cards            → версии [{id,valid_from,valid_to,consumption,cost_cents}]
GET   /restaurants/{id}/inventory/products/{pid}/tech-cards/active?on= → активная на дату + строки + текущая себестоимость
POST  /restaurants/{id}/inventory/products/{pid}/tech-cards            {valid_from,consumption,yield_qty?,lines:[{ingredient_product_id,qty,unit}]}
GET   /restaurants/{id}/inventory/tech-cards/{tcid}                    → версия + строки + cost history
POST  /restaurants/{id}/inventory/tech-cards/{tcid}/recost             → новая строка recipe_costings, текущая cost
```
422 `recipe_cycle` / `empty_recipe` / `duplicate_ingredient` / `unit_incompatible`;
409 `version_exists` (уже есть версия с этим `valid_from`).

### Поставщики (storekeeper+)

```
GET   /restaurants/{id}/inventory/suppliers          → [{id,name,contacts,archived}]
POST  /restaurants/{id}/inventory/suppliers          {name,contacts?,note?}
PATCH /restaurants/{id}/inventory/suppliers/{sid}    {name?,contacts?,archived?}
```
409 `supplier_name_taken`.

### Приходные накладные (storekeeper+)

```
GET   /restaurants/{id}/inventory/receipts?from=&status=   → список
POST  /restaurants/{id}/inventory/receipts                 {supplier_id?,business_date,note?,lines:[{product_id,qty,unit,unit_price_cents}]}  // draft
GET   /restaurants/{id}/inventory/receipts/{rid}           → шапка + строки + posted_at/reversal_of
POST  /restaurants/{id}/inventory/receipts/{rid}/post      → движения + on_hand + GL (одна tx)
POST  /restaurants/{id}/inventory/receipts/{rid}/cancel    → reversal
```
409 `already_posted` / `already_cancelled`; 422 `empty_document` /
`backdated_before_last_move` / `unit_incompatible`.

### Акты списания (storekeeper+)

```
GET   /restaurants/{id}/inventory/write-offs?from=&status=
POST  /restaurants/{id}/inventory/write-offs               {reason,note?,business_date,lines:[{product_id,qty,unit}]}
GET   /restaurants/{id}/inventory/write-offs/{wid}
POST  /restaurants/{id}/inventory/write-offs/{wid}/post
POST  /restaurants/{id}/inventory/write-offs/{wid}/cancel
```

### Инвентаризация (создание/counts storekeeper+, dry-run/post manager+)

```
POST  /restaurants/{id}/inventory/stocktakes                {note?}                    // draft, ≤1 открытая
GET   /restaurants/{id}/inventory/stocktakes?status=
GET   /restaurants/{id}/inventory/stocktakes/{sid}          → строки {product_id,counted_qty,expected_qty?,variance?}
PATCH /restaurants/{id}/inventory/stocktakes/{sid}          {lines:[{product_id,counted_qty,unit}]}   // ввод пересчёта, только draft
POST  /restaurants/{id}/inventory/stocktakes/{sid}/dry-run  → expected+variance+cost, БЕЗ сохранения
POST  /restaurants/{id}/inventory/stocktakes/{sid}/post     → фикс expected + surplus/shortage движения + GL
POST  /restaurants/{id}/inventory/stocktakes/{sid}/cancel   → reversal
```
409 `stocktake_open_exists` / `already_posted`.

### Остатки и отчёты (manager+, остатки — storekeeper+)

```
GET   /restaurants/{id}/inventory/on-hand?low_stock=       → [{product_id,sku,name,qty,unit,value_cents,avg_cents,below_min}]
GET   /restaurants/{id}/inventory/stock-moves?from=&product=  → книга движений (append-only, две даты)
GET   /restaurants/{id}/inventory/reports/food-cost?from=&to=  → {items:[{menu_item_id,name,revenue_cents,actual_cogs_cents,theoretical_cogs_cents,food_cost_pct}],totals,estimated_share}
```

### POS (без нового эндпоинта)

COGS — побочный эффект существующего `POST /pos/tickets/{id}/close`. Тело/ответ
инкремента-1 не меняются; списание невидимо клиенту POS (виден только в
admin-отчётах). При отсутствии тех-карты — тихий пропуск, не ошибка (продажа не
блокируется).

---

## 11. Карта работ

### Backender (Go)

**Ledger — минимальное расширение (не новый контекст):**
- `ledger/app`: метод `PostInventoryJournal(ctx, tx, InventoryJournalInput)` (§2);
  расширить `seed.go` (6 счетов + 6 purposes, §9); `migrations/ledger/0002_inventory_
  accounts` (backfill существующих). Юнит-тест: инвентарный журнал балансируется,
  постится, отменяется reversal'ом.

**Inventory — новый контекст:**
- `internal/domain/inventory`: `Product` (+unit-конверсия, инварианты §3),
  `TechCard`/`TechCardLine`/`RecipeCosting` (calendar-versioning, cycle-DFS, costing
  §4), `StockMove`/`OnHand` (moving-average apply §5), `GoodsReceipt`/`WriteOff`/
  `Stocktake`/`Supplier` (lifecycle §6). Юнит-тесты: конверсия единиц, закрытие
  версии по backdate, цикл-guard, moving-average (приход/расход/недостаток/reversal),
  dry-run расчёт.
- `internal/inventory/ports`: `Store` (products, tech_cards, costings, moves, on_hand
  `FOR UPDATE`, receipts/writeoffs/stocktakes CRUD+post, suppliers, `InTx`,
  `WithTx`), `Ledger` (обёртка над `PostInventoryJournal`/`CancelJournal`/
  `AccountForPurpose`), `SalesReader` (для food cost).
- `internal/inventory/app`: use-cases §10 — `CreateProduct/Update/Archive`,
  `CreateTechCardVersion/Recost/ActiveOn`, `Create/Post/Cancel Receipt`,
  `…WriteOff`, `Start/EnterCounts/DryRun/Post/Cancel Stocktake`, `ConsumeForSale`
  (вызывается pos), `OnHand`, `StockMoves`, `FoodCostReport`.
- `internal/inventory/adapters/postgres` (+ `inventorydb` sqlc-генерат,
  `queries/inventory/inventory.sql`), `migrations/inventory/0001_init.*`.
- `internal/inventory/adapters/ledgerbridge`: `ports.Ledger` поверх `ledger/app`.

**Pos — минимальное расширение:**
- `pos/ports.Inventory { ConsumeForSale(ctx, tx, restaurantID, businessDate, lines) (cogsCents int64, err) }`.
- `pos/adapters/inventorybridge`: реализация поверх `inventory/app`.
- `pos/app.CloseTicket`: в существующей tx после записи payments — вызвать
  `Inventory.ConsumeForSale`; ошибка отсутствия карты — не фейл (пропуск).
  Никаких новых pos-таблиц/колонок.

**HTTP (в `platform/adapters/http` — существующая конвенция):** хендлеры §10
(products, tech-cards, suppliers, receipts, write-offs, stocktakes, on-hand,
stock-moves, food-cost) с гейтами storekeeper+/manager+.

**Роль `storekeeper`** — добавить в platform-роли (`domain.md` §3a: `users.role`
text, дешёвое расширение; статическая матрица прав в коде). Если раскатка ролей
отдельным слайсом ещё не сделана — временно гейтить склад ролью `manager+`, отметить
в DoD.

**Транзакции — где:**
- `PostReceipt` / `PostWriteOff` / `PostStocktake` — **1 tx**: moves + on_hand
  (`FOR UPDATE`) + `PostInventoryJournal`.
- `CancelX` — 1 tx: reversal-moves + on_hand + `CancelJournal`.
- `CloseTicket` — существующая tx **расширяется** COGS: payments + `closed_at` +
  `ConsumeForSale` (moves + on_hand + cogs-журнал). Долг 1 (атомарность) сохранён.
- `CreateTechCardVersion` — 1 tx: закрыть предыдущую версию (`valid_to=D`) + вставить
  новую + первая строка costing.

### Frontender

**`frontend/admin`** (новые экраны):
- **Номенклатура**: список/создание/архив продуктов, тип, базовая единица, привязка к
  позиции меню (для `dish`), min_stock.
- **Тех-карты**: редактор строк рецепта, `consumption`, поле `valid_from`, **лента
  версий** (интервалы валидности, текущая себестоимость), кнопка recost.
- **Поставщики**: справочник.
- **Накладные**: черновик (строки: продукт, кол-во+единица, цена за единицу, сумма),
  проведение, просмотр, отмена (reversal).
- **Инвентаризация**: старт, ввод пересчёта, **dry-run превью расхождений** (без
  сохранения), проведение, отмена.
- **Остатки**: таблица on-hand (кол-во, средняя, стоимость, подсветка ниже min);
  книга движений; **food cost отчёт** (факт vs теория, %, доля estimated).
- Типы (`frontend/admin/src/api/types.ts`): `Product`, `Unit`, `TechCard`,
  `TechCardLine`, `RecipeCosting`, `Supplier`, `GoodsReceipt`, `WriteOff`,
  `Stocktake`, `StockMove`, `OnHand`, `FoodCostRow`.

**`frontend/pos`** — без изменений (COGS невидим POS).

### Tester (интеграционные тесты)

**Refuted assumptions §15.2 / §15.3 как тест-кейсы:**
1. **§15.3 — ни один путь не мутирует проведённый факт (append-only держится).**
   Проведённое `stock_move` и проведённый инвентарный GL-документ не редактируются
   и не удаляются; повторный `post` документа → `409 already_posted`; коррекция
   только через `cancel`→reversal (зеркальные движения + GL сегодняшней датой,
   оригинал `cancelled`).
2. **§15.2 — «read-only» склад имеет только санкционированные write-пути; read без
   side-effect.** (а) `stock_on_hand` меняется **только** через проведение
   документа — прямого «adjust on-hand» эндпоинта нет; единственный путь коррекции
   остатка — **инвентаризация** (проведённый документ). (б) `dry-run`
   инвентаризации не создаёт движений/проводок и не меняет состояние (идемпотентен,
   вызвать дважды — одинаковый ответ, `stock_moves` пуст).

**Инварианты склада/тех-карт:**
- `Σ stock_moves.qty = stock_on_hand.qty` и `Σ signed(cost) = value_cents` по
  позиции (кэш равен фолду).
- Взвешенная средняя: приход 5000@30000 → приход 5000@40000 ⇒ средняя 7c/ед;
  расход по средней; недостаток остатка → `estimated=true`, `qty<0`, не блокирован.
- Reversal накладной откатывает on_hand точно по исходной стоимости (не по текущей
  средней).
- Backdate: пост документа с `business_date` раньше последнего движения по позиции
  → `422 backdated_before_last_move`.
- Конверсия: `kg` для g-позиции = ×1000; `kg` для pcs-позиции → `422`.

**Calendar-versioning (D5):**
- Backdated-создание версии на дату `D` закрывает предыдущую `valid_to=D`; активная
  на дату между интервалами резолвится однозначно; вторая версия с тем же
  `valid_from` → `409 version_exists`; ≤1 открытой (`valid_to IS NULL`) на продукт.
- Себестоимость — append-only: recost создаёт **новую** строку `recipe_costings`,
  предыдущая не переписана; текущая = последняя.
- Рецепт-цикл (A→B→A) при создании → `422 recipe_cycle`.

**COGS по продаже:**
- Закрытие тикета с dish (`assemble`) списывает ингредиенты по активной на
  бизнес-дату карте; GL `debit 5000 / credit 1200` на Σ cost; движения `sale`
  датированы бизнес-датой смены (не таймстемпом, refuted §15.4).
- `deplete_finished` списывает сам продукт.
- Идемпотентность: повторное закрытие того же тикета не задваивает движения
  (`source_event_id UNIQUE`).
- Позиция без тех-карты — продажа проходит, движений нет, не ошибка.

**Инвентаризация server-computed:**
- `expected_qty` фиксируется на post (продажа между dry-run и post меняет expected);
  недостача → `stocktake_shortage` + `debit 5910 / credit 1200`; излишек →
  `debit 1200 / credit 4910`.

**Тенант-изоляция:** ресторан A не видит номенклатуру/накладные/остатки/движения B.
GL инвентарных документов балансируется, несёт две даты и оба измерения
(`restaurant_id` + `cost_center_id`).

---

## 12. Definition of Done

- `migrations/inventory/0001` + `migrations/ledger/0002` применяются; sqlc-блок
  `inventory` генерирует `inventorydb`; `go build -C backend ./... && go vet -C
  backend ./... && go test -C backend ./...` — зелёные.
- `cmd/aivo-seed` и live-провижининг создают ресторану расширенный план счётов
  (6 новых счетов + map); backfill демо-тенанта отработал.
- **Номенклатура**: продукты с закрытым type-enum, базовыми единицами и конверсией;
  `dish → menu_item` без FK, ≤1 dish на позицию меню.
- **Тех-карты**: calendar-versioned (интервалы, ≤1/день, backdate закрывает
  предыдущую), цикл-guard, себестоимость — append-only ряд, `consumption` на карте.
- **Склад**: `stock_moves` append-only с двумя датами; взвешенная средняя;
  материализованный `stock_on_hand` == фолду движений; backdate раньше последнего
  движения запрещён; FIFO явно не сделан (зафиксирован анти-скоуп).
- **Документы**: накладная / списание / инвентаризация — `draft→posted→cancelled=
  reversal`; каждое проведение атомарно (moves + on_hand + GL в одной tx); dry-run
  инвентаризации без сохранения; авто-проводка излишков/недостач.
- **COGS**: закрытие тикета списывает по активной карте и постит `debit 5000 /
  credit 1200` отдельным документом в той же tx; идемпотентно; продажа без карты не
  падает.
- **Food cost отчёт**: факт vs теория + доля estimated.
- **GL**: каждый инвентарный документ балансируется, несёт две даты и измерения;
  маппинг purpose→счёт — конфиг (§15.6); отмена — reversal, оригинал не мутирован.
- Тесты §11 (refuted §15.2/§15.3 + инварианты + calendar-versioning + COGS +
  инвентаризация + тенант-изоляция) — зелёные. `npm run build` в `frontend/admin`
  зелёный.
- **Анти-скоуп соблюдён**: нет repost / FIFO / PurchaseOrder / EDI / landed cost /
  batch-expiry / мультивалюты / мульти-складов / received-not-billed-цепочки /
  рекурсивного списания. Backdate-запрет и single-currency помечены комментариями
  с upgrade-path.
- Обновлены: `PLATFORM.md` (эндпоинты §10), `CONTEXT.md` (новый контекст inventory +
  решения: weighted-average, calendar-versioning, синхронный COGS-порт,
  AP-вместо-RNB), `docs/adr/` — при желании ADR на «weighted average вместо FIFO» и
  «COGS синхронно, не событием».

---

## Отклонения (backender, инкремент-2)

Где реализация минимально отступила от буквы контракта — и почему.

1. **Количества — BIGINT милли-единицы, не NUMERIC(12,3).** Контракт (§3/§5) назвал
   `NUMERIC(12,3)` в базовой единице. Реализовано целочисленно: домен — `int64`
   милли, БД — `bigint` милли-единицы. Это точная реализация «cents-analog»
   (domain.md §0), устраняет NUMERIC-scan трение и совпадает с деньгами
   (`bigint` cents). Функционально эквивалентно (1 базовая единица = 1000 милли).

2. **Роль `storekeeper` отложена — склад гейтится `manager+`.** §11 явно разрешает
   этот фолбэк, если раскатка ролей отдельным слайсом не сделана. Все inventory-
   эндпоинты — `restaurant(true, …)` (owner/manager). Полноценная роль storekeeper
   + матрица прав — отдельный слайс (как и 6-ролевая матрица инкремента-1).

3. **`last_avg_cents` — cents за базовую единицу (не «×1000»).** §9 в примере
   показал `last_avg=6000`, посчитав `value×1000/qty` с qty в **базовых** единицах;
   реализация держит qty в **милли** повсюду (как §5 и говорит), поэтому
   `last_avg = value×1000/qty_milli = 6` (cents/база), самосогласованно. Основной
   путь стоимости расхода считается напрямую из `(value, qty)` без промежуточного
   округления средней (точнее, чем «unit затем ×q/1000»); `last_avg` используется
   только для оценки при неположительном остатке. Тестируемые числа (средняя 7,
   расход 200г@6=1200) воспроизводятся.

4. **Per-item «фактический» COGS в food-cost — на уровне итога, не строки.** §8/§10
   просят `actual_cogs_cents` в строке позиции, но sale-движение висит на общем
   ингредиенте и чисто на одну позицию меню не раскладывается без кросс-контекстного
   джойна `stock_moves ⨝ ticket_lines` (запрещён ddd §4). Реализовано: **теоретический**
   COGS — per-item (из активной карты), **фактический** — в `totals`
   (Σ|cost| sale-движений) + доля estimated. Upgrade-path: тегировать sale-движение
   `menu_item_id` (аддитивно) для per-item факта.

5. **Теоретический COGS — по активной карте на конец периода, не по дате каждой
   продажи.** §8 говорит «на дату продажи». `SoldDishes` отдаёт суммарное qty за
   период, а не по-датам, поэтому теоретическая себестоимость берёт активную карту
   на `to`. Для сдвига цен внутри периода — задокументированное упрощение (минимальный
   отчёт). Upgrade-path: агрегировать продажи по датам.

6. **`stock_unit` не редактируется через API вовсе.** §10 PATCH-тело не содержит
   `stock_unit`, поэтому инвариант «неизменяем после первого движения» держится **по
   отсутствию пути мутации** (как freeze счёта в инкременте-1). `ErrUnitLocked` и
   код `unit_locked` не заведены; вернуть вместе с эндпоинтом смены единицы.

7. **`storage_id` / мульти-склад — не заведены** даже как колонка-заготовка (§1.7
   «добавится позже без ломки схемы»). Аддитивно, добавится при появлении требования.

Проверка: `go build/vet/test -C backend ./...` зелёные (юнит-тесты домена: конверсия
единиц, взвешенная средняя приход/расход/недостаток/reversal, fold-инвариант,
интервалы D5, цикл-guard, recipe cost, bankRound; + интеграционные с `DATABASE_URL`:
средняя+GL прихода/списания/инвентаризации, backdate-запрет, точный reversal,
calendar-versioning+цикл, COGS+идемпотентность, dry-run без движений). Инвентарный
GL-журнал (`ledger.PostInventoryJournal`) покрыт интеграционно через посты
документов; reversal — через `CancelReceipt`/`CancelJournalForSource`.
