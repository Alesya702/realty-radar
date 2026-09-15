# Core MVP — фактическое состояние реализации

> **Статус: реализовано (2026-09-15).** Документ описывает только то, что есть в коде.
> Целевая модель — [core-domain-model.md](core-domain-model.md). Решения — ADR-0006, ADR-0007
> (приняты; в MVP реализованы частично). Следующий этап — импорт PDF
> ([pdf-extraction.md](pdf-extraction.md)); он **не реализован**.

## 1. Что реализовано

Вертикальный срез модуля Core:

- единая модель рынка: девелоперы, проекты (собственные и внешние), корпуса, секции, лоты;
- принадлежность проекта портфелю компании и направленные отношения проектов (прямой / косвенный
  конкурент, референс);
- реестр источников данных (идентичность источника);
- размещение лота в источнике и история его состояния (версии только при изменении,
  `last_observed_at`, состояние на момент);
- входной контракт `IngestObservationBatch` (MVP-версия) — через него работают ручной ввод в UI и
  демонстрационные данные, к нему подключится импорт PDF;
- миграция схемы `core` с ограничениями целостности;
- минимальный SSR-интерфейс для ручной проверки;
- демонстрационные данные (`realtyradar demo`);
- unit-, интеграционные и архитектурные тесты.

## 2. Сущности и таблицы

Миграция: `migrations/20260915160000_core_mvp_model.sql` (схема `core`, создана миграцией
`20260915120000_core_create_schema.sql`).

| Сущность (Go, `internal/domain`) | Таблица | Ключевые поля и ограничения |
|---|---|---|
| `Developer` | `developers` | `name` не пустое |
| `Project` | `projects` | `developer_id` → developers (nullable), `name`, `city` (nullable) |
| `Building` | `buildings` | `project_id` NOT NULL; `UNIQUE (project_id, name)` |
| `Section` | `sections` | `building_id` NOT NULL; `UNIQUE (building_id, name)` |
| `DataSource` | `data_sources` | `code` UNIQUE и `CHECK` формата; `kind` ∈ `manual`, `document_channel`; `is_active`. Миграция создаёт источник `manual` («Ручной ввод») |
| `Lot` (+ `Placement`) | `lots` | `project_id` NOT NULL; `building_id`, `section_id`, `floor` nullable; `lot_type` ∈ `apartment`, `commercial`, `storage`, `parking`; `number` NOT NULL; составные FK: корпус — из проекта лота, секция — из корпуса лота; `CHECK (section_id IS NULL OR building_id IS NOT NULL)`; `primary_listing_id` → собственное размещение лота |
| `Listing` | `lot_listings` | `lot_id`, `data_source_id`; `source_key`, `source_url`, `published_at` nullable; `first_seen_at`, `last_seen_at`; `UNIQUE (lot_id, data_source_id)`, `UNIQUE (data_source_id, source_key)` |
| `IngestionBatch` | `ingestion_batches` | `data_source_id`; `origin_kind` ∈ `manual_entry`, `document_import`; `origin_ref` (без FK, идемпотентность); `observed_at`, `received_at`; `UNIQUE (origin_kind, origin_ref)` |
| `Version` (+ `LotState`) | `lot_listing_versions` | `[valid_from, valid_to)`, `last_observed_at`; `presence` ∈ `listed`, `delisted`; `status` ∈ `available`, `reserved`, `sold`, `unknown`; `rooms` ≥ 0; `area_total numeric(10,2)` > 0; `price numeric(15,2)` > 0; `price_before_discount` только вместе с `price` и больше неё; `finish` text; `batch_id` NOT NULL; одна текущая версия (частичный уникальный индекс); `CHECK` границ интервала |
| `PortfolioMembership` | `portfolio_memberships` | `project_id`; `valid_from`, `valid_to` (date, nullable); `note` |
| `ProjectRelation` | `project_relations` | `subject_project_id`, `related_project_id`, `relation_type` ∈ `direct_competitor`, `indirect_competitor`, `reference`; `rationale`; `valid_from` NOT NULL, `valid_to`; `CHECK (subject <> related)` |

