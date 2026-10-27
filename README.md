# java-crud-api (Go)

A REST API for creating, reading, updating and deleting user records in MySQL. This is the Go port of the Spring Boot project `abdullaharshadd/java-crud-api`. All 8 modules were migrated.

Each user has these fields:

- name
- email (unique)
- password
- role
- about (up to 500 characters)

## Verification status

These checks were run against the migrated code:

| Check | Result |
|---|---|
| Build (`go build ./cmd/server`) | passed |
| Unit tests (`go test ./...`) | passed |
| Behaviour compared with the original Spring app | passed |

The migration tool also reported a self-assessed confidence of 95%. That figure is the model's own estimate, not a measured result. Some modules are still flagged for review (see [Manual review required](#manual-review-required)).

## Tech stack

- **Go**, with the module `migrated-app`. `go.mod` declares `go 1.23`. Verification ran on the Go toolchain from `golang:1.27-alpine`.
- **HTTP router:** [chi](https://github.com/go-chi/chi) v5.1.0, with the `Logger` and `Recoverer` middleware.
- **Database driver:** [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) v1.8.1, used through the standard library's `database/sql`.
- **Configuration:** [viper](https://github.com/spf13/viper) v1.19.0, reading environment variables.
- **Logging:** [zerolog](https://github.com/rs/zerolog) v1.33.0.
- **Database:** MySQL. The original Spring app also used MySQL. `docker-compose.yml` uses the `mysql:8` image.

## Prerequisites

- **Go toolchain.** Verification used the toolchain shipped in `golang:1.27-alpine`. `go.mod` requires at least Go 1.23.
- **git and a C build toolchain.** On Alpine, install them with:
  ```sh
  apk add --no-cache git build-base
  ```
- **A reachable MySQL 8 server** with a database and a user that can create tables in it.
- **Optional:** Docker with the Compose plugin, if you want to use the Docker Compose path.

## Getting started

Run these steps from the root of a fresh clone.

### 1. Resolve dependencies

Module checksums are not committed, so you must resolve modules first:

```sh
go mod tidy
```

### 2. Build

```sh
mkdir -p bin
go build -o bin/server ./cmd/server
```

### 3. Prepare the database

You do not need to run a separate schema step. At startup the server runs idempotent `CREATE TABLE IF NOT EXISTS` statements (see `internal/repository/schema.go`) that create:

- the `user` table, with a unique key on `user_email`
- the `hibernate_sequence` table, used for ID generation, seeded with `next_val = 1` when empty

You only need to provide:

- an existing MySQL database
- a user with `CREATE`, `INSERT`, `SELECT`, `UPDATE` and `DELETE` privileges on that database

The startup check is retried up to 30 times, 2 seconds apart, so the server can wait for a database that is still starting. If every attempt fails, the process exits.

### 4. Configure the environment

The connection string is passed directly to `go-sql-driver/mysql`. Use that driver's DSN format, `user:password@tcp(host:port)/dbname?parseTime=true`. Adjust the values below to match your MySQL instance:

```sh
export DATABASE_URL='app:app@tcp(localhost:3306)/app?parseTime=true'
export PORT=8080
export JWT_SECRET=change-me
```

If you set nothing, the server listens on port **8082** and connects with `root:root@tcp(localhost:3306)/barcode?parseTime=true`. These defaults match the original Spring configuration.

### 5. Run

```sh
./bin/server
```

The server logs `server started on :8080` and listens on `http://localhost:8080`. It shuts down gracefully on `SIGINT` or `SIGTERM`, with a 10 second timeout.

### A first request to try

These requests were verified against the migrated server:

```sh
curl -i http://localhost:8080/get_user_data
```

Expected response: `200`.

Other verified requests:

| Request | Status |
|---|---|
| `GET /get_user_data` | 200 |
| `GET /get_user_data/1` | 200 |
| `GET /get_user_name/name/Priya%20Sharma` | 200 |
| `GET /delete_user_data/1` | 405 (wrong method) |
| `POST /save_user_data` with a JSON body containing `name`, `email`, `password`, `role` and `about` | 200 |
| `PUT /update_user_data/1` with the body below | 200 |
| `DELETE /delete_user_data/1` | 200 |

```sh
curl -i -X POST http://localhost:8080/save_user_data \
  -H 'Content-Type: application/json' \
  -d '{"name": "Priya Sharma", "email": "priya.sharma@example.com", "password": "Secure@Pass2024", "role": "ROLE_USER", "about": "Software engineer based in Bangalore."}'

curl -i -X PUT http://localhost:8080/update_user_data/1 \
  -H 'Content-Type: application/json' \
  -d '{"name": "Jane Smith", "email": "jane.smith@example.com", "password": "SecurePass123", "role": "ROLE_USER", "about": "Software engineer who enjoys hiking and photography."}'

curl -i -X DELETE http://localhost:8080/delete_user_data/1
```

The ID-based requests assume that a user with ID `1` already exists. Create one first with `POST /save_user_data`, as shown above.

The server also exposes a liveness endpoint, `GET /healthz`, which returns `200` with the body `ok`.

### Alternative: Docker Compose

The `DATABASE_URL` committed in `docker-compose.yml` (`mysql://app:app@mysql:3306/app`) is in URL form, which `go-sql-driver/mysql` cannot parse as a DSN, so the app cannot connect and exits after its 30 schema retries. Before starting the stack, change that line in `docker-compose.yml` to the driver's DSN form:

```yaml
      DATABASE_URL: "app:app@tcp(mysql:3306)/app?parseTime=true"
```

Then run:

```sh
docker compose up --build
```

This starts two containers:

- `mysql`: MySQL 8 with database `app`, user `app` and password `app`. A named volume, `mysql-data`, persists the data. Port 3306 is exposed only on the Compose network, not on the host.
- `app`: the API. It waits until the MySQL healthcheck passes, then serves on `http://localhost:8080`.

## Running tests

```sh
go test ./...
```

## Environment variables

| Variable | Used | Default | Description |
|---|---|---|---|
| `PORT` | yes | `8082` | HTTP listen port. |
| `SERVER_PORT` | yes | — | Overrides `PORT` when set. This is the original Spring property name. |
| `DATABASE_URL` | yes | `root:root@tcp(localhost:3306)/barcode?parseTime=true` | MySQL DSN in `go-sql-driver/mysql` format. |
| `DB_DSN` | yes | — | Overrides `DATABASE_URL` when set. |
| `JWT_SECRET` | read, **not used** | — | Loaded into the config, but no code uses it. |

`docker-compose.yml` also sets `DB_HOST`, `DB_NAME`, `DB_PASSWORD`, `DB_PORT`, `DB_USER`, `DB_USERNAME` and `MYSQL_URL`. The application does not read any of these.

## Architecture overview

```
cmd/server/main.go                  Entry point: loads config, starts http.Server, graceful shutdown
cmd/server/router.go                chi router, middleware, /healthz, DB connection,
                                    schema bootstrap with retries, dependency wiring
internal/config/config.go           Env-based configuration (viper) and defaults
internal/httpapi/handler.go         HTTP handlers and User routes
internal/httpapi/errors.go          Maps errors to HTTP error responses
internal/model/user.go              User struct
internal/model/error_message.go     Error response body
internal/repository/schema.go       Idempotent CREATE TABLE statements
internal/repository/user_repository.go   MySQL repository with explicit SQL
internal/service/user_service.go    Business logic between handlers and repository
internal/service/errors.go          Service-level errors (e.g. user not found)
Dockerfile, docker-compose.yml      Container build and local stack
```

A request passes through these layers in order:

1. chi middleware (Logger, Recoverer)
2. `httpapi.Handler`
3. `service.UserService`
4. `repository.NewMySQLUserRepository`
5. MySQL, through `database/sql`

The router mounts these routes:

| Method | Path |
|---|---|
| `POST` | `/save_user_data` |
| `GET` | `/get_user_data` |
| `GET` | `/get_user_data/{id}` |
| `PUT` | `/update_user_data/{id}` |
| `DELETE` | `/delete_user_data/{id}` |
| `GET` | `/get_user_name/name/{name}` |
| `GET` | `/healthz` |

## Migration notes

Changes from the original Spring codebase:

- **JSON decoding differences.**
  - Changes:
    - Request keys now match case-insensitively. Jackson is case-sensitive.
    - A quoted number such as `"3"` is no longer coerced into an int field. It is rejected with a `400`.
  - Reason: the migration plan accepts these as known differences that do not affect well-formed clients.
- **405 response body.**
  - Change: a wrong HTTP method still returns `405` with an `Allow` header. The body is now a short text line from Go's ServeMux. Spring sent an empty body.
  - Reason: the plan considers this small body difference acceptable.
- **Hibernate dialect setting removed.**
  - Reason: it has no equivalent in Go's `database/sql`.
- **`ddl-auto=update` replaced by `CREATE TABLE IF NOT EXISTS` at startup.**
  - Change: columns are no longer added automatically to an existing table.
  - Reason: the model is fixed, so adding columns is not needed. `IF NOT EXISTS` also keeps an existing Hibernate-created database safe.
- **Thymeleaf and devtools removed.**
  - Reason: Thymeleaf was unused (there are no templates), and devtools is only for development.
- **New `GET /healthz` endpoint.**
  - Change: it returns `200` with the body `ok`.
  - Reason (inferred): lets container orchestration check that the process is alive.
- **Additional configuration variables.**
  - Change: `PORT` and `DATABASE_URL` are now read in addition to `SERVER_PORT` and `DB_DSN`. `JWT_SECRET` is read but not used.
  - Reason (inferred): matches the generic container environment set in `docker-compose.yml`.
- **Dockerfile and docker-compose.yml added.**
  - Change: these run the service on port 8080 against a bundled MySQL 8 container with database `app` and credentials `app`/`app`. The original ran on port 8082 against database `barcode` with `root`/`root`.
  - Reason (inferred): containerised local deployment.
- **Schema creation retried at startup.**
  - Change: up to 30 attempts, 2 seconds apart, before the process exits. The plan had it exit on the first failure.
  - Reason (inferred): tolerates a database container that is still starting.

## Known limitations

These parts of the original could not be translated one-to-one:

- **`UserNotFoundException(String, Throwable, boolean, boolean)` protected constructor.**
  - Why: it controls suppressed-exception tracking and stack-trace writability. These are JVM `Throwable` features with no Go equivalent.
  - Current state: the constructor is dropped. Errors are plain Go error values (`internal/service/errors.go`).
- **Hibernate schema auto-generation for the `User` entity.**
  - Why: implicit DDL generated from annotations has no direct equivalent in Go.
  - Current state: the schema is hand-written in `internal/repository/schema.go`. It keeps the table and column names, the unique email constraint, the 500-character length for `about`, and the `hibernate_sequence` ID table.
  - Consequence: future model changes need manual DDL.
- **Lombok annotations on `User`.**
  - Why: Lombok is Java-only compile-time code generation.
  - Current state: replaced by a plain Go struct in `internal/model/user.go`.
- **`UserDao` (Spring Data `JpaRepository` proxy).**
  - Why: Spring generates the implementation at runtime. Go has no equivalent.
  - Current state: replaced by explicit SQL in `internal/repository/user_repository.go`. Only the operations the service uses are implemented, plus find-by-name.
- **JPA persistence context and dirty checking.**
  - Why: Hibernate automatically flushed changes to managed entities. Go has no implicit flush.
  - Consequence: any update must be an explicit repository call.
- **`JWT_SECRET` is configuration only.**
  - Current state: the value is loaded, but no authentication is implemented.

## Manual review required

- **Code ported from the original `User` entity (low confidence)**, in `internal/model/user.go` and `internal/repository/schema.go`. Verify:
  - column names and types
  - the unique email constraint
  - the `about` length
  - that ID generation through `hibernate_sequence` matches the original
  - that the password is not exposed unintentionally in JSON responses or logs
- **Code ported from `RestResponseEntityExceptionHandling` (low confidence)**, in `internal/httpapi/errors.go` and `internal/model/error_message.go`. Verify that the status codes and error body shape match the original for not-found and validation errors.
- **Code ported from `UserServiceImp` (low confidence)**, in `internal/service/user_service.go`. Verify:
  - that the update path writes every field explicitly
  - that the original did not rely on JPA dirty checking anywhere a Go equivalent might be missing
- **The `DATABASE_URL` format in `docker-compose.yml`.**
  - Problem: it uses `mysql://app:app@mysql:3306/app`. The app passes this value unchanged to `go-sql-driver/mysql`, which expects `user:pass@tcp(host:port)/db` and does not convert URL-style strings, so the Compose stack as committed cannot connect.
  - What to do: change it in `docker-compose.yml` to `app:app@tcp(mysql:3306)/app?parseTime=true` (as described in [Alternative: Docker Compose](#alternative-docker-compose)), or set `DB_DSN` to that value.
- **Unused variables in `docker-compose.yml`.** `DB_HOST`, `DB_NAME`, `DB_PASSWORD`, `DB_PORT`, `DB_USER`, `DB_USERNAME` and `MYSQL_URL` are never read. Remove them, or wire them into the configuration on purpose.
