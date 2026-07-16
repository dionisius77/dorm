# dorm

> **Policy-driven PostgreSQL ORM for Go.**
> Automatically enforce data access policies to eliminate forgotten tenant filters, while providing deterministic migrations, schema drift detection, built-in seeding, and first-class OpenTelemetry support.

## Why dorm?

Multi-tenant applications often contain queries like:

```go
db.Where("company_id = ?", companyID).
    Find(&products)
```

This works... until someone forgets the filter.

```go
// Missing company filter
db.Find(&products)
```

A single missing condition can expose data belonging to another company or tenant.

`dorm` is designed to prevent this class of bugs.

Instead of manually writing access filters everywhere, developers write business queries while `dorm` automatically injects the required access policies from `context.Context`.

```go
db.Find(ctx, &products)
```

Automatically becomes:

```sql
SELECT *
FROM products
WHERE company_id = $1
  AND deleted_at IS NULL
```

No boilerplate.

No forgotten filters.

Secure by default.

---

# Features

## Policy-Driven Access Engine

The primary feature of `dorm`.

Automatically applies:

* Company isolation
* Row-level access policies
* Soft delete filtering
* Context-aware security

Supports multiple policy levels:

* Default
* IgnoreCompany
* IgnoreRLS
* System

Designed for:

* SaaS platforms
* ERP
* WMS
* CRM
* Multi-tenant systems

---

## PostgreSQL First

Built specifically for PostgreSQL.

Rather than supporting every SQL dialect from day one, `dorm` focuses on delivering the best possible PostgreSQL experience.

---

## Deterministic Migrations

Model-driven migration generation.

```bash
go install github.com/dionisius77/dorm/cmd/orm

orm migrate generate
```

No automatic schema mutation.

Developers review generated migrations before execution.

```bash
orm migrate run
```

If no schema changes exist, no migration is generated.

---

## 🔍 Schema Drift Detection

Detect differences between:

* Go models
* PostgreSQL schema

before they become production issues.

---

## Seed Engine

Built-in, idempotent seed synchronization.

```go
seed.Sync(
    []Role{
        {
            Code: "ADMIN",
            Name: "Administrator",
        },
    },
    seed.Key("Code"),
)
```

Running the same seed repeatedly always produces the same final database state.

---

## OpenTelemetry

First-class observability.

Automatically traces:

* Queries
* Transactions
* Migrations
* Seeds
* Schema inspection

SQL visibility is configurable:

* Disabled
* Metadata
* Statement
* StatementWithArgs

---

## Raw SQL Escape Hatch

`dorm` also supports explicit native SQL for cases where the high-level API is not the best fit.

Raw SQL never bypasses access policy implicitly.

Developers must explicitly opt out:

```go
db.Raw(
    ctx,
    `
    SELECT *
    FROM users
    WHERE email = ?
    `,
    email,
).
    WithoutPolicy().
    Scan(&users)
```

Notes:

* `WithoutPolicy()` is required before `Scan()` or `Exec()`
* `?` placeholders are rebound by the active dialect
* Raw SQL participates in the current transaction automatically
* The ORM does not parse or rewrite SQL beyond placeholder conversion

---

## Composable Models

Choose only the capabilities your model requires.

Full entity:

```go
type User struct {
    model.Entity

    ID   uuid.UUID
    Name string
}
```

Company only:

```go
type Product struct {
    model.Company

    ID   uuid.UUID
    Name string
}
```

Raw model:

```go
type Country struct {
    ID   int
    Name string
}
```

---

# Installation

```bash
go get github.com/dionisius77/dorm
```

---

# Quick Start

## Connect

```go
driver := postgres.New(postgres.Config{
    Host:     "localhost",
    Port:     5432,
    Database: "app",
    Username: "postgres",
    Password: "secret",
})

db, err := dorm.Open(ctx, driver)
if err != nil {
    panic(err)
}

defer db.Close()
```

---

## Create a model

```go
type Product struct {
    model.Entity

    ID    uuid.UUID
    Name  string
    Price decimal.Decimal
}
```

---

## CRUD

```go
err := db.Create(ctx, &product)

err = db.Find(ctx, &products)

err = db.Update(ctx, &product)

err = db.Delete(ctx, &product)
```

---

# Access Policy

Access policies are automatically resolved from `context.Context`.

Normal application code:

```go
db.Find(ctx, &products)
```

No manual company filtering is required.

Override policies when needed.

Default:

```go
db.WithPolicy(access.Default())
```

Ignore company isolation:

```go
db.WithPolicy(access.IgnoreCompany())
```

Ignore row-level isolation:

```go
db.WithPolicy(access.IgnoreRLS())
```

System mode:

```go
db.WithPolicy(access.System())
```

Policy changes are explicit and observable.

---

# Migrations

Generate:

```bash
orm migrate generate
```

Run:

```bash
orm migrate run
```

Rollback:

```bash
orm migrate rollback
```

Schema verification:

```bash
orm schema check
```

---

# Seeds

Register seeders:

```go
seed.Register(
    RoleSeeder{},
    PermissionSeeder{},
    AdminSeeder{},
)
```

Run:

```bash
orm seed run
```

---

# Tracing

`dorm` integrates with OpenTelemetry out of the box.

Database operations automatically generate traces for:

* Query execution
* Transactions
* Migrations
* Schema inspection
* Seed synchronization

SQL trace visibility is configurable depending on the environment.

---

# CLI

All database tooling is included.

```bash
go install github.com/dionisius77/dorm/cmd/orm

orm --help

orm migrate generate

orm migrate run

orm migrate rollback

orm schema check

orm seed run

orm analyze --sql "SELECT * FROM users WHERE email = $1"
```

The CLI reuses the same Driver configuration as the application, ensuring a single source of truth for database access.

---

# Example Applications

Explore complete examples:

```text
examples/
├── basic/
├── todo/
└── multi-tenant/
```

---

# Philosophy

`dorm` is built around a small set of principles:

* Policy-driven data access
* Secure by default
* Explicit over magic
* Deterministic schema management
* PostgreSQL first
* Production-ready observability
* Idiomatic Go APIs

---

# Documentation

```text
docs/
├── adr/
├── architecture/
├── guides/
└── examples/
```

---

# Roadmap

Current priorities:

* Production hardening
* Performance optimization
* Relationship API
* Plugin ecosystem
* Additional SQL dialects

---

# Contributing

Contributions are welcome.

Before opening a Pull Request:

* Run unit tests
* Run integration tests
* Run benchmarks
* Ensure examples compile
* Follow the project's architectural decisions (ADR)

---

# License

MIT License.