Не создано: `phases`, ссылки источников на проекты и корпуса, история внешних ключей лота,
справочники класса проекта, отделки, видов и особенностей, условия продаж, опубликованные
представления для других модулей (п. 9).

Технические решения реализации:

- **ID** — UUIDv7, генерируются в `domain.NewID` (стандартная библиотека, без новой зависимости).
- **Деньги и площади** — `domain.Money` (копейки, int64) и `domain.Area` (сотые м², int64). В БД
  передаются как десятичный текст `numeric`, без float (A-08).
- **SQL** — написан вручную на pgx в `internal/adapters/postgres`. sqlc в Core MVP **не подключён**:
  запросов немного, генератор добавил бы инструмент в сборку. Направление ADR-0002 сохраняется и
  пересматривается при росте числа запросов.
- **Время** — `timestamptz` UTC; UI показывает и принимает время Europe/Moscow (A-07).

## 3. Устройство модуля

```
src/modules/core/
  module.go                        публичный контракт: core.New, RegisterRoutes, Nav, типы-алиасы, use cases
  internal/domain/                 сущности и правила (чистый Go)
  internal/app/                    use cases, порт Store/Repository, контракт ObservationBatch
  internal/adapters/postgres/      репозиторий pgx
  internal/adapters/http/          страницы (html/template) и шаблоны
src/web/                           общий layout (web.Renderer), стартовая страница
src/app/app.go, demo.go            связывание модуля; демо-данные
```

Архитектурный тест дополнительно проверяет, что `internal/app` не импортирует адаптеры своего
модуля.

## 4. Use cases (публичный Go-контракт)

`core.Module` встраивает `*app.Service`, поэтому его методы — публичный контракт для других
модулей. Типы доступны через алиасы корневого пакета (`core.ID`, `core.Money`,
`core.ObservationBatch`, …).

| Группа | Методы |
|---|---|
| Девелоперы | `CreateDeveloper`, `GetDeveloper`, `ListDevelopers` |
| Проекты | `CreateProject` (с девелопером или без), `SetProjectDeveloper`, `GetProject`, `ListProjects(on)` (с девелопером, признаком портфеля на дату, числом лотов) |
| Структура | `AddBuilding`, `AddSection`, `ListBuildings` (с секциями) |
| Источники | `RegisterDataSource`, `DataSourceByCode`, `ListDataSources` |
| Портфель | `AddProjectToPortfolio`, `ListPortfolioMemberships`, `ListPortfolioProjects(on)` |
| Отношения | `EstablishProjectRelation`, `ListProjectRelations(subject, on)` |
| Лоты | `IngestObservationBatch` (создание лота и запись состояния), `GetLot`, `ListLots(project)` (с текущим состоянием), `GetLotHistory` (размещения, версии, `Current()`, `At(t)`), `GetLotState(lot, t)` |

Нарушение правил возвращается как `*core.RuleError` со стабильным кодом (`must_be_positive`,
`price_before_discount_not_greater`, `lot_already_exists`, `observation_out_of_order`, …).
Отсутствующий объект — ошибка, оборачивающая `core.ErrNotFound`.

## 5. Входной контракт `IngestObservationBatch` (MVP)

```
ObservationBatch
  DataSourceID          источник (обязателен, активен)
  OriginKind            manual_entry | document_import
  OriginRef?            ID импорта в модуле-отправителе; повтор не применяется (AlreadyApplied)
  ObservedAt            момент, к которому относятся данные (не в будущем)
  ProjectID             проект (выбран пользователем)
  Buildings[]           Ref → ExistingID | NewName        (подтверждённое решение)
  Sections[]            Ref → ExistingID | BuildingRef + NewName
  Items[]
    Ref
    Target              ExistingLotID | NewLot{BuildingRef?, SectionRef?, Type, Number, Floor?}
    SourceKey?, SourceURL?, PublishedAt?
    State               ObservedState: Presence, Status, Rooms, AreaTotal, Price,
                        PriceBeforeDiscount, Finish — каждое поле Value | Unknown | Absent
→ BatchResult{BatchID, AlreadyApplied, Items[{Ref, LotID, ListingID, LotIsNew, Change, VersionID}]}
```

