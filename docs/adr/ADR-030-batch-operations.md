# ADR-030: Batch Operations

- **Status:** Accepted

## Context

Applications frequently need to process multiple records in a single operation.

Batch APIs must remain consistent with existing ORM behavior so developers do not have to learn separate semantics for single-record and multi-record flows.

The batch layer must preserve the same architectural guarantees as the rest of the ORM:

- access policy enforcement
- audit field population
- transaction safety
- lifecycle hooks
- observability
- error wrapping

Batch operations must also remain deterministic and predictable so they can be used safely in production systems.

## Decision

The ORM will provide dedicated batch APIs for multi-record operations.

The initial batch surface will include:

- `CreateMany`
- `UpdateMany`
- `DeleteMany`

These operations will follow the same architectural rules as their single-record counterparts while operating over collections of entities.

Batch behavior must remain explicit. The ORM will not infer batch semantics from single-record methods or hidden heuristics.

## Consistency

Batch operations must integrate with the same subsystems as single-record operations:

- Access Policy
- Audit Fields
- Transactions
- Lifecycle Hooks
- OpenTelemetry
- Error Model

Each batch operation must preserve predictable behavior across every entity in the batch.

## Transactions

Batch operations execute inside a transaction by default.

This preserves atomicity and keeps batch work aligned with the ORM's existing transactional guarantees.

Applications may disable automatic transaction management explicitly when they need lower-level control.

## Hooks

Lifecycle hooks must execute for every entity in the batch.

Hook ordering must remain deterministic and must match the behavior of the corresponding single-record operations.

Future performance optimizations must not change observable hook behavior.

## Access Policy

Access Policy enforcement remains active during batch operations.

Company isolation and row-level policies must apply to every entity processed by the batch API.

Batch APIs must never bypass security controls or create a separate policy model.

## Audit Fields

Audit fields must be populated automatically for every entity affected by a batch operation.

Developers should not be required to assign audit values manually as part of batch processing.

## Observability

Batch operations must integrate naturally with ADR-021.

The ORM should record a single batch span that includes:

- batch size
- execution duration
- affected rows

The span should include relevant events for batch progress and failure conditions.

## Error Handling

Batch operations must integrate with ADR-027.

Errors must preserve wrapped driver errors and identify the failing batch operation clearly.

When a batch fails, the returned error should provide enough context for callers to determine which operation failed and why.

## Performance

Batch operations should support configurable batch size.

The implementation should avoid unnecessary allocations and should reuse prepared statements where appropriate.

Behavior must remain deterministic even when performance optimizations are introduced later.

## Future Compatibility

The batch architecture must remain compatible with future capabilities such as:

- `UpsertMany`
- COPY protocol support
- bulk import
- retry policies

These additions should not require changes to the public batch API.

## Consequences

- The ORM gains a clear, explicit API for multi-record operations.
- Batch behavior remains aligned with single-record semantics.
- Security, auditing, and observability stay consistent across all write paths.
- The implementation can evolve for performance without changing the public contract.

## Alternatives Considered

### Reusing single-record APIs for collections

### Database-specific bulk SQL helpers

### COPY-only batch support

### Client-side looping over single-record operations

## Why Alternatives Were Rejected

### Reusing single-record APIs for collections

Rejected because batch semantics should be explicit and discoverable rather than inferred from overloaded method behavior.

### Database-specific bulk SQL helpers

Rejected because they would weaken portability and make behavior harder to reason about across ORM subsystems.

### COPY-only batch support

Rejected because COPY is efficient but too specialized to serve as the only batch abstraction for the ORM.

### Client-side looping over single-record operations

Rejected because it would duplicate control flow, increase overhead, and make atomic batch behavior harder to guarantee.
