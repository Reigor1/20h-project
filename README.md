# 20h-project

REST API на Go для управления пользователями и задачами, а также получения статистики по задачам. Приложение использует PostgreSQL и предоставляет Swagger UI с описанием API.

## Возможности

- создание, просмотр, изменение и удаление пользователей;
- создание, просмотр, изменение и удаление задач;
- постраничный просмотр пользователей и задач с параметрами `limit` и `offset`;
- фильтрация задач и статистики по `user_id`;
- получение статистики по задачам с необязательными параметрами периода `from` и `to`;
- просмотр спецификации API в Swagger UI.

Основной API имеет базовый путь `/api/v1`. Полный перечень параметров и схем запросов доступен в Swagger UI.

## Технологии и требования

- Go 1.26.0 (версия указана в `go.mod` и Dockerfile);
- PostgreSQL 18.1 (образ из `docker-compose.yaml`);
- Docker с командой `docker compose` для запуска базы и миграций через Makefile;
- GNU Make для команд из `Makefile`.

Для локального запуска приложения без Docker также потребуется доступный PostgreSQL. Отдельная инструкция по ручной установке PostgreSQL в репозитории не приведена.

## Настройка

1. Создайте локальный файл конфигурации из примера:

   ```sh
   cp .env.example .env
   ```

2. В `.env` задайте непустые `POSTGRES_USER`, `POSTGRES_PASSWORD` и `POSTGRES_DB`. Остальные значения можно оставить из примера, если они подходят вашему окружению.

Makefile подключает `.env` и экспортирует его значения для команд Make. Docker Compose также использует этот файл.

### Переменные окружения

| Переменная | Назначение | Значение по умолчанию / требование |
| --- | --- | --- |
| `HTTP_ADDR` | Адрес HTTP-сервера | обязательна; в `.env.example` — `:5050` |
| `HTTP_SHUTDOWN_TIMEOUT` | Таймаут завершения HTTP-сервера | `30s` в конфигурации |
| `POSTGRES_HOST` | Хост PostgreSQL | обязательна; локальный запуск через Makefile задаёт `localhost`, Docker Compose — `todoapp-postgres` |
| `POSTGRES_PORT` | Порт PostgreSQL | `5432` |
| `POSTGRES_USER` | Пользователь PostgreSQL | обязательна |
| `POSTGRES_PASSWORD` | Пароль PostgreSQL | обязательна |
| `POSTGRES_DB` | Имя базы PostgreSQL | обязательна |
| `POSTGRES_TIMEOUT` | Таймаут подключения к PostgreSQL | обязательна; в `.env.example` — `10s` |
| `LOGGER_LEVEL` | Уровень логирования | `debug` в конфигурации; в `.env.example` — `DEBUG` |
| `LOGGER_FOLDER` | Каталог файлов логов | обязательна; Makefile задаёт `out/logs` для локального запуска, Compose — `/app/out/logs` в контейнере |
| `TIME_ZONE` | Часовой пояс приложения | `UTC` |

## Запуск

### Через Docker Compose и Makefile

После настройки `.env` запустите PostgreSQL:

```sh
make env-up
```

Примените миграции, затем соберите и запустите приложение в контейнере:

```sh
make migrate-up
make todoapp-deploy
```

Сервер опубликован на `http://localhost:5050`. Swagger UI доступен по адресу `http://localhost:5050/swagger/`.

Чтобы остановить приложение и базу данных, используйте:

```sh
make todoapp-undeploy
make env-down
```

### Локальный запуск Go-приложения

Убедитесь, что PostgreSQL запущен и доступен, затем примените миграции и выполните:

```sh
make env-up
make migrate-up
make todoapp-run
```

Цель `todoapp-run` задаёт `LOGGER_FOLDER=out/logs` и `POSTGRES_HOST=localhost`, выполняет `go mod tidy`, а затем запускает `cmd/todoapp/main.go`.

## База данных и миграции

Миграции находятся в `migrations/` и запускаются контейнером `migrate/migrate:v4.19.1` через Makefile. Начальная миграция создаёт схему `todoapp` и таблицы `users` и `tasks`.

```sh
make env-up
make migrate-up
```

Команда `migrate-down` без параметров передаёт `down` мигратору без ограничения числа шагов. Чтобы откатить один шаг, задайте `version=1`:

```sh
make migrate-action action=down version=1
```

`make migrate-create seq=имя` создаёт последовательную пару SQL-файлов миграции. В Makefile также предусмотрены `make migrate-action action=up` и передача версии через `version=...`.

## API и документация

Базовый путь API: `/api/v1`.

| Метод | Путь | Назначение |
| --- | --- | --- |
| `POST`, `GET` | `/users` | Создание пользователя и список пользователей |
| `GET`, `PATCH`, `DELETE` | `/users/{id}` | Получение, изменение и удаление пользователя |
| `POST`, `GET` | `/tasks` | Создание задачи и список задач |
| `GET`, `PATCH`, `DELETE` | `/tasks/{id}` | Получение, изменение и удаление задачи |
| `GET` | `/statistics` | Статистика задач |

У списков пользователей поддерживаются query-параметры `limit` и `offset`; у списка задач — `user_id`, `limit` и `offset`. Статистика принимает `user_id`, `from` и `to`; формат дат — `YYYY-MM-DD`.

При запущенном приложении Swagger UI доступен по адресу `/swagger/`, спецификация — в `docs/swagger.yaml` и `docs/swagger.json`.

## Тестирование

Файлы Go-тестов (`*_test.go`) в репозитории не обнаружены. Тестирование проекта находится в разработке.

## Структура проекта

```text
cmd/todoapp/                 точка входа приложения и Dockerfile
docs/                        Swagger-спецификация и сгенерированная документация
internal/core/                конфигурация, доменные типы, логирование, HTTP и PostgreSQL-инфраструктура
internal/features/users/      API, сервисы и PostgreSQL-репозиторий пользователей
internal/features/tasks/      API, сервисы и PostgreSQL-репозиторий задач
internal/features/statistics/ API, сервисы и PostgreSQL-репозиторий статистики
migrations/                   SQL-миграции базы данных
out/                          локальные каталоги данных PostgreSQL и логов, используемые конфигурацией Compose/Makefile
```

Ключевые команды Makefile: `env-up`, `env-down`, `migrate-up`, `migrate-down`, `todoapp-run`, `todoapp-deploy`, `todoapp-undeploy`, `swagger-gen` и `ps`.
