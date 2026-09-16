# Realty Radar

Система сбора, хранения, анализа и сравнения данных по рынку новостроек и конкурентным
жилым проектам.

## Для чего

Аналитикам и продуктовой команде девелопера нужна единая база по рынку:

- текущие цены и статусы лотов конкурентов и их изменение во времени;
- квартирография и структура предложения с BI-срезами по фильтрам;
- анализ планировочных решений;
- сравнение продуктовых характеристик ЖК;
- анализ локации и доступности;
- данные проектных деклараций с наш.дом.рф: общий, проданный и остаточный объём.

Бизнес-требования — [исходное ТЗ](docs/requirements/source/tz-text.md) (оригинал .docx в той
же папке), границы системы — [scope.md](docs/requirements/scope.md).

## Текущее состояние

**Реализован Core MVP** — единая модель рынка и история лотов с минимальным интерфейсом
([фактическое состояние](docs/architecture/core-mvp.md)).

| Есть и работает | Нет (следующие этапы) |
|---|---|
| Core MVP: девелоперы, проекты (собственные и внешние в одной модели), корпуса, секции, лоты; портфель компании; отношения проектов (прямой / косвенный конкурент, референс); источники данных | Импорт PDF с AI-извлечением (**следующий этап**) |
| История лота: версия только при изменении, текущее состояние, состояние на дату, последнее подтверждение | Очереди, сопоставление импортируемых объектов, слияния, исправление импортов |
| Входной контракт `IngestObservationBatch` (ручной ввод; к нему подключится импорт PDF) | Автоматический сбор с сайтов (future scope) |
| Страницы: проекты, карточка проекта, карточка лота с историей, портфель, девелоперы, источники; демо-данные | Аналитика, дашборды, графики; планировки, бенчмарк, локация, декларации |
| Конфигурация `RR_*` / `.env`, PostgreSQL, миграции (goose), `/healthz`, `/readyz` | Авторизация |
| Unit-, архитектурные и интеграционные тесты; CI (GitHub Actions) | |

**Scope MVP (2026-09-15):** Core, импорт PDF с AI-извлечением и обязательной проверкой
пользователем, ручной ввод. Автоматический сбор с сайтов конкурентов — future scope
([scope.md](docs/requirements/scope.md) п. 3.1–3.2,
[ADR-0008](docs/decisions/0008-mvp-document-import-ai-extraction.md) — Принят).

## Архитектура

Модульный монолит на Go: один бинарник, одна БД PostgreSQL, бизнес-модули с явными
границами вокруг центрального модуля Core.

```
 offeranalytics ─┐
 collection ─────┤
 declarations ───┼──► core        по умолчанию модули зависят только от публичных контрактов
 floorplans ─────┤                Core; Core — ни от кого; связи между модулями — по ADR
 benchmark ──────┤
 location ───────┘

 src/app        связывает модули и инфраструктуру (composition root)
 src/platform   config, logging, database, migrate, httpserver — без бизнес-логики
 src/web        UI-оболочка (серверный рендеринг)
 PostgreSQL     одна БД, схема на модуль
```

| Модуль | Назначение |
|---|---|
| `core` | Девелоперы, ЖК, очереди, корпуса, секции, лоты, конкуренты, портфель, источники, история лотов — **Core MVP реализован** |
| `offeranalytics` | Квартирография, обязательные и ценовые показатели, сравнение проектов |
| `collection` | Импорт PDF с AI-извлечением (MVP, следующий этап); в будущем коллекторы сайтов |
| `declarations` | Проектные декларации и наш.дом.рф, ежемесячные срезы |
| `floorplans` | Анализ планировок (PDF, изображения, распознавание) |
| `benchmark` | Продуктовые характеристики и их оценка |
| `location` | POI, расстояния, время в пути, барьеры |

Подробно: [обзор архитектуры](docs/architecture/overview.md), [модули](docs/architecture/modules.md).

## Стек

- **Go 1.27**, стандартные `net/http`, `html/template`, `log/slog`
- **PostgreSQL 17** (Docker-образ с PostGIS), драйвер **pgx v5**
- **goose v3** — SQL-миграции, встроенные в бинарник
- **godotenv** — загрузка `.env` при локальной разработке
- Docker Compose (локальная БД), GitHub Actions (CI)

Планируется по мере реализации модулей: sqlc (в Core MVP SQL написан вручную на pgx), htmx,
Apache ECharts, очередь задач на PostgreSQL, файловое хранилище, PostGIS; headless-браузер —
только для будущего сбора с сайтов (future scope). Обоснования и альтернативы —
[ADR-0002](docs/decisions/0002-tech-stack.md).

UI ([ADR-0005](docs/decisions/0005-ui-approach-and-bi.md)): решение для MVP — серверный
рендеринг + htmx, с контрольной точкой при реализации Offer Analytics. Если интерактивная
аналитика потребует значительного клиентского состояния, вариант React/TypeScript
пересматривается отдельным ADR. Yandex DataLens
зафиксирован как допустимый внешний BI поверх опубликованных представлений; решение об
использовании не принято.

## Структура каталогов

