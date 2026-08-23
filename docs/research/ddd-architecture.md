# DDD-архитектура AIVO: синтез (кратко)

Выжимка из трёх отчётов (`platform.md`, `menu.md`, `pos.md`). Полная версия — в git-истории этого файла.

## 1. Контекст-карта

```
Platform (upstream: тенантность, auth, лимиты)
   │ customer-supplier          │ customer-supplier
   ▼                            ▼
 Menu ◄──────────────────────── POS      POS conformist: живёт в типах
 (upstream для POS:             menu-домена; menubridge — полу-ACL
  каталог, handoff)             (транслирует ошибки, типы — нет)

 sharedkernel (ID, Entity, AggregateRoot, DomainEvent) — общий для всех
```

Решения:

- **`restaurants` владеет Platform** (жизненный цикл, провижининг, событие `RestaurantProvisioned`). Menu/POS держат проекции-ссылки. Новые колонки — только в `migrations/platform/`; `CREATE TABLE` физически не переносить (сломает порядок батчей).
- **Кросс-контекстные ссылки — без FK** (`customer_id` в `cart_handoffs`, `tickets` уже так). Существующий FK на `orders.customer_id` не трогать, но кодом на него не полагаться.
- **Handoff** — published contract: Menu владеет агрегатом и инвариантами (single-use, TTL 15м, ≤1 активного на стол); `HandoffLine` несёт снапшот для превью + source-IDs для перевалидации. `CreateHandoff` → `menu/app`, `AcceptHandoff` (сага consume→AddLines→компенсация) → `pos/app` — сейчас всё это лежит в HTTP-адаптере platform (`handoff.go`).
- **menubridge** остаётся полу-ACL (конформизм признан): обязательна только трансляция ошибок и статус единственной точки pos→menu. Полный ACL — если menu-изменения ломают POS чаще раза в квартал.
- **`pkg/session` импортирует menu-домен** — единственное нарушение уровня pkg→domain, убрать (шаг 1 миграции).

## 2. Shared kernel

Состав правильный и финальный: `ID`, `Entity`, `AggregateRoot`, `DomainEvent`. Единственный допустимый кандидат — `type Cents int`. Запрещено: бизнес-типы контекстов, конкретные события, порты/сентинели, утилиты, generic-библиотеки «на вырост». Kernel меняется только с согласия всех трёх контекстов — потому маленький.

## 3. Агрегаты (корень → главный инвариант → что чинить)

| Ctx | Агрегат | Инвариант | Дыра сейчас |
|---|---|---|---|
| P | Subscription | state machine, canceled терминален | `ChangePlan` собирает структуру мимо `Transition` (`platform/app/app.go:282`) |
| P | User | owner ⇒ без RestaurantID, staff ⇒ с ним | конвенция → фабрики `NewOwner`/`NewStaff` |
| P | Restaurant | валидный slug/hostname, лимиты плана | ок; regex из app → VO в домен |
| P | RestaurantTheme | accent enum, CSS-injection guard | ок (`App.SaveTheme` — единая точка) |
| P | AssistantThread | decide-once, allowlist действий | логика в хендлере → `Thread.DecideMessage` |
| P | Organization, Customer, GuestProfile | — | ок |
| M | **Order** (+ снапшот-строки) | строки валидны на момент подачи, write-once | `NewOrderLine` не проверяет single/multi-группы и дубли опций (`domain/menu/domain.go:187`); нет CHECK `price_cents >= 0` |
| M | **Handoff** | single-use, TTL, ≤1 активного | исполнение в чужом адаптере (см. §1) |
| M | MenuItem, Menu, Category, Table, ServiceRequest | один default-menu, уникальный токен, ≤1 pending запроса — всё держит БД (правильно) | lifecycle ServiceRequest — строка без guard'а; дубль дедупа в `pkg/session` |
| POS | **Shift** | одна открытая на ресторан; иммутабельна после закрытия (`Shift.Close` — лучший инвариант системы) | **sales считается вне транзакции закрытия — гонка потери денег** (`pos/app/app.go:127-155`) |
| POS | **Ticket** (+ строки) | один открытый на стол; закрытый иммутабелен | `FireTicket`/`AppendTicketNote`/`LinkTicketCustomer` не проверяют `status='open'` |

