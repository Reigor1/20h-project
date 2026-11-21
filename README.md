# 20h-project

Backend-проект на Go, предназначенный для изучения практик разработки серверных приложений, ориентированных на эксплуатацию в production: проектирования REST API, транзакций PostgreSQL, кэширования в Redis, асинхронной обработки событий через Kafka и наблюдаемости.

Приложение управляет пользователями и задачами. Изменения задач сохраняются в PostgreSQL и асинхронно публикуются через Transactional Outbox. Отдельный Activity Worker получает события из Kafka и формирует историю изменений задач.

> Репозиторий является учебным проектом и проектом для портфолио. Архитектура демонстрирует практики надёжности и эксплуатации, однако локальная среда Docker Compose предназначена для разработки, а не для развёртывания в production.

## Содержание

- [Обзор проекта](#обзор-проекта)
- [Архитектура](#архитектура)
- [Жизненный цикл события задачи](#жизненный-цикл-события-задачи)
- [Надёжность и семантика доставки](#надёжность-и-семантика-доставки)
- [Кластер Kafka](#кластер-kafka)
- [REST API](#rest-api)
- [Технологический стек](#технологический-стек)
- [Наблюдаемость](#наблюдаемость)
- [Локальный запуск](#локальный-запуск)
- [Запуск через Docker Compose](#запуск-через-docker-compose)
- [Остановка и очистка](#остановка-и-очистка)
- [Конфигурация](#конфигурация)
- [Тестирование](#тестирование)
- [Полезные команды Make](#полезные-команды-make)
- [Структура репозитория](#структура-репозитория)
- [Текущий объём проекта и ограничения](#текущий-объём-проекта-и-ограничения)

## Обзор проекта

Проект состоит из трёх исполняемых приложений:

| Приложение | Ответственность |
|---|---|
| <code>cmd/todoapp</code> | HTTP API, бизнес-функциональность, доступ к PostgreSQL, кэш Redis и фоновый Outbox Publisher |
| <code>cmd/activity-worker</code> | Kafka Consumer, который проверяет события задач и сохраняет историю Activity |
| <code>cmd/activity-dlq-replay</code> | Утилита для повторной публикации отдельного сообщения из Dead Letter Queue |

API и Activity Worker являются отдельными процессами. Их можно независимо собирать, запускать, останавливать и масштабировать.

### Функциональность

- Управление пользователями: создание, получение, обновление и удаление.
- Управление задачами: создание, получение, обновление и удаление.
- Пагинация списков и истории Activity с помощью параметров <code>limit</code> и <code>offset</code>.
- Фильтрация списков задач и статистики по <code>user_id</code>.
- Фильтрация статистики по диапазону дат.
- История изменений задач, формируемая асинхронно на основе доменных событий.
- Кэш Redis для общей статистики задач без фильтров.
- Доставка событий Kafka с использованием Transactional Outbox в PostgreSQL.
- Повторная обработка ошибок и Dead Letter Queue для сообщений, которые невозможно обработать.
- Метрики, распределённые трассировки, структурированные логи и сценарии нагрузочного тестирования и проверки наблюдаемости.

## Архитектура

~~~mermaid
flowchart LR
    Client[HTTP client] --> API[cmd/todoapp]
    API --> Features[Users / Tasks / Statistics / Activity]
    Features --> DB[(PostgreSQL)]
    Features --> Cache[(Redis)]

    Features --> Outbox[(Transactional Outbox)]
    Outbox --> Publisher[Outbox Publisher]
    Publisher --> Events[(Kafka: task-events)]

    Events --> Worker[cmd/activity-worker]
    Worker --> Activity[(PostgreSQL: task_activity)]
    Worker --> DLQ[(Kafka: task-events.dlq)]

    Activity --> API

    API -. telemetry .-> OTel[OpenTelemetry Collector]
    Publisher -. telemetry .-> OTel
    Worker -. telemetry .-> OTel
    Events -. broker metrics .-> Exporters[Kafka exporters]

    OTel --> Prometheus[Prometheus]
    OTel --> Tempo[Tempo]
    Exporters --> Prometheus
    API -. log files .-> Alloy[Grafana Alloy]
    Worker -. log files .-> Alloy
    Alloy --> Loki[Loki]

    Prometheus --> Grafana[Grafana]
    Tempo --> Grafana
    Loki --> Grafana
~~~

### Основные компоненты

| Компонент | Назначение |
|---|---|
| HTTP API | Предоставляет функциональность приложения по базовому пути <code>/api/v1</code> |
| PostgreSQL | Основной источник данных для пользователей, задач, событий Outbox и истории Activity |
| Redis | Кэш общей статистики; PostgreSQL остаётся основным источником данных |
| Outbox Publisher | Получает неопубликованные записи Outbox и отправляет их в Kafka |
| Kafka | Передаёт доменные события задач между независимыми процессами |
| Activity Worker | Получает события и сохраняет историю изменений задач |
| DLQ | Хранит сообщения, которые невозможно обработать из-за постоянных ошибок |
| Prometheus | Хранит метрики в виде временных рядов |
| OpenTelemetry Collector | Получает телеметрию от Go-процессов и направляет её в Prometheus и Tempo |
| Tempo | Хранит распределённые трассировки |
| Grafana Alloy и Loki | Собирают и хранят логи приложения |
| Grafana | Предоставляет единый интерфейс для просмотра метрик, трассировок и логов |

## Жизненный цикл события задачи

При изменении задачи приложение не отправляет событие в Kafka непосредственно в рамках HTTP-запроса. Вместо этого используется паттерн Transactional Outbox.

1. Запрос поступает в HTTP Handler задачи, а затем в Task Service.
2. Tasks Repository начинает транзакцию PostgreSQL.
3. Репозиторий создаёт, обновляет или удаляет задачу.
4. Он формирует версионированный конверт события, например <code>task.created</code>, <code>task.updated</code> или <code>task.deleted</code>.
5. Событие записывается в <code>todoapp.outbox</code> в рамках той же транзакции.
6. PostgreSQL атомарно фиксирует изменение задачи и запись Outbox.
7. Outbox Publisher получает подходящие записи и публикует их в <code>task-events</code>.
8. После получения от Kafka подтверждения успешной доставки Publisher помечает запись Outbox как опубликованную.
9. Activity Worker из Consumer Group <code>activity-workers</code> получает событие, проверяет его и сохраняет в <code>todoapp.task_activity</code>.
10. Worker фиксирует Kafka offset только после успешной записи в базу данных.

Такой подход отделяет HTTP-запрос от обработки истории Activity. API не ожидает завершения работы Activity Worker.

### Конверт события

События используют версионированный конверт со следующими полями:

- <code>event_id</code> — уникальный идентификатор события.
- <code>event_type</code> — тип доменного события.
- <code>schema_version</code> — версия схемы события.
- <code>aggregate_type</code>, <code>aggregate_id</code>, <code>aggregate_version</code> — тип, идентификатор и версия агрегата.
- <code>occurred_at</code> — время создания события.
- <code>payload</code> — данные конкретного события.

Контракт события и его валидация реализованы в <code>internal/core/events</code>.

### Transactional Outbox

Таблица Outbox хранит данные события вместе с состоянием его публикации:

- <code>published_at</code> указывает, что событие было успешно опубликовано.
- <code>next_attempt_at</code> определяет время следующей попытки после ошибки.
- <code>lease_token</code> и <code>lease_until</code> защищают записи, полученные Publisher для обработки.
- <code>attempts</code> и <code>last_error</code> хранят информацию о попытках публикации и последней ошибке.
- <code>traceparent</code> и <code>tracestate</code> сохраняют контекст W3C Trace Context при переходе к асинхронной обработке.

Запрос получения записей использует <code>FOR UPDATE SKIP LOCKED</code>, поэтому несколько экземпляров Publisher не захватывают одну и ту же активную запись. Кроме того, следующая запись для того же агрегата не может быть получена, пока более раннее событие этого агрегата остаётся неопубликованным.

Publisher передаёт ID задачи в качестве ключа Kafka-сообщения. Благодаря этому Kafka направляет события одной задачи в одну партицию в соответствии с правилом партиционирования по ключу.

## Надёжность и семантика доставки

Используется семантика доставки **at-least-once (как минимум один раз)**, а не exactly-once (ровно один раз).

Одно и то же сообщение может быть опубликовано в Kafka несколько раз. Например, Kafka может подтвердить доставку сообщения, но последующее обновление статуса Outbox завершится ошибкой. После истечения lease событие может быть опубликовано повторно.

Обработка Activity рассчитана на такие повторные доставки:

- На <code>task_activity.event_id</code> установлено ограничение уникальности.
- При добавлении Activity используется <code>ON CONFLICT (event_id) DO NOTHING</code>.
- Репозиторий сообщает, была ли действительно добавлена новая запись.
- Повторная доставка считается успешно обработанной после подтверждения существующей записи и фиксации Kafka offset.

### Повторные попытки

Outbox Publisher планирует новую попытку, если сериализация события или его публикация в Kafka завершается ошибкой. Задержка между попытками экспоненциально увеличивается до заданного максимального значения.

Activity Worker повторяет операции обработки временных ошибок и фиксации offset, используя экспоненциальную задержку. На время повторной попытки он приостанавливает получение сообщений и возобновляет его после успешного завершения операции.

Сообщения с постоянными ошибками — например, некорректным JSON, невалидным конвертом, неподдерживаемым типом события или версией схемы — отправляются в <code>task-events.dlq</code>.

Исходный offset фиксируется только после того, как DLQ Producer подтвердит успешную публикацию сообщения в DLQ.

### Повторная публикация из DLQ

Отдельное сообщение из DLQ можно повторно опубликовать, указав его партицию и offset:

~~~bash
make activity-dlq-replay DLQ_PARTITION=1 DLQ_OFFSET=125
~~~

Утилита повторной публикации считывает выбранную запись DLQ и отправляет исходные payload и key в исходный топик и партицию. Содержимое события при этом не изменяется автоматически.

Повторная публикация может привести к повторной доставке события. Ограничение уникальности <code>event_id</code> в Activity Repository защищает сохранённую историю от появления дублирующих записей.

## Кластер Kafka

Локальная среда запускает три брокера Apache Kafka 4.4.0 в режиме KRaft. Каждый узел совмещает роли broker и controller; ZooKeeper не используется.

| Параметр | Значение |
|---|---:|
| Брокеры | 3 |
| Контроллеры | 3 (совмещены с брокерами) |
| Количество партиций в каждом прикладном топике | 3 |
| Фактор репликации | 3 |
| Минимальное количество синхронных реплик | 2 |
| Автоматическое создание топиков | Отключено |
| Подтверждения Producer | <code>acks=all</code> |
| Идемпотентность Producer | Включена |

### Топики

| Топик | Назначение | Время хранения |
|---|---|---:|
| <code>task-events</code> | Обычные доменные события задач | 7 дней |
| <code>task-events.dlq</code> | События, которые невозможно обработать | 30 дней |

Оба топика используют три партиции, фактор репликации 3 и <code>min.insync.replicas=2</code>. Нештатное избрание лидера (unclean leader election) отключено.

Топики создаются и настраиваются явно с помощью Makefile:

~~~bash
make kafka-up
make kafka-topics-setup
~~~

Kafka UI доступен по адресу [http://localhost:8080](http://localhost:8080).

### Масштабирование Consumer

Экземпляры Activity Worker входят в одну Consumer Group — <code>activity-workers</code>. Цель Make по умолчанию запускает три экземпляра Worker:

~~~bash
make activity-workers-up
~~~

Количество экземпляров можно изменить:

~~~bash
make activity-workers-up ACTIVITY_WORKER_REPLICAS=2
~~~

Kafka распределяет партиции между участниками Consumer Group. При наличии трёх партиций не более трёх участников могут одновременно владеть партициями этого топика.

## REST API

Базовый путь: <code>/api/v1</code>

Интерактивная документация API доступна по адресу:

[http://localhost:5050/swagger/index.html](http://localhost:5050/swagger/index.html)

### Пользователи

| Метод | Endpoint | Описание |
|---|---|---|
| <code>POST</code> | <code>/users</code> | Создать пользователя |
| <code>GET</code> | <code>/users</code> | Получить список пользователей; поддерживаются <code>limit</code> и <code>offset</code> |
| <code>GET</code> | <code>/users/{id}</code> | Получить пользователя |
| <code>PATCH</code> | <code>/users/{id}</code> | Обновить пользователя |
| <code>DELETE</code> | <code>/users/{id}</code> | Удалить пользователя |

### Задачи

| Метод | Endpoint | Описание |
|---|---|---|
| <code>POST</code> | <code>/tasks</code> | Создать задачу |
| <code>GET</code> | <code>/tasks</code> | Получить список задач; поддерживаются <code>user_id</code>, <code>limit</code> и <code>offset</code> |
| <code>GET</code> | <code>/tasks/{id}</code> | Получить задачу |
| <code>PATCH</code> | <code>/tasks/{id}</code> | Обновить задачу |
| <code>DELETE</code> | <code>/tasks/{id}</code> | Удалить задачу |

При обновлении пользователя или задачи используется поле <code>version</code> для оптимистичной блокировки. Попытка обновления на основе устаревшей версии приводит к конфликту, а не к незаметной перезаписи результата параллельного изменения.

### История Activity

| Метод | Endpoint | Описание |
|---|---|---|
| <code>GET</code> | <code>/tasks/{task_id}/activity</code> | Получить историю событий задачи; поддерживаются <code>limit</code> и <code>offset</code> |

История Activity читается из PostgreSQL. Результаты упорядочены по версии задачи по возрастанию.

Для событий <code>task.updated</code> и <code>task.completed</code> payload содержит снимки состояния <code>before</code> и <code>after</code>. Поле <code>author_user_id</code> обозначает владельца задачи и не обязательно указывает на пользователя, который выполнил изменение.

### Статистика

| Метод | Endpoint | Описание |
|---|---|---|
| <code>GET</code> | <code>/statistics</code> | Получить статистику задач |

Необязательные query-параметры:

- <code>user_id</code> — ограничить статистику задачами конкретного пользователя.
- <code>from</code> — начальная дата в формате <code>YYYY-MM-DD</code>.
- <code>to</code> — конечная дата в формате <code>YYYY-MM-DD</code>.

Ответ содержит количество созданных задач, количество завершённых задач, процент завершения и среднее время выполнения.

В Redis кэшируется только общий ответ со статистикой без фильтров. Время жизни записи в кэше — 60 секунд. Запросы с фильтрами обходят кэш. Сбои Redis не делают недоступной статистику, получаемую из PostgreSQL.

### Примеры запросов

Создание пользователя:

~~~bash
curl -X POST http://localhost:5050/api/v1/users \
  -H "Content-Type: application/json" \
  -d '{"full_name":"Ada Lovelace","phone_number":"+79998887766"}'
~~~

Создание задачи:

~~~bash
curl -X POST http://localhost:5050/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{"title":"Prepare project demo","description":"Review the architecture","author_user_id":1}'
~~~

Получение истории задачи:

~~~bash
curl "http://localhost:5050/api/v1/tasks/1/activity?limit=20&offset=0"
~~~

Получение статистики:

~~~bash
curl "http://localhost:5050/api/v1/statistics?user_id=1&from=2026-01-01&to=2026-12-31"
~~~

## Технологический стек

| Область | Технология |
|---|---|
| Язык | Go 1.26.0 |
| HTTP API | Go <code>net/http</code>, собственная маршрутизация и middleware |
| Документация API | Swagger / Swaggo |
| Основная база данных | PostgreSQL 18.1 |
| Драйвер и пул соединений PostgreSQL | pgx v5 |
| Кэш | Redis 8.2.11, <code>go-redis/v9</code> |
| Брокер сообщений | Apache Kafka 4.4.0 |
| Go-клиент Kafka | <code>confluent-kafka-go/v2</code> (librdkafka) |
| Логирование | Uber Zap |
| Метрики и трассировки | OpenTelemetry |
| Хранилище метрик | Prometheus |
| Хранилище распределённых трассировок | Grafana Tempo |
| Хранилище и сбор логов | Grafana Loki и Grafana Alloy |
| Визуализация | Grafana |
| Локальная инфраструктура | Docker Compose |
| Миграции базы данных | golang-migrate |

Go-клиент Confluent для Kafka использует CGO. Поэтому для локальной нативной сборки необходима рабочая C toolchain. Dockerfile приложения устанавливают необходимые зависимости для сборки и создают отдельные образы для API и Worker.

## Наблюдаемость

Конфигурация стека наблюдаемости находится в каталоге <code>monitoring/</code>.

### Телеметрия приложения

Оба приложения — <code>todoapp</code> и <code>activity-worker</code> — инициализируют провайдеры OpenTelemetry SDK. Go-процессы отправляют телеметрию по OTLP HTTP в OpenTelemetry Collector.

- Метрики доступны через Prometheus exporter Collector и собираются Prometheus.
- Collector передаёт трассировки в Tempo.
- Файлы логов приложения читает Grafana Alloy и отправляет их в Loki.

### Телеметрия Kafka

Метрики брокеров Kafka собираются с помощью JMX Prometheus Java Agent, подключённого к каждому брокеру. Отдельный Kafka Exporter предоставляет информацию о топиках, партициях, Consumer Groups, offset и lag.

Prometheus собирает метрики со следующих целей:

| Job | Target |
|---|---|
| <code>otel-collector</code> | <code>otel-collector:8889</code> |
| <code>kafka-brokers</code> | <code>kafka-1:9404</code>, <code>kafka-2:9404</code>, <code>kafka-3:9404</code> |
| <code>kafka-exporter</code> | <code>kafka-exporter:9308</code> |

### Метрики Kafka на уровне приложения

Основные метрики приложения:

| Метрика | Тип | Значение |
|---|---|---|
| <code>outbox_pending_events</code> | Gauge | Количество записей Outbox, не помеченных как опубликованные |
| <code>outbox_oldest_event_age_seconds</code> | Gauge | Возраст самой старой неопубликованной записи Outbox |
| <code>outbox_published_total</code> | Counter | Количество публикаций Outbox, подтверждённых Kafka |
| <code>outbox_publish_errors_total</code> | Counter | Количество ошибок сериализации или публикации событий |
| <code>activity_events_processed_total</code> | Counter | Количество успешно сохранённых или подтверждённых доставок Activity |
| <code>activity_events_duplicate_total</code> | Counter | Количество успешно подтверждённых повторных доставок |
| <code>activity_events_failed_total</code> | Counter | Количество окончательных ошибок обработки, после которых сообщение успешно отправлено в DLQ |
| <code>activity_dlq_messages_total</code> | Counter | Количество сообщений, успешно опубликованных в DLQ |
| <code>activity_processing_duration_seconds</code> | Histogram | Длительность обработки, включая повторные попытки и фиксацию offset |

Для счётчиков доступна детализация по атрибуту <code>event_type</code>. Гистограмма длительности обработки также содержит атрибут <code>outcome</code>.

Метрики Outbox собираются через callback, который выполняет запрос к Outbox Repository. Они отражают текущее состояние базы данных, а не накопительное количество событий.

### Доступ к инструментам мониторинга

| Инструмент | Локальный URL |
|---|---|
| Prometheus | [http://localhost:9090](http://localhost:9090) |
| Grafana | [http://localhost:3000](http://localhost:3000) |
| Tempo | [http://localhost:3200](http://localhost:3200) |
| Loki | [http://localhost:3100](http://localhost:3100) |
| Kafka UI | [http://localhost:8080](http://localhost:8080) |

Grafana автоматически настраивает источники данных Prometheus, Tempo и Loki из каталога <code>monitoring/grafana/provisioning/datasources</code>. Конфигурация мониторинга, дашборды и правила exporter хранятся в репозитории в каталоге <code>monitoring/</code>.

Следующая команда запускает сервисы наблюдаемости с учётом порядка зависимостей:

~~~bash
make observability-up
~~~

## Локальный запуск

### Предварительные требования

- Go 1.26.0.
- Docker Engine или Docker Desktop с плагином Docker Compose.
- GNU Make и Bash-совместимая оболочка (Git Bash в Windows).
- C-компилятор и toolchain для нативной сборки <code>confluent-kafka-go</code>.

### 1. Настройка окружения

Клонируйте репозиторий и создайте локальный файл окружения:

~~~bash
git clone https://github.com/Reigor1/20h-project.git
cd 20h-project
cp .env.example .env
~~~

Перед запуском стека отредактируйте <code>.env</code>. В частности, задайте непустые значения вместо шаблонных для учётных данных PostgreSQL, Redis и Grafana. Не добавляйте <code>.env</code> и реальные учётные данные в Git.

Makefile подключает <code>.env</code> и экспортирует его переменные. Создайте этот файл до запуска целей Make.

### 2. Запуск приложения и инфраструктуры

Обе цели запуска самостоятельно поднимают PostgreSQL, дожидаются состояния `healthy` по Docker healthcheck и применяют миграции перед запуском приложения. Поэтому вручную выполнять `make env-postgres-up` и `make migrate-up` перед первым запуском не требуется.

Чтобы запустить Go API локально, а инфраструктуру и Activity Worker — в Docker:

~~~bash
make todoapp-run-locally
~~~

Цель запускает Redis, PostgreSQL, миграции, проброс порта PostgreSQL, сервисы наблюдаемости, кластер Kafka и топики, а затем Activity Worker. После этого она запускает `cmd/todoapp` на хостовой машине.

Kafka UI в этом режиме можно запустить отдельно:

~~~bash
make kafka-ui-up
~~~

Миграции запускаются через Compose и ожидают готовности PostgreSQL благодаря условию `service_healthy`.
## Запуск через Docker Compose
После создания и настройки `.env` запустите весь стек:

~~~bash
make todoapp-run-in-docker
~~~

Цель самостоятельно запускает Redis и PostgreSQL, ожидает готовности PostgreSQL, применяет миграции, поднимает проброс порта базы данных, сервисы наблюдаемости и кластер Kafka, создаёт и настраивает топики, запускает Kafka UI и Activity Worker, а затем собирает и запускает HTTP API в Docker.

Отдельно запускать `env-postgres-up` и `migrate-up` перед этой командой не нужно.

API доступно на порту `5050`. Внутри сети Compose сервисы обращаются друг к другу по именам Docker-сервисов, например `todoapp-postgres`, `redis`, `kafka-1` и `otel-collector`.

### Полезные команды для управления сервисами

~~~bash
make kafka-status
make kafka-topics
make kafka-topics-describe
make activity-workers-up
make activity-workers-down
make observability-up
make observability-down
~~~

Три брокера Kafka публикуют на хосте порты <code>29092</code>, <code>39092</code> и <code>49092</code>. API и интерфейсы мониторинга предназначены для локальной разработки.


## Остановка и очистка

### Обычная остановка

~~~bash
make todoapp-down
~~~

Команда выполняет `docker compose down --remove-orphans`: останавливает и удаляет контейнеры Compose, удаляет сети проекта и orphan-контейнеры. Именованные Docker volumes и содержимое `out/` сохраняются.

Если API запущено локально через `make todoapp-run-locally`, сначала остановите процесс Go сочетанием `Ctrl+C`. Docker Compose не управляет этим процессом.

### Выборочная очистка данных

Очистка наблюдаемости:

~~~bash
make observability-down
make observability-cleanup
~~~

Сначала остановите сервисы наблюдаемости, затем выполните очистку. Цель `observability-cleanup` удаляет локальные данные Prometheus, Grafana, Tempo, Loki и Alloy из каталогов `out/`. История метрик, трассировок, логов и локальное состояние Grafana будут потеряны. Конфигурация в `monitoring/` не удаляется.

Удаление результатов нагрузочного тестирования:

~~~bash
make load-tests-cleanup
~~~

Цель удаляет только каталог `out/load-tests`.

Удаление файлов логов:

~~~bash
make logs-cleanup
~~~

Цель удаляет только каталог `out/logs`. Перед очисткой остановите процессы, которые могут продолжать записывать логи.

Выборочные цели очистки запрашивают подтверждение перед удалением данных.

### Полный сброс окружения

~~~bash
make todoapp-cleanup
~~~

Цель требует ввести `yes` для подтверждения. После подтверждения она:

1. Останавливает Compose-проект и удаляет его контейнеры, сети, orphan-контейнеры и управляемые Compose volumes.
2. Удаляет весь каталог `out/`, включая данные PostgreSQL, Kafka, наблюдаемости, логи и результаты нагрузочного тестирования.

Полный сброс необратимо удаляет локальные данные приложения. Исходный код, конфигурация в репозитории, файл `.env`, Docker images и build cache сохраняются. External Docker volumes, не управляемые Compose, этой командой не удаляются.

Перед запуском полного сброса остановите локально запущенный Go-процесс через `Ctrl+C`.

## Конфигурация

Полный перечень переменных окружения и их значений по умолчанию приведён в <code>.env.example</code>. Основные группы:

| Префикс / переменная | Назначение |
|---|---|
| <code>HTTP_*</code>, <code>TIME_ZONE</code> | HTTP-сервер и часовой пояс приложения |
| <code>POSTGRES_*</code> | Подключение к PostgreSQL и таймауты операций |
| <code>REDIS_*</code> | Подключение к Redis |
| <code>KAFKA_*</code> | Адреса Kafka bootstrap servers и настройки Producer |
| <code>OUTBOX_PUBLISHER_*</code> | Топик Outbox, polling, размер пакета, lease, повторные попытки и настройки завершения работы |
| <code>KAFKA_CONSUMER_*</code> | Consumer Group, исходный топик и DLQ, повторные попытки, очередь ожидающих сообщений и настройки завершения работы |
| <code>LOGGER_*</code> | Настройки логирования приложения |
| <code>OTEL_EXPORTER_OTLP_ENDPOINT</code> | Endpoint OpenTelemetry Collector |
| <code>SERVICE_VERSION</code>, <code>DEPLOYMENT_ENVIRONMENT</code> | Атрибуты ресурсов телеметрии |
| <code>GRAFANA_*</code> | Учётные данные Grafana и необязательная конфигурация SMTP |

Для API, запущенного на хостовой машине, используется <code>POSTGRES_HOST=localhost</code>, а Kafka bootstrap servers указывают на порты хоста. Docker Compose переопределяет адреса сервисов на внутренние адреса Docker-сети.

Настройки тестовой базы данных хранятся отдельно в <code>.env.test.example</code>.

## Тестирование

В репозитории есть unit-тесты, интеграционные тесты PostgreSQL, E2E-тесты API, а также отдельные сценарии наблюдаемости и нагрузочного тестирования.

### Unit-тесты

~~~bash
go test ./internal/...
~~~

### Интеграционные тесты

Создайте файл тестового окружения:

~~~bash
cp .env.test.example .env.test
~~~

Запустите интеграционные наборы тестов:

~~~bash
make tests-integration
~~~

Цели интеграционного тестирования запускают отдельный тестовый контейнер PostgreSQL, выполняют выбранный набор тестов, а затем удаляют тестовый контейнер.

### End-to-end тесты API

~~~bash
make tests-e2e
~~~

Цель E2E-тестирования API запускает изолированную тестовую базу данных, применяет миграции, выполняет набор тестов API и удаляет тестовый контейнер.

### Сценарий проверки наблюдаемости

Генератор сценария наблюдаемости требует запущенного API. Он создаёт тестовые данные, выполняет запросы к маршрутам и сценарии нагрузки, генерирует HTTP-ответы 5xx, а затем удаляет созданные им данные.

~~~bash
make test-observability
~~~

### Нагрузочное тестирование

Для нагрузочных тестов также требуется уже запущенное API:

~~~bash
make test-load PROFILE=standard
make test-load PROFILE=ramp-up
make test-load PROFILE=steady-state
make test-load PROFILE=stress
make test-load PROFILE=all
~~~

Доступные параметры включают <code>STAGE_DURATION</code>, <code>STEADY_WORKERS</code>, <code>STEADY_DURATION</code>, <code>STRESS_WORKERS</code>, <code>USERS</code> и <code>TASKS_PER_USER</code>.

По умолчанию JSON-отчёты сохраняются в <code>out/load-tests</code>.

### Статические проверки и сборка

~~~bash
go vet ./...
go build ./cmd/todoapp
go build ./cmd/activity-worker
go build ./cmd/activity-dlq-replay
~~~

## Полезные команды Make

| Цель | Назначение |
|---|---|
| <code>env-postgres-up</code> / <code>env-postgres-down</code> | Запустить или остановить PostgreSQL |
| <code>env-redis-up</code> / <code>env-redis-down</code> | Запустить или остановить Redis |
| <code>migrate-up</code> / <code>migrate-down</code> | Применить или откатить миграции базы данных |
| <code>kafka-up</code> / <code>kafka-down</code> | Запустить или остановить кластер Kafka |
| <code>kafka-status</code> | Показать состояние брокеров и кворума KRaft |
| <code>kafka-topics-setup</code> | Создать топики и применить их конфигурацию |
| <code>kafka-topics-describe</code> | Показать информацию о партициях, репликах, лидерах и ISR |
| <code>kafka-ui-up</code> / <code>kafka-ui-down</code> | Запустить или остановить Kafka UI |
| <code>activity-workers-up</code> / <code>activity-workers-down</code> | Запустить или остановить реплики Activity Worker |
| <code>todoapp-run-locally</code> | Запустить зависимости и API на хостовой машине |
| <code>todoapp-run-in-docker</code> | Запустить стек приложения в Docker |
| <code>todoapp-down</code> | Остановить Compose-проект без удаления volumes и локальных данных |
| <code>todoapp-cleanup</code> | Полностью сбросить Compose-окружение и удалить локальные runtime-данные |
| <code>observability-cleanup</code> | Удалить локальные данные Prometheus, Grafana, Tempo, Loki и Alloy |
| <code>load-tests-cleanup</code> | Удалить результаты нагрузочного тестирования из <code>out/load-tests</code> |
| <code>logs-cleanup</code> | Удалить локальные файлы логов из <code>out/logs</code> |
| <code>observability-up</code> / <code>observability-down</code> | Запустить или остановить стек мониторинга |
| <code>tests-integration</code> | Запустить интеграционные тесты PostgreSQL и инфраструктуры |
| <code>tests-e2e</code> | Запустить end-to-end тесты API |
| <code>test-observability</code> | Запустить сценарий проверки наблюдаемости |
| <code>test-load</code> | Запустить выбранный профиль нагрузочного тестирования |
| <code>activity-dlq-replay</code> | Повторно опубликовать выбранное сообщение из DLQ |

## Структура репозитория

~~~text
.
├── cmd/
│   ├── todoapp/                 # Точка входа HTTP API и Outbox Publisher
│   ├── activity-worker/         # Точка входа Kafka Activity Worker
│   └── activity-dlq-replay/     # Утилита повторной публикации из DLQ
├── docs/                        # Сгенерированная спецификация Swagger
├── internal/
│   ├── core/
│   │   ├── domain/              # Доменные модели и инварианты
│   │   ├── events/              # Конверт события и конструкторы событий задач
│   │   ├── kafka/               # Адаптеры Kafka Producer и Consumer
│   │   ├── observability/       # Настройка OpenTelemetry и метрики
│   │   ├── repository/          # Инфраструктура PostgreSQL и Redis
│   │   └── transport/http/     # HTTP-сервер, middleware и обработка запросов/ответов
│   └── features/
│       ├── users/               # Функциональность пользователей
│       ├── tasks/               # Функциональность задач и создание событий Outbox
│       ├── outbox/              # Outbox Repository и Publisher
│       ├── activity/            # Activity Worker, DLQ, Repository, Service и HTTP
│       └── statistics/          # Статистика, PostgreSQL Repository и кэш Redis
├── migrations/                  # Последовательные SQL-миграции up/down
├── monitoring/
│   ├── alloy/                   # Сбор логов
│   ├── grafana/                 # Настройка источников данных и дашборды
│   ├── kafka/                   # Образ JMX Exporter и правила сбора метрик
│   ├── loki/
│   ├── otel-collector/
│   ├── prometheus/
│   └── tempo/
├── tests/
│   ├── integration/             # Интеграционные тесты PostgreSQL и инфраструктуры
│   └── e2e/                     # E2E-тесты API и сценарии наблюдаемости/нагрузки
├── docker-compose.yaml          # Локальный стек приложения и инфраструктуры
├── docker-compose.test.yaml     # Изолированный тестовый сервис PostgreSQL
├── Makefile                     # Команды разработки, инфраструктуры и тестирования
└── go.mod
~~~

## Текущий объём проекта и ограничения

- Локальные Kafka listeners используют PLAINTEXT, а конфигурация Compose предназначена для разработки. Аутентификация, TLS, управление секретами и политики production-развёртывания не входят в текущий объём проекта.
- Семантика доставки — at-least-once. Архитектура опирается на идемпотентное сохранение Activity и не заявляет сквозную обработку exactly-once.
- Activity Worker повторяет обработку временных ошибок и ошибки фиксации offset с ограниченной экспоненциальной задержкой. Конверты событий с постоянными ошибками направляются в DLQ.
- Redis является необязательным кэшем, а не источником истины. При недоступности кэша статистика продолжает поступать из PostgreSQL.
- В API пока есть CRUD-операции без слоя аутентификации и авторизации. Перед предоставлением публичного доступа необходимо добавить соответствующие механизмы безопасности.
- Дашборды Grafana и правила оповещений могут развиваться независимо от инструментирования приложения. Конвейер телеметрии и источники данных конфигурируются в репозитории; анализ на уровне конкретных дашбордов является отдельным слоем.

## Лицензия

В репозитории пока нет файла лицензии. Перед повторным использованием проекта или его кода свяжитесь с владельцем репозитория.
