# ADR-022: Connection Layer & Driver Architecture

- **Status:** Accepted

## Context

The ORM needs a consistent way to open and manage database connections without embedding database-specific behavior in the core packages. Connection handling is not just an implementation detail; it is the base layer for runtime ORM usage, migrations, schema inspection, drift detection, CLI operations, observability, and connection pooling.

The project is PostgreSQL-first, but the architecture must remain open to future SQL dialects without requiring changes to the ORM core.

The public API should feel idiomatic to Go developers. That means a small number of entry points, explicit `context.Context` support, and an API style familiar from Go's standard library rather than a framework-heavy configuration surface.

The architecture also needs one source of truth for database configuration. The registered driver instance must own that configuration and be reused everywhere the database is accessed. Database configuration must not be duplicated across ORM, migration, schema, CLI, observability, or code generation layers.

## Decision

The project will use an `Open()`-based API inspired by Go's `database/sql` package.

The ORM owns the connection lifecycle and returns a `*dorm.DB` handle that represents the opened database connection and associated runtime behavior.

Database-specific connection behavior belongs in driver implementations. The ORM core must not construct DSNs directly, infer PostgreSQL-specific defaults, or know how to initialize a particular SQL driver.

The driver instance is the single source of truth for database configuration. Applications create one driver configuration, register that driver once, and reuse it consistently across ORM, migration, schema, CLI, observability, and future code generation paths.

All database operations must originate from the registered driver. The expected flow is:

```text
Application

↓

Register Driver

↓

dorm.Open()

↓

*dorm.DB

↓

Migration

↓

Schema Check

↓

ORM Operations
```

Every subsystem should share the same database abstraction rather than constructing its own connection path.

## Public API

The primary API should be compact and predictable:

```go
driver := postgres.New(postgres.Config{
    Host:     "localhost",
    Port:     5432,
    Database: "app",
    Username: "postgres",
    Password: "secret",
})

db, err := dorm.Open(ctx, driver)
```

The project will expose a single primary entry point for opening a database connection. It should avoid multiple overlapping constructors such as `Connect()`, `NewDatabase()`, or `NewClient()`.

The CLI is considered part of the application, not a globally installed standalone tool. The recommended execution model is:

```bash
go run ./cmd/dorm migrate run
```

or a compiled project-local binary.

Every application using dorm should own its CLI entry point. A typical structure is:

```text
cmd/
└── dorm/
    └── main.go
```

The CLI should be compiled together with the application so it naturally inherits application configuration, environment variables, driver registration, observability, logging, and dependency injection. It should not require a separate configuration system.

## Driver Architecture

The ORM will define a `Driver` abstraction and depend on that interface rather than on concrete database packages.

Driver responsibilities include:

- opening connections
- building DSNs or equivalent connection strings
- applying driver-specific configuration and defaults
- returning the SQL driver or connection primitive required by the runtime
- exposing the SQL dialect
- exposing driver capabilities
- reporting driver-specific validation errors

The ORM core should only interact with the driver interface and should not embed knowledge of PostgreSQL, MySQL, SQL Server, or any other database family.

Drivers are the place for database-specific bootstrapping. The core must stay generic.

The application is responsible for registering its driver. A typical pattern is:

```go
driver := postgres.New(postgres.Config{
    ...
})

dorm.RegisterDriver(driver)
```

Once registered, the same driver must be reused by the ORM, migration engine, schema checker, CLI, and any future code generation tooling. No component should instantiate its own driver independently.

## PostgreSQL Driver

The PostgreSQL driver will encapsulate PostgreSQL-specific concerns such as:

- DSN construction
- SSL mode
- timezone
- search path
- connection options
- driver-specific defaults

This keeps PostgreSQL behavior close to the code that owns it and prevents the ORM core from accumulating database-specific branching logic.

## Connection Pooling

Connection pooling belongs to the connection layer and must be configurable through the same driver-owned configuration path.

The architecture should support configuring:

- maximum open connections
- maximum idle connections
- connection lifetime
- idle timeout
- health checks
- `Ping`
- graceful `Close`

Defaults should be sensible for production use, but callers must be able to override them explicitly when application needs require it.

Connection pooling must therefore remain part of the shared driver lifecycle rather than becoming a separate configuration surface for each subsystem.

## Configuration Philosophy

Configuration should be strongly typed and compact.

Example:

```go
postgres.Config{
    Host:     "...",
    Port:     5432,
    Database: "...",
}
```

The architecture should avoid long parameter lists and avoid builder-heavy APIs that obscure the actual connection settings.

Validation should happen before the connection attempt whenever possible, so misconfiguration fails fast and produces actionable errors.

The architecture must not introduce additional configuration files such as `database.yml`, `orm.yml`, `dorm.toml`, or `migration.json`. Configuration should remain type-safe, explicit, and validated through Go code.

## Context Support

The connection API must accept `context.Context`.

The provided context becomes the root for:

- OpenTelemetry tracing
- deadlines
- cancellation
- access context
- logging correlation