Кросс-агрегатные уникальности (default-menu, open shift/ticket, pending request, active code) — правильно живут в partial unique индексах БД, в агрегаты не тащить.

## 4. События vs прямые вызовы

Outbox (`events` таблица, sqlc-запросы, `AggregateRoot`) существует, но **мёртв**: ни Raise, ни записей, ни паблишера.

Правило: **нужен ответ другого контекста внутри use case — синхронный порт; другой контекст лишь реагирует на факт — событие.** Никогда оба сразу.

- Синхронно (есть, остаётся): POS→Menu (menubridge), Platform→Menu (лимиты, default menu), POS→Platform (auth).
- События — писать в outbox в транзакции команды. Первое проводное — с реальным потребителем: `AssistantActionsApplied` (аудит AI вместо slog) или `OrderPlaced` (CRM вместо прямого SQL по чужим таблицам). Паблишер — только с первым внешним консьюмером.
- `docs/EVENTS.md` поправить сразу (дёшево): убрать `code` из `HandoffCreated` (секрет в outbox), добавить `ServiceRequestClosed`, `TicketLinesAdded`, `AssistantActionsApplied/Discarded`, обогатить payload'ы Shift-событий.
- Если `AggregateRoot` не приживётся за два события — удалить его и писать outbox прямо из app.

## 5. Целевая структура и миграция

Домен переезжает внутрь контекстов: `internal/<ctx>/domain` вместо `internal/domain/<ctx>`.

Правила импортов: domain → только sharedkernel+stdlib; app → свой domain/ports; чужой контекст — только через bridge-адаптеры; `pos → menu/domain` легален (конформизм), `menu → pos` и `menu/pos → platform` запрещены; `pkg/*` — без `internal/*`.

Шаги (каждый — коммит, build+test зелёные):

1. Развязать `pkg/session` от домена: дедуп сервис-запросов через БД-индекс/`HasOpenServiceRequest` → 409 (чинит два бага).
2. Мёртвый код: либо мигрировать сторы на sqlc-генераты (`menudb`/`posdb`), либо удалить их — не держать оба слоя; легаси `menu/adapters/http` удалить.
3–5. Переезд domain-пакетов (menu, pos, platform) — механика, `gopls rename`.
6. `AcceptHandoff` → `pos/app` (+3 метода в `pos/ports.Menu`, реализация в menubridge).
7. `CreateHandoff` → `menu/app`.
8. `AssistantService` → `platform/app`.
9. Лимит блюд плана → `platform/app` (`EnsureCanCreateItem`).
10. `TouchGuest` в одну точку app-слоя.

## 6. Топ-10 по приоритету

1. **Атомарный `CloseShift`** — одна транзакция: lock смены → закрыть тикеты → агрегат продаж → `Shift.Close`. Money path.
2. Дедуп сервис-запросов через БД + развязка `pkg/session` (= шаг 1 миграции).
3. Иммутабельность закрытого тикета: `AND status='open'` в три UPDATE + RowsAffected.
4. `Subscription.ChangePlan` через `Transition`; типизированный конфликт слага вместо `strings.Contains(err, "slug")`.
5. `NewOrderLine`: single/multi-группы, запрет дублей опций, cap qty; CHECK `price_cents >= 0`.
6. Handoff по местам (шаги 6–7).
7. Судьба sqlc-генератов + удалить легаси menu/http (шаг 2).
8. Переезд доменов + зафиксировать решения §1 в `CONTEXT-MAP.md`.
9. Ассистент и лимит блюд → app (шаги 8–9).
10. Правки `EVENTS.md` + первое проводное событие.

Не делать: Money-VO с валютами, generic-репозитории, event bus поверх пустого outbox, полный ACL, gRPC/микросервисы (ADR 0001), физический перенос `CREATE TABLE restaurants`.