```
.
├── src/
│   ├── cmd/realtyradar/      точка входа (serve, migrate, demo, version)
│   ├── app/                  composition root
│   ├── platform/             config, logging, database, migrate, httpserver
│   ├── web/                  UI-оболочка и шаблоны
│   └── modules/              бизнес-модули (core — Core MVP; остальные — границы, doc.go)
├── migrations/               SQL-миграции (goose)
├── tests/
│   ├── architecture/         проверка правил зависимостей
│   └── integration/          тесты с PostgreSQL (build tag integration)
├── scripts/                  check.sh, test-integration.sh
├── docker/                   Dockerfile приложения, init-скрипты PostgreSQL
├── docs/
│   ├── architecture/         overview, modules, core-mvp, core-domain-model, pdf-extraction
│   ├── requirements/         scope, open-questions, assumptions, source/ (ТЗ)
│   └── decisions/            ADR
├── .github/workflows/ci.yml
├── docker-compose.yml
├── Makefile                  необязательные обёртки над командами ниже
├── .env.example
├── CLAUDE.md                 правила разработки (для Claude Code и людей)
└── go.mod
```

## Установка

Требуется:

- Go 1.27+ — <https://go.dev/dl/>
- PostgreSQL 17: через Docker (`docker compose`) или локальная установка
- Bash для скриптов (на Windows — Git Bash)

```bash
go mod download
```

## Настройка окружения

```bash
cp .env.example .env
```

| Переменная | По умолчанию | Описание |
|---|---|---|
| `RR_ENV` | `local` | `local` / `test` / `production` |
| `RR_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `RR_LOG_FORMAT` | `text` | `text` / `json` |
| `RR_HTTP_ADDR` | `127.0.0.1:8080` | адрес HTTP-сервера (по умолчанию только localhost) |
| `RR_HTTP_SHUTDOWN_TIMEOUT` | `10s` | время на корректную остановку |
| `RR_DATABASE_URL` | — (обязательна) | строка подключения PostgreSQL |
| `RR_DATABASE_MAX_CONNS` | `10` | размер пула соединений |
| `RR_TEST_DATABASE_URL` | — | отдельная БД для интеграционных тестов (не используется приложением) |

Переменные окружения процесса имеют приоритет над `.env`.

## Запуск

```bash
# 1. База данных (создаёт БД realtyradar и realtyradar_test)
docker compose up -d postgres

# 2. Миграции
go run ./src/cmd/realtyradar migrate up
go run ./src/cmd/realtyradar migrate status

# 3. Демонстрационные данные (необязательно; вымышленные проекты и лоты, повторный запуск ничего не меняет)
go run ./src/cmd/realtyradar demo

# 4. Приложение
go run ./src/cmd/realtyradar serve
```

Проверка:

- <http://localhost:8080/> — стартовая страница
- <http://localhost:8080/projects> — проекты; из карточки проекта — корпуса, секции, лоты, портфель и конкуренты; из карточки лота — текущее состояние, история, состояние на дату и новое наблюдение
- <http://localhost:8080/healthz> — приложение живо: `{"status":"ok","version":"dev"}`
- <http://localhost:8080/readyz> — БД доступна: `200` или `503`

Сборка бинарника: `go build -o bin/realtyradar ./src/cmd/realtyradar`.
Всё в Docker: `docker compose --profile app up -d --build`, миграции —
`docker compose --profile app run --rm app migrate up`, демо-данные —
`docker compose --profile app run --rm app demo`.

Без Docker: подойдёт любой PostgreSQL 17. Создайте базы `realtyradar` и `realtyradar_test` и
укажите их в `RR_DATABASE_URL` и `RR_TEST_DATABASE_URL`.

## Тесты

```bash
# gofmt + go vet + unit и архитектурные тесты (без БД)
bash scripts/check.sh

# интеграционные тесты: применяют миграции и откатывают их до нуля — только тестовая БД!
export RR_TEST_DATABASE_URL=postgres://realtyradar:realtyradar@localhost:5432/realtyradar_test?sslmode=disable
bash scripts/test-integration.sh
```

То же без скриптов: `go test ./...` и `go test -tags integration ./tests/integration/...`.

## Документация

| Что | Где |
|---|---|
| Исходное ТЗ | [docs/requirements/source/](docs/requirements/source/) |
| Границы системы | [docs/requirements/scope.md](docs/requirements/scope.md) |
| Открытые вопросы к бизнесу | [docs/requirements/open-questions.md](docs/requirements/open-questions.md) |
| Допущения | [docs/requirements/assumptions.md](docs/requirements/assumptions.md) |
| Архитектура | [docs/architecture/overview.md](docs/architecture/overview.md), [modules.md](docs/architecture/modules.md) |
| Core MVP — что реализовано | [docs/architecture/core-mvp.md](docs/architecture/core-mvp.md) |
| Доменная модель Core (целевая, утверждена) | [docs/architecture/core-domain-model.md](docs/architecture/core-domain-model.md) |
| Импорт PDF с AI-извлечением, MVP (утверждён, не реализован) | [docs/architecture/pdf-extraction.md](docs/architecture/pdf-extraction.md) |
| Архитектурные решения (ADR) | [docs/decisions/](docs/decisions/README.md) |
| Правила разработки | [CLAUDE.md](CLAUDE.md) |
