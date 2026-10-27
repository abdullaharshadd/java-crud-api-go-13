# Migration Notes

**Model self-assessed confidence:** 95% (not a measured result)  
**Build at time of writing:** passed  
**Target unit tests:** passed  
**Behavior compared with the original:** passed

The final recommendation is in the pull request description.

---

## What was migrated

- `src/main/java/com/smartContact/error/UserNotFoundException.java` → `internal/service/errors.go` (86% confidence)
- `src/main/java/com/smartContact/model/User.java` → `internal/model/user.go` (72% confidence) ⚠️ needs review
- `src/main/java/com/smartContact/model/ErrorMessage.java` → `internal/model/error_message.go` (88% confidence)
- `src/main/java/com/smartContact/repository/UserDao.java` → `internal/repository/user_repository.go` (87% confidence)
- `src/main/java/com/smartContact/service/UserService.java` → `internal/service/user_service.go` (87% confidence)
- `src/main/java/com/smartContact/error/RestResponseEntityExceptionHandling.java` → `internal/httpapi/errors.go` (84% confidence) ⚠️ needs review
- `src/main/java/com/smartContact/Controller/UserController.java` → `internal/httpapi/handler.go` (86% confidence)
- `src/main/java/com/smartContact/service/UserServiceImp.java` → `internal/service/user_service.go` (72% confidence) ⚠️ needs review

## Components that could not be automatically migrated

These components require manual implementation. The migrated code contains
`MIGRATION_NOTE` comments at the relevant locations.

### `UserNotFoundException(String, Throwable, boolean, boolean) protected constructor` in `src/main/java/com/smartContact/error/UserNotFoundException.java`
**Reason:** Controlling suppressed-exception tracking and stack trace writability is a JVM Throwable feature with no direct equivalent in most target runtimes.
**Suggestion:** Drop this constructor in the target. If it was used for performance (no stack trace), use a lightweight error value instead, such as a Go sentinel error or a Python exception raised without traceback concerns.

### `User (@Entity schema auto-generation via Hibernate ddl-auto)` in `src/main/java/com/smartContact/model/User.java`
**Reason:** Hibernate's implicit DDL generation from annotations at ApplicationContext startup has no 1:1 equivalent in most targets. The table, unique constraint, column lengths and PK generation all come from annotations plus a properties flag.
**Suggestion:** Define an equivalent ORM model (e.g. SQLAlchemy/TypeORM/GORM/EF Core/Prisma) with the same table and column names, the unique constraint on User_Email and length 500 for User_About. Then explicitly create the schema at boot (e.g. Base.metadata.create_all, synchronize, AutoMigrate, Database.Migrate()), or wire migrations so they run automatically on startup.

### `User (Lombok annotations)` in `src/main/java/com/smartContact/model/User.java`
**Reason:** Compile-time code generation specific to Java.
**Suggestion:** Use the target language's native data-class/record/struct features or write the accessors manually. Exclude password from toString/serialization.

### `UserDao (JpaRepository runtime proxy generation)` in `src/main/java/com/smartContact/repository/UserDao.java`
**Reason:** Spring Data synthesizes the implementation at runtime from the interface and method names, and Go has no equivalent reflection-based interface implementation.
**Suggestion:** Manually write a Go repository struct with explicit SQL (sqlx) or gorm calls. Implement only the inherited CRUD methods actually used by callers, plus FindByName.

### `JPA persistence context / dirty checking` in `src/main/java/com/smartContact/repository/UserDao.java`
**Reason:** Hibernate automatically flushes changes to managed entities at transaction commit. Code may rely on modifying a loaded User without calling save().
**Suggestion:** Audit service code for implicit updates (entity mutated inside @Transactional without an explicit save) and add explicit Update calls in Go.

## Observer agent findings

The Observer agent monitored the migration and identified these patterns:

- **After 3 modules:** Two things drive confidence down: the target expert treats Java null/empty and string-format semantics loosely, and it defers required side effects (schema DDL) to files it never produces, so the validator cannot verify behavior.
- **After 6 modules:** Java null semantics (null fields, null exception messages, JPA-generated schema) are being mapped to Go zero values, fallbacks, or unverified claims instead of being reproduced faithfully.

## Files requiring manual review

These files were migrated but scored below the confidence threshold.
Review them carefully before merging.

### `src/main/java/com/smartContact/model/User.java`
Confidence: 72%
Issues:
  - [warning] The model says the DDL lives in internal/repository/schema.go, but searching the target for 'CREATE TABLE', 'user_email' and 'UserTable' found nothing. I could not confirm that the table and its unique constraint are created at startup.

### `src/main/java/com/smartContact/error/RestResponseEntityExceptionHandling.java`
Confidence: 84%
Issues:
  - [info] In Java, a null message gives a JSON null. Here a UserNotFoundError with no message produces whatever Error() returns, which may be a fallback string instead of null.

### `src/main/java/com/smartContact/service/UserServiceImp.java`
Confidence: 72%
Issues:
  - [info] The upsert runs as an update and then an insert, with no transaction around them, so it is not atomic. When the id does not exist, the row is inserted with a new generated id. That matches merge with an IDENTITY generator.
  - [info] Returns the mutated user instead of void, which is a harmless addition.