- Пакет применяется **атомарно**. Ошибка элемента возвращается как `*core.ItemError{Ref, Err}`.
- Решение о сопоставлении передаёт отправитель (пользователь): существующий лот или новый. **Fuzzy
  matching нет.** Новый лот с точно таким же размещением (проект, корпус, секция, тип, номер)
  отклоняется (`lot_already_exists`), чтобы подтверждённое «новый» не создало дубликат молча.
- `Unknown` сохраняет прежнее значение, `Absent` очищает его (например, скидка закончилась).
- Цена — актуальная цена предложения на момент наблюдения. `PriceBeforeDiscount` — только при
  явно указанной скидке, иначе пусто; скидка и цена за м² не хранятся.

Точка подключения импорта PDF: после подтверждения импорта Collection вызывает
`IngestObservationBatch` с `OriginKind = document_import` и `OriginRef` = ID подтверждённого
импорта. Проект создаётся заранее `CreateProject`, корпуса и секции — решениями в пакете.

## 6. История лота

| Ситуация | Реализация |
|---|---|
| Первое наблюдение | создаются размещение (лот × источник) и первая версия `[t, ∞)`; `first_seen_at = last_seen_at = t`; размещение становится основным для лота |
| Те же значения | новой версии нет; `last_observed_at` версии и `last_seen_at` размещения сдвигаются |
| Значения изменились | текущая версия закрывается в `t`, открывается новая |
| Наблюдение раньше текущей версии | отклоняется (`observation_out_of_order`) |
| Другие значения не позже последнего подтверждения | отклоняется (`history_conflict`) |
| Текущее состояние | версия основного размещения с `valid_to IS NULL` |
| Состояние на момент T | версия с `valid_from ≤ T < valid_to` |
| «Снят с публикации» | задаётся явно в наблюдении (`presence = delisted`); из отсутствия не выводится |

Пример (демо-данные, лот 145): 01.09 — 15 900 000 ₽ (версия 1); 08.09 — те же значения
(подтверждение, версий по-прежнему одна); 12.09 — 16 400 000 ₽ (версия 2). Состояние на 10.09 —
15 900 000 ₽, последнее подтверждение — 08.09.

## 7. Интерфейс

SSR (`html/template`) без htmx и JavaScript: формы и переходы PRG. При ошибке формы страница
показывается повторно со статусом 422, сообщением и введёнными значениями.

| Маршрут | Экран / действие |
|---|---|
| `GET /` | стартовая страница |
| `GET /projects`, `POST /projects` | список проектов (девелопер, город, роль, число лотов), создание проекта |
| `GET /projects/{id}` | карточка проекта: девелопер, портфель, отношения, корпуса и секции, лоты с текущим состоянием и ценой за м², форма нового лота |
| `POST /projects/{id}/developer` · `/portfolio` · `/relations` · `/buildings` · `/sections` · `/lots` | действия карточки проекта |
| `GET /lots/{id}[?at=YYYY-MM-DDTHH:MM]` | карточка лота: размещение, текущее состояние, состояние на дату, история с отметкой изменённых полей, размещения в источниках, форма нового наблюдения |
| `POST /lots/{id}/observations` | новое наблюдение (форма заполнена текущими значениями; очищенное поле — значение отсутствует) |
| `GET /portfolio` | проекты портфеля компании на сегодня |
| `GET /developers`, `POST /developers` | девелоперы |
| `GET /data-sources`, `POST /data-sources` | источники данных |

Ручной ввод в UI использует тот же `IngestObservationBatch` (`OriginKind = manual_entry`) —
отдельного пути записи в историю в обход контракта нет.

## 8. Демонстрационные данные

`go run ./src/cmd/realtyradar demo` (повторный запуск ничего не меняет). Данные вымышленные:

