# ADR-033: Raw SQL & Explicit Policy Bypass

- **Status:** Accepted

## Context

The ORM provides a high-level API for common database work, but some operations are naturally expressed as SQL.

Examples include:

- Common Table Expressions (CTE)
- window functions
- recursive queries
- materialized views
- PostgreSQL extensions
- vendor-specific SQL

The framework needs a native escape hatch for these cases without weakening its security model. In particular, access policy must never be bypassed implicitly.

## Decision

The ORM will expose a dedicated Raw SQL builder for explicit SQL execution.

Raw SQL is not a special case of the normal query builder. It is a distinct execution path with its own policy boundary.

Example usage:

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

Raw SQL must never apply access policy automatically.

Before execution, callers must explicitly acknowledge the bypass by calling `WithoutPolicy()`. If that acknowledgement is omitted, the ORM must return a typed error and must not execute the SQL.

The Raw SQL API should support both reads and writes:

```go
db.Raw(ctx, query, args...).WithoutPolicy().Scan(&users)
db.Raw(ctx, query, args...).WithoutPolicy().Exec()
```

Execution must remain deferred until an explicit terminal operation is invoked.

## Philosophy

Raw SQL is an explicit security decision.

The framework must never silently disable access policy, even when the SQL is authored by the application.

Instead:

- the ORM owns execution
- the developer owns query text
- the developer must opt out of policy explicitly

This keeps the escape hatch available without making it easy to misuse.

## Public API

The Raw SQL API should be idiomatic and builder-based.

Required behavior:

- `db.Raw(ctx, query, args...)` creates a builder but does not execute immediately
- `WithoutPolicy()` marks the request as an intentional policy bypass
- `Scan(...)` executes a read query
- `Exec()` executes a write or command query

The API should remain consistent with the ORM's existing builder style and should fit naturally alongside the normal query API.

## Parameter Binding

Raw SQL should use generic positional placeholders in the public API.

Example:

```sql
WHERE email = ?
```

The active dialect is responsible for converting those placeholders into the syntax required by the backend:

- PostgreSQL: `?` -> `$1`
- MySQL: `?` -> `?`
- SQL Server: `?` -> `@p1`

Placeholder conversion belongs to the dialect layer, not to the Raw SQL builder.

The Raw SQL builder must not parse or rewrite SQL structure. It may only delegate placeholder conversion to the dialect layer.

## Access Policy

Raw SQL never receives implicit access-policy injection.

It must never automatically add:

- company filters
- tenant filters
- workspace filters
- organization filters
- row-level policies
- soft-delete predicates

If a caller wants a privileged execution path, it must be made explicit with `WithoutPolicy()`.

If `WithoutPolicy()` is omitted, execution must fail with a typed error that clearly states the policy decision is missing.

## Transactions

Raw SQL must participate in the current transaction automatically.

If executed inside `db.Transaction(...)`, the Raw SQL builder must reuse the active transaction boundary and behave like any other ORM operation.

No separate transaction manager is required for the Raw SQL path.

## Observability

Raw SQL execution must integrate with ADR-021.

Each execution should emit traces with metadata indicating:

- raw SQL execution
- execution duration
- affected rows when available

The SQL text shown in traces must obey the configured SQL visibility mode.

Raw SQL tracing should remain low-cardinality and should reuse the same observability controls as the rest of the ORM.

## Error Handling

Raw SQL errors must integrate with ADR-027.

The Raw SQL path should return typed errors for:

- missing explicit policy acknowledgement
- SQL execution failure
- scan failure

Underlying driver errors must remain available through standard Go error inspection.

Error messages should point developers toward the explicit policy requirement rather than implying that policy bypass is automatic.

## Performance

Raw SQL should add minimal framework overhead.

The ORM must not:

- infer query intent
- parse SQL semantics
- rewrite SQL structure
- inject policy predicates

Only placeholder conversion and execution orchestration are permitted.

Prepared statement support should remain possible in future versions, but it is not required to change the Raw SQL API.

## Future Compatibility

The Raw SQL architecture should leave room for future extensions without breaking the public API.

Examples include:

- named parameters
- prepared statement cache
- SQL files
- COPY support
- query advisor integration
- dry-run inspection

These additions should build on the same explicit policy contract.

## Repository Structure

A dedicated package is recommended to keep Raw SQL behavior isolated from the normal ORM query builder.

Example layout:

```text
raw/
├── builder.go
├── scanner.go
├── executor.go
└── placeholder.go
```

Placeholder conversion should remain delegated to the dialect layer.

Raw SQL execution should remain independent from the ORM query builder.

## Consequences

- Applications get a safe escape hatch for native SQL.
- The ORM preserves explicit security boundaries instead of guessing intent.
- Dialect-specific placeholder handling stays centralized.
- Raw SQL can support advanced database features without expanding the normal query builder.
- Callers must make privileged behavior obvious in code.

## Alternatives Considered

### Fold raw SQL into the normal query builder

### Implicitly bypass access policy for raw SQL

### Force all raw SQL through a separate connection layer

### Reject raw SQL entirely

## Why Alternatives Were Rejected

### Fold raw SQL into the normal query builder

Rejected because native SQL is semantically different from ORM-generated queries and needs a distinct security contract.

### Implicitly bypass access policy for raw SQL

Rejected because access policy must never be disabled silently.

### Force all raw SQL through a separate connection layer

Rejected because it would fragment execution behavior, duplicate transaction handling, and weaken observability consistency.

### Reject raw SQL entirely

Rejected because advanced PostgreSQL use cases require an explicit escape hatch for correctness and ergonomics.
