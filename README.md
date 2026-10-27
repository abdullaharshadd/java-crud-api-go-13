# java-crud-api (Go)

A REST CRUD API for managing users, backed by MySQL. This is a Go port of the Spring Boot project `abdullaharshadd/java-crud-api`. All 8 modules of the original were migrated to the Go standard library stack.

**Verification status for this migration:**

| Check | Result |
|---|---|
| Build | passed |
| Target unit tests | passed |
| Behavior compared with the original | passed |

The migration tool rated its own confidence at 95%. That figure is a self-assessment, not a measured result. Several components still need manual review. See [Manual review required](#manual-review-required).

## Tech stack

- **Language:** Go (standard library HTTP server)
- **Database:** MySQL
- **Packaging:** Go modules (`go.mod`), Docker (`Dockerfile`, `docker-compose.yml`)

## Prerequisites

- **Go 1.27.** The migration was built and tested with the `golang:1.27-alpine` image.
- **git.** `go mod tidy` uses it to fetch modules.
- **A C build toolchain** (gcc, make, libc headers). The build environment installed Alpine's `build-base` package. On other systems, install the equivalent, such as `build-essential` on Debian/Ubuntu or Xcode Command Line Tools on macOS.
- **A reachable MySQL server**, with a database the app can connect to.
- **Docker with Docker Compose** (optional). Needed only for the compose-based setup.

## Getting started

Run all commands from the root of a fresh clone.

### 1. Install dependencies and build

```sh
mkdir -p bin
go mod tidy
go build -o bin/server ./cmd/server
```

This produces the server binary at `bin/server`.

### 2. Set environment variables

```sh
export DATABASE_URL='<your MySQL connection string>'
export PORT=8080
export JWT_SECRET='<a long random secret>'
```

See [Environment variables](#environment-variables) for details.

### 3. Database setup

This run recorded no explicit database setup command. Before starting the server, make sure the MySQL database referenced by `DATABASE_URL` exists and the configured user can access it.

The original Spring app relied on Hibernate `ddl-auto` to create the `User` table. The Go port includes `internal/repository/schema.go` for schema definition. Confirm that the schema it defines is created at startup before relying on it in a real environment (see [Known limitations](#known-limitations)).

### 4. Run the server

```sh
./bin/server
```

The server listens on port **8080**: `http://localhost:8080`.

**A first request to try:** this migration run recorded no individually verified example requests. To find the available endpoints, read the route registrations in `cmd/server/router.go`.

### Alternative: Docker Compose

The repository includes a `Dockerfile` and a `docker-compose.yml`:

```sh
docker compose up --build
```

Before relying on this setup, review `docker-compose.yml` for the services, ports and environment values it defines. This migration run did not exercise the compose setup.

## Running tests

```sh
go test ./...
```

The target unit tests passed during migration.

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | yes | Connection string for the MySQL database. The exact format the app expects is parsed in `internal/config/config.go` and was not recorded in this run. |
| `PORT` | yes | Port for the HTTP server to listen on. The migration ran with `8080`. |
| `JWT_SECRET` | yes | Secret used for signing and verifying JWTs. Use a long random value and never commit it. |

## Architecture overview

```
cmd/server/
  main.go              Entry point: loads config, wires dependencies, starts the HTTP server
  router.go            HTTP route registration
internal/config/
  config.go            Reads configuration from environment variables
internal/httpapi/
  handler.go           HTTP handlers for the user CRUD endpoints (replaces the Spring controller)
  errors.go            Maps service errors to HTTP error responses (replaces RestResponseEntityExceptionHandling)
internal/model/
  user.go              User struct (replaces the JPA/Lombok User entity)
  error_message.go     JSON error response body
internal/repository/
  schema.go            Database schema for the user table (replaces Hibernate ddl-auto)
  user_repository.go   Explicit SQL data access for users (replaces the Spring Data UserDao)
internal/service/
  user_service.go      Business logic for users (replaces UserServiceImp)
  errors.go            Service-level error values, e.g. user not found (replaces UserNotFoundException)
```

A request flows through these layers: `router.go` → `httpapi` handler → `service` → `repository` → MySQL. Errors returned from the service layer are translated into HTTP responses in `internal/httpapi/errors.go`.

## Migration notes

No individual behavioral changes from the original were recorded for this migration. The structural changes below follow from moving off the Spring/JPA runtime:

- **Spring dependency injection → explicit wiring.** Components are constructed and connected by hand in `cmd/server/main.go`, because Go has no DI container.
- **Spring Data `UserDao` → hand-written repository.** Spring Data generates repository implementations at runtime from interface method names, and Go has no equivalent. `internal/repository/user_repository.go` therefore uses explicit SQL.
- **Hibernate schema auto-generation → explicit schema.** Hibernate's annotation-driven DDL has no 1:1 Go equivalent. The schema is defined in `internal/repository/schema.go` instead.
- **Lombok → plain Go struct.** Lombok is Java compile-time code generation. `internal/model/user.go` defines the fields directly.
- **`@ControllerAdvice` exception handling → error mapping.** Java exceptions became Go error values in `internal/service/errors.go`. They are translated to HTTP responses in `internal/httpapi/errors.go`.

## Known limitations

These components of the original could not be translated directly:

- **`UserNotFoundException(String, Throwable, boolean, boolean)` protected constructor.** This constructor controls suppressed-exception tracking and stack-trace writability, which are JVM `Throwable` features. It was dropped. "User not found" is represented as a lightweight Go error value instead.
- **Hibernate `ddl-auto` schema generation for `User`.** The original derived the table, the unique constraint on `User_Email`, the length of 500 for `User_About`, and primary key generation from annotations plus a properties flag. The Go schema must reproduce these explicitly and must be created at startup or by migrations.
- **Lombok annotations on `User`.** Java-specific code generation. The Go struct must not expose the password in serialized output or logs.
- **Spring Data JPA runtime proxy for `UserDao`.** Only the CRUD methods actually used by callers, plus `FindByName`, should exist in the Go repository. Any other inherited `JpaRepository` behavior is not available.
- **JPA persistence context / dirty checking.** Hibernate automatically flushed changes to managed entities at transaction commit. Go has no such mechanism. If the original service mutated a loaded `User` inside a `@Transactional` method without calling `save()`, the Go code must issue an explicit update.

## Manual review required

The migration flagged these as low-confidence. Verify them by hand:

1. **`User` model** (`internal/model/user.go`, `internal/repository/schema.go`)
   - Table and column names match the original schema.
   - `User_Email` has a unique constraint.
   - `User_About` has length 500.
   - Primary key generation matches the original.
   - The password is excluded from JSON responses and string output.
   - The schema is actually created at startup, or a migration step is documented.
2. **Exception handling** (`internal/httpapi/errors.go`, `internal/model/error_message.go`)
   - HTTP status codes and error response bodies match the original for each error case, including user-not-found and validation errors.
3. **User service** (`internal/service/user_service.go`)
   - Every update path persists changes with an explicit repository call. Check especially for any original logic that relied on JPA dirty checking.
   - Transactional boundaries are preserved where the original used `@Transactional`.
4. **Repository** (`internal/repository/user_repository.go`)
   - `FindByName` and the CRUD methods used by the service behave like their Spring Data counterparts, including the "not found" result.
5. **Configuration** (`internal/config/config.go`)
   - Document the expected `DATABASE_URL` format.
   - Confirm how `JWT_SECRET` is used and whether missing values fail fast at startup.