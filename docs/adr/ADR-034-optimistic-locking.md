# ADR-034: Optimistic Locking

- **Status:** Accepted

## Context

Applications frequently update the same record concurrently.

Without concurrency protection, the last writer silently overwrites previous changes.

The ORM needs an optional concurrency-control mechanism that prevents lost updates without introducing pessimistic locking or forcing applications to manage row locks manually.

## Decision

The ORM will support optimistic locking through a composable model trait.

Example:

```go
type User struct {
    model.Entity
    model.Version

    Name string
}
```

Models opt into optimistic locking by embedding `model.Version`.

Models that do not embed the Version trait will not participate in optimistic locking and will continue to use the existing update behavior.

## Philosophy

Concurrency control should be explicit.

The Version trait makes the opt-in visible in code and keeps optimistic locking aligned with the ORM's compositional model design.

The ORM should avoid:

- struct tags
- boolean flags
- hidden configuration switches

## Version Trait

The Version trait introduces a managed column:

```text
version BIGINT NOT NULL DEFAULT 1
```

The ORM owns this column and updates it automatically.

Applications must never increment the version manually.

The trait should be reusable across models and should remain compatible with other composable traits such as entity identity, timestamps, and access-scoped fields.

## Update Behavior

During `Update`, the ORM should generate a guarded update that checks the current version before applying changes.

Conceptually:

```sql
UPDATE ...
SET
    version = version + 1
WHERE
    id = ?
    AND version = ?
```

If no rows are affected, the ORM must return `ErrOptimisticLockFailed`.

The update path must preserve the existing update semantics for fields, hooks, access policy, and transaction participation.

## Access Policy

Optimistic locking must integrate with ADR-023.

Automatic access-policy filters remain active and must be appended alongside the version predicate.

The version check is a concurrency constraint, not a security boundary, so it must never replace or weaken access-policy enforcement.

## Hooks

Optimistic locking must integrate with ADR-029.

Lifecycle execution order should remain deterministic:

1. `BeforeUpdate`
2. Optimistic lock predicate evaluation
3. Update execution
4. `AfterUpdate`

Hook behavior should remain unchanged except that the update may fail with an optimistic-lock conflict if the version predicate does not match.

## Transactions

Optimistic locking must behave consistently inside and outside transactions.

No additional transaction API is required.

The version check should participate in the current transaction boundary automatically and should fail atomically with the rest of the update operation.

## Observability

Optimistic locking must integrate with ADR-021.

The execution trace should record:

- current version
- next version
- conflict detection

When a version mismatch occurs, the trace should make the conflict visible without exposing sensitive internal state unnecessarily.

## Error Handling

Optimistic locking must integrate with ADR-027.

Version conflicts must return `ErrOptimisticLockFailed`.

Underlying driver errors and scan/update failures must remain wrapped and inspectable through standard Go error handling.

Applications should be able to distinguish a conflict from a transport or SQL failure with `errors.Is()`.

## Dry Run

Optimistic locking must integrate with ADR-032.

Execution inspection should show:

- current version
- next version
- optimistic locking enabled

Dry Run should explain the concurrency constraint without executing the update.

## Performance

Optimistic locking should add minimal overhead.

It should not require an additional query.

The version check must be folded into the existing update statement so concurrency protection stays efficient and predictable.

## Future Compatibility

The design should support future features without changing the public API, including:

- Batch Operations
- Relationship Engine
- Retry Policies

The optimistic-locking contract should remain stable even as additional execution modes and higher-level workflows are introduced.

## Consequences

- Applications can opt into safe concurrent updates without adopting pessimistic locking.
- Lost updates become detectable instead of silent.
- The concurrency mechanism stays composable and model-driven.
- Access policy, hooks, observability, transactions, and error handling remain part of the same update pipeline.
- The ORM preserves its explicit configuration style and avoids extra API surface for a capability that belongs on the model itself.

## Alternatives Considered

### Pessimistic locking with row locks

### A global optimistic-locking configuration flag

### Struct tags on version fields

### Requiring applications to implement manual compare-and-swap logic

## Why Alternatives Were Rejected

### Pessimistic locking with row locks

Rejected because it can block concurrent writers, increases contention, and is a heavier concurrency model than the ORM needs for ordinary record updates.

### A global optimistic-locking configuration flag

Rejected because concurrency behavior should be explicit at the model level, not controlled indirectly through runtime configuration.

### Struct tags on version fields

Rejected because the project favors composable traits and code-visible behavior over metadata-driven conventions for important runtime semantics.

### Requiring applications to implement manual compare-and-swap logic

Rejected because the ORM should own the concurrency pattern once the model opts in, otherwise every caller would need to reimplement the same error-prone update logic.
