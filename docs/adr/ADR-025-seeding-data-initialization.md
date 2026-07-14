# ADR-025: Seeding & Data Initialization

- **Status:** Accepted

## Context

Most applications need a deterministic way to initialize baseline data such as roles, permissions, countries, currencies, languages, default settings, and system users. This data is not schema, and it does not belong in migrations. It is also not part of the model definition.

The ORM therefore needs a dedicated seeding system that is safe to run repeatedly, integrates with the existing `*dorm.DB` lifecycle, and fits the project’s PostgreSQL-first architecture without introducing a separate database configuration path.

## Decision

The ORM will introduce a dedicated Seed Engine.

Seeding is a separate concern from models, the migration engine, and the schema builder. Seeders will operate through the same `*dorm.DB` instance used by the application, and the registered driver remains the single source of truth for database access.

Seeders will describe desired data rather than raw SQL operations. The framework will reconcile the database state to match that desired data.

## Seeder Philosophy

Seeders define intent, not implementation details.

The seeding layer should express what data must exist, not how to construct SQL for inserting or updating it. The framework owns the mechanics of determining whether records should be inserted, updated, or left unchanged.

## Idempotency

All seed operations must be idempotent.

Running the same seeder multiple times must always produce the same final database state. Duplicate records must not be created, and existing records must be updated only when the configured seed key identifies the same logical entity.

## Public API

The public API should express developer intent directly.

Instead of exposing database-oriented methods inside seeders, the framework should provide synchronization primitives such as:

```go
seed.Sync(
    Role{
        Code: "ADMIN",
        Name: "Administrator",
    },
    seed.Key("Code"),
)
```

Collections should be supported as well:

```go
seed.Sync(
    []Role{
        {
            Code: "ADMIN",
            Name: "Administrator",
        },
        {
            Code: "USER",
            Name: "User",
        },
    },
    seed.Key("Code"),
)
```

The framework will determine whether each record should be inserted, updated, or ignored based on the configured key.

## Seeder Interface

Seeders should use a small interface that is easy to implement and compose.

At minimum, a seeder should expose:

- `Name`
- `Run`
- optional dependencies

The runner should resolve execution order and invoke each seeder through a consistent lifecycle.

## Seeder Registration

Applications will register seeders explicitly.

Registration should be straightforward and ordered only by dependencies, not by global discovery:

```go
seed.Register(
    RoleSeeder{},
    PermissionSeeder{},
    AdminSeeder{},
)
```

The Seed Engine will discover and execute the registered seeders.

## Dependencies

Seeders may depend on other seeders.

The execution order should be resolved automatically from declared dependencies so that foundational data such as roles can run before dependent data such as permissions or admin users.

Circular dependencies must fail with a clear error.

## CLI Integration

The CLI should support:

- `seed run`
- `seed list`
- `seed reset` as an optional operation
- `seed verify` as a future operation

The CLI must reuse the application’s registered driver and must not require separate database configuration.

## Transactions

Each seeder should run in a transaction by default.

If a seeder fails, only that seeder’s transaction should roll back unless the caller explicitly chooses a broader transactional boundary. The framework should also support executing all seeders in a single transaction when the application requires it.

## Observability

The seeding system must integrate with ADR-021.

Each seeder should emit traces and metrics automatically, including events such as:

- `seed.run`
- `seed.sync`
- `seed.insert`
- `seed.update`

Execution duration should be recorded, and failures should be marked as span errors.

## Environment Support

The architecture should support environment-specific seeders for development, testing, staging, and production.

The framework should encourage reuse and avoid duplicating seed logic across environments where possible.

## Future Extensibility

The Seed Engine should be designed so future features can be added without changing the public API.

Examples include:

- JSON seed sources
- YAML seed sources
- CSV import
- generated seed data
- faker integration
- dry run mode
- seed diff
- seed verification

## Repository Structure

The seeding package should remain separate from models and migrations.

An implementation may follow a structure similar to:

```text
seed/
├── role.go
├── permission.go
├── admin.go
└── runner.go
```

Applications should keep seed definitions isolated from schema definitions so that schema concerns and data initialization remain clearly separated.

## Consequences

- Initial data becomes a first-class concern instead of a migration workaround.
- Seeding can be rerun safely without creating duplicates.
- Applications can express data initialization in Go code using the same database abstraction as the rest of the ORM ecosystem.
- Seed execution can be ordered, observed, and tested in a predictable way.
- The design leaves room for future import formats and dry-run workflows without changing the core API.
- The system adds another lifecycle layer that must remain disciplined about transactions and dependency management.

## Alternatives Considered

### Put seed data in migrations

Use migration files to insert initial data alongside schema changes.

### Put seed data in model definitions

Attach default data or initialization logic directly to model types.

### Expose raw SQL helpers for seeding

Let seeders execute ad hoc SQL inserts and updates directly.

### Use global auto-discovery

Discover seeders implicitly through filesystem scanning or package initialization.

## Why Alternatives Were Rejected

### Put seed data in migrations

Rejected because schema evolution and data initialization are separate concerns. Mixing them makes migrations harder to reason about, harder to rerun safely, and more likely to entangle schema changes with application-specific data lifecycle.

### Put seed data in model definitions

Rejected because models should describe schema, not initialization policy. Embedding seed logic in models reduces clarity and makes the schema layer do too much work.

### Expose raw SQL helpers for seeding

Rejected because raw SQL would shift responsibility for idempotency, reconciliation, and portability to application code. The framework should own synchronization semantics.

### Use global auto-discovery

Rejected because explicit registration is easier to reason about, more testable, and less surprising in large applications. It also fits the project’s preference for deliberate configuration over hidden magic.