- «Девелопер А (демо)» → «ЖК «Альфа» (демо)» — проект компании (портфель с 01.01.2026), Москва;
  корпус «Корпус 1» с секцией «1»;
  - лот 145 (Корпус 1, без секции, 7 этаж, 2 комнаты, 67,4 м²): 15 900 000 → 16 400 000 ₽;
  - лот 146 (Корпус 1, секция 1, 1 комната, 41,2 м²): 11 200 000 ₽ при цене до скидки
    11 800 000 ₽; 12.09 — забронирован;
- «Девелопер Б (демо)» → «ЖК «Бета» (демо)» — внешний проект; лот 12;
- отношение: «Бета» — прямой конкурент для «Альфа».

## 9. Сознательно отложено

| Что | Почему / когда |
|---|---|
| Phase (очередь) | не нужна для вертикального среза; добавляется `phases` + `buildings.phase_id` без изменения модели (Q-CORE-01) |
| Поиск кандидатов `FindMatchCandidates`, состояния сопоставления | нужен интерфейсу проверки импорта PDF — следующий этап |
| Основание момента данных, область и полнота пакета, автоматическое `delisted` по отсутствию, порог промахов | нужны при полных перечнях (Q-CORE-17, Q-PDF-06); сейчас исчезновение задаётся только явно |
| Вставка неупорядоченных (более ранних) наблюдений | в MVP отклоняются (Q-CORE-27) |
| `CorrectBatchData`, отзыв пакета, слияние лотов и структуры, перенос размещения, выбор основного размещения | следующий этап (Q-PDF-11, Q-CORE-18) |
| История внешних ключей (`LotListingKey`), ссылки источников на проекты и корпуса | при появлении источников с ID |
| Исправление размещения существующего лота (номер, этаж, корпус) | не реализовано; нужна операция исправления |
| Класс проекта, район, срок сдачи, адрес и координаты | ждут ответов Q-CORE-04, 13, 12; координаты — с Location |
| Виды из окон, особенности, признак акции, условия продаж | справочники и модель не утверждены (Q-CORE-08, Q-CORE-10 — LATER) |
| Опубликованные SQL-представления Core | вместе с Offer Analytics |
| Изменение и удаление проектов, девелоперов, отношений (закрытие периода), выход из портфеля | не требовались срезом |
| Аутентификация, CSRF-защита форм | пользователи не определены (Q-GEN-01, A-02) |

## 10. Известные ограничения

- Пакет, который только подтвердил значения (без новой версии), сохраняется в
  `ingestion_batches`, но с версией не связывается. Какой пакет последним подтвердил значения,
  по данным Core не узнать; видно только `last_observed_at`.
- Каноническое состояние лота — история основного размещения (первого источника лота).
  Сведения других источников хранятся в их размещениях, но в карточке лота не показываются.
- Уникальность номера лота в БД не обеспечивается (нумерация бывает по секциям). Точный дубликат
  размещения проверяет приложение.
- Цена за м² считается от общей площади (вид площади — Q-CORE-09).

## 11. Проверка

```bash
bash scripts/check.sh                      # gofmt, vet, unit и архитектурные тесты
RR_TEST_DATABASE_URL=... bash scripts/test-integration.sh
```

| Тесты | Что покрывают |
|---|---|
| `internal/domain/*_test.go` | ID, деньги и площади, цена за м², размещение (необязательные уровни, несогласованная иерархия), валидация цены и цены до скидки, применение наблюдений (первое, изменение, повтор без версии, unknown/absent, раньше текущего, конфликт), состояние на момент, отношения, портфель |
| `internal/adapters/http/format_test.go` | форматирование, сообщения для всех кодов ошибок |
| `tests/integration/core_test.go` | иерархия, лоты без корпуса и секции, атомарность пакета, история и состояние на дату, валидация цены (включая `CHECK` в БД), идемпотентность, отношения и портфель, составные FK, демо-данные |
| `tests/integration/core_ui_test.go` | сценарий UI: девелопер → проект → корпус → секция → лот → наблюдение → история → состояние на дату; ошибки форм; портфель и отношения; источники |
| `tests/architecture` | правила зависимостей, включая запрет `internal/app` → адаптеры |
