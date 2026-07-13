# dorm

`dorm` is a PostgreSQL-first ORM for Go.

It is designed around explicit schema changes, deterministic migrations, schema drift detection, and context-aware access control.

## What This Project Is

- PostgreSQL-first, not database-agnostic by default
- Model-driven, with models as the source of truth
- Migration-based, with no AutoMigrate behavior
- Context-aware, for company, tenant, and audit injection
- Observable by design, with an OpenTelemetry-ready API surface

## Package Map

- `orm` - runtime CRUD, queries, transactions, and session handling
- `migrate` - model parsing, diffing, migration generation, and execution
- `schema` - schema representation, snapshots, and drift comparison
- `access` - context-aware ownership and audit injection
- `dialect` - SQL rendering abstractions

## Quickstart

### 1. Define a model

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID        uuid.UUID `orm:"pk"`
	Email     string    `orm:"unique"`
	CompanyID string    `orm:"company"`
	CreatedAt time.Time `orm:"created_at"`
	UpdatedAt time.Time `orm:"updated_at"`
}
```

### 2. Build the ORM

```go
db := orm.New(orm.Config{
	Dialect: postgres.New(),
	Schema:   expectedSchema,
	Observability: orm.DefaultObservabilityConfig(),
})
```

### 3. Use a context

```go
ctx := access.WithContext(context.Background(), access.Context{
	UserID:    "user-123",
	CompanyID: "company-123",
})
```

### 4. Query data

```go
var users []models.User
err := db.WithContext(ctx).Find(&users)
```

### 5. Create data

```go
u := models.User{
	Email: "alice@example.com",
}

err := db.WithContext(ctx).Create(&u)
```

## CLI Tutorial

The CLI is part of the workflow for schema changes.

### Initialize a project

```bash
orm init
```

### Generate a migration

```bash
orm migrate generate
```

### Apply migrations

```bash
orm migrate run
```

### Check drift

```bash
orm schema check
```

### Inspect status

```bash
orm migrate status
```

## Recommended Workflow

1. Edit Go models.
2. Generate a migration.
3. Review the generated SQL.
4. Apply the migration.
5. Run schema drift checks in CI and at startup.

## Tutorial Notes

- The project does not use AutoMigrate.
- Schema generation starts from models, not from live database state.
- Soft delete, company injection, and audit fields are driven by model metadata and request context.
- Observability is part of the architecture, but full tracing and metrics wiring are added behind the API surface.

## Development

```bash
go test ./...
```

## Architectural Rules

- Keep schema changes explicit.
- Keep PostgreSQL as the primary target.
- Keep public APIs small and stable.
- Keep security and correctness ahead of convenience.

## Docs

- [Architecture Decision Records](docs/adr/README.md)

## License

This project is licensed under the [MIT License](LICENSE).
