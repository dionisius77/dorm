# ADR-028: Transaction API & Transaction Lifecycle

- **Status:** Accepted

## Context

Applications frequently need to execute multiple database operations as a single unit of work.

The transaction API should feel idiomatic to Go developers, minimize boilerplate, propagate context automatically, and integrate naturally with Access Policy and OpenTelemetry.

The ORM should own transaction lifecycle management so callers do not need to coordinate commit and rollback logic manually for the common case.

## Decision

The ORM will use a callback-based transaction API as the primary transaction interface.

Example:

```go
err := db.Transaction(ctx, func(tx *dorm.DB) error {
    // operations
    return nil
})
```

Behavior:

- `nil` return value commits the transaction
- non-nil return value rolls the transaction back

The framework will manage `Commit` and `Rollback` automatically for the callback API.

## Context Propagation

Transactions inherit the parent context automatically.

The transaction context includes:

- OpenTelemetry span
- Company
- User
- Access Policy
- deadlines
- cancellation
- logging correlation

Applications should not need to manually copy or rebuild transaction context.

## Nested Transactions

Nested transactions will be supported.

The PostgreSQL implementation should use `SAVEPOINT` rather than opening an independent database transaction for nested scopes.

Nested transactions must remain observable and must not break the outer transaction lifecycle.

## Manual Transactions

The ORM should still support manual transaction control when required.

Example:

```go
tx, err := db.Begin(ctx)
defer tx.Rollback()

// operations

err = tx.Commit()
```

The callback API remains the preferred default because it reduces boilerplate and lowers the risk of leaked transactions.

## Access Policy

Transactions inherit Access Policy from the parent context.

Changing policy inside a transaction must remain explicit.

Example:

```go
tx.WithPolicy(access.IgnoreCompany())
```

Policy changes affect only the current transaction scope.

## Observability

Transactions must integrate naturally with ADR-021.

Transactions should automatically create spans for:

- Begin
- Commit
- Rollback

Nested transactions should also be observable so developers can understand transaction boundaries during debugging and tracing.

## Error Handling

Transaction errors must integrate with ADR-027.

The architecture should support transaction-specific errors including:

- `ErrTransactionClosed`
- `ErrCommitFailed`
- `ErrRollbackFailed`

Underlying driver errors must remain accessible through standard Go error wrapping and inspection.

## Performance

Transaction handling should avoid unnecessary allocations and avoid additional abstraction layers.

The callback API should remain lightweight and direct.

## Future Compatibility

The transaction architecture should remain compatible with future features such as:

- batch operations
- relationship engine
- plugins
- retry policies

This must be done without breaking the public API.

## Repository Structure

The project should keep transaction lifecycle logic inside the runtime ORM layer rather than splitting it across unrelated packages.

Suggested organization:

```text
orm/
├── transaction.go
├── transaction_test.go
└── transaction_observability.go
```

## Consequences

- Callers can use a simple callback-based API for the most common transaction flow.
- Commit and rollback handling becomes safer because lifecycle control is centralized.
- Context, access policy, and observability all flow through the same transaction boundary.
- Nested transaction support becomes an implementation detail rather than an application burden.
- Manual transaction control remains available for advanced use cases.

## Alternatives Considered

### Only manual transactions

### A builder-based transaction API

### Separate transaction manager objects for every subsystem

### No nested transaction support

## Why Alternatives Were Rejected

### Only manual transactions

Rejected because it adds boilerplate and increases the risk of leaked or improperly finalized transactions.

### A builder-based transaction API

Rejected because it adds indirection without improving the core use case.

### Separate transaction manager objects for every subsystem

Rejected because it would fragment lifecycle management and make context, policy, and observability harder to keep consistent.

### No nested transaction support

Rejected because real applications often need composable transactional scopes, especially when higher-level services call lower-level ORM routines.