Future operations initiated from the opened database handle should continue to inherit the same context-driven behavior rather than introducing parallel context models.

## Observability

The connection layer must integrate naturally with ADR-021.

Tracing should be created automatically for:

- `Open`
- `Ping`
- `Close`

The connection layer should also expose connection pool metrics and structured failures suitable for logs and traces.

Connection failures should be observable by default, with enough context to diagnose configuration problems, network issues, authentication errors, and driver initialization failures.

CLI operations must inherit the application's observability configuration automatically. Migration execution, schema inspection, schema drift detection, connection open, and connection close should all emit OpenTelemetry traces without requiring separate CLI instrumentation.

## Error Handling

Connection errors must preserve the underlying driver error while adding enough structure to make the failure actionable.

The architecture should:

- wrap errors consistently
- avoid hiding PostgreSQL error details
- preserve driver-specific diagnostics
- fail configuration validation before connection establishment when possible

The ORM should not replace low-level errors with generic messages that make operational debugging harder.

## Driver Responsibilities

Drivers own:

- DSN generation
- driver-specific defaults
- SQL driver initialization
- database capability detection

Drivers do not own:

- query building
- ORM logic
- migration logic
- access engine behavior
- schema builder behavior

Those responsibilities belong to their dedicated packages.

## Extensibility

The connection architecture must support additional drivers without modifying the ORM core.

Future drivers such as MySQL or SQL Server should implement the same common interface and plug into the existing opening flow.

This preserves the architecture contract while allowing new dialects and connection models to be added incrementally.

## Testing

The architecture should support:

- mock drivers
- fake drivers
- integration testing
- PostgreSQL Docker testing

These testing modes should work without changing ORM code or introducing special-case test hooks into the core connection path.

## Repository Structure

A structure similar to the following is recommended:

```text
dorm/
├── access/
├── cmd/
├── dialect/
├── driver/
│   └── postgres/
├── migrate/
├── orm/
├── schema/
└── observability/
```

Package boundaries should be interpreted as follows:

- `orm` owns runtime database access and the public `Open()` entry point
- `driver` contains the shared driver abstractions
- `driver/postgres` contains PostgreSQL-specific connection behavior and defaults
- `dialect` contains SQL rendering and capability-aware abstractions
- `migrate` owns migration generation and execution
- `schema` owns database inspection and drift detection
- `access` owns access-control behavior
- `observability` owns tracing, metrics, and logging integration
- `cmd` owns project-local CLI entry points that reuse the same application driver

The repository structure should make it clear that connection setup is foundational, but not a place for unrelated ORM features.

The CLI should orchestrate existing ORM packages instead of duplicating behavior. For example:

```text
migrate run

↓

migrate.Run(db)
```

```text
schema check

↓

schema.Check(db)
```

```text
migrate generate

↓

migrate.Generate(driver.Dialect())
```

The CLI must not reimplement business logic that already exists in the runtime packages.

## Consequences

- The public API remains small and familiar to Go developers.
- The registered driver becomes the single source of truth for all database configuration.
- Database-specific logic stays in driver packages instead of leaking into the ORM core.
- ORM, migrations, schema tools, CLI, observability, and code generation can reuse one driver instance and one configuration source.
- Connection pooling and operational behavior are centralized and easier to reason about.
- New SQL dialects can be added by implementing the driver interface rather than redesigning the core.
- Testing becomes easier because the connection layer can be mocked or faked at the driver boundary.
- The developer experience stays consistent because application code and project-local CLI commands share the same database setup.

## Alternatives Considered

### Multiple connection constructors

Expose separate APIs such as `Connect()`, `NewDatabase()`, or `NewClient()`.

### Core-owned DSN construction

Let the ORM build connection strings and encode database-specific defaults itself.

### Config file driven connection setup

Store connection settings in separate YAML, JSON, or TOML files and load them outside Go code.

### Separate driver configuration per subsystem

Allow ORM, migration, schema, and CLI code to create their own database configuration independently.

### Global CLI installation

Ship the CLI as a globally installed standalone tool rather than compiling it with each application.

### External YAML or TOML configuration files

Load database settings from separate configuration files instead of Go code.

## Why Alternatives Were Rejected

### Multiple connection constructors

Rejected because it fragments the public API, increases ambiguity, and makes it harder to explain the canonical entry point.

### Core-owned DSN construction

Rejected because it couples the ORM to specific database implementations and makes future drivers harder to add without core changes.

### Config file driven connection setup

Rejected because it duplicates configuration sources, weakens type safety, and makes the application less explicit about how database access is configured.

### Separate driver configuration per subsystem

Rejected because it violates the single source of truth principle and creates drift between runtime, schema tooling, CLI, and observability behavior.

### Global CLI installation

Rejected because it decouples the CLI version from the application version, makes driver registration less predictable, and introduces a second operational artifact that can drift from the codebase.

### External YAML or TOML configuration files

Rejected because they duplicate configuration sources, weaken type safety, and move an important architectural decision out of compile-time Go code.
