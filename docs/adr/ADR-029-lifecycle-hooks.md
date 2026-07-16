# ADR-029: Lifecycle Hooks

- **Status:** Accepted

## Context

Applications often need deterministic lifecycle behavior around ORM operations.

Common examples include:

- validation
- slug generation
- password hashing
- audit customization
- business rule enforcement
- soft delete customization
- event publishing

The ORM needs a hook system that is explicit, extensible, and idiomatic to Go. Hook behavior must remain predictable and should not depend on reflection-based method discovery or magic method names.

## Decision

The ORM will use explicit, interface-based lifecycle hooks.

Models participate in lifecycle events by implementing well-defined hook interfaces that are detected through normal Go type assertions.

Hook execution will be deterministic, context-aware, and optional. Models that do not implement hook interfaces will incur no runtime overhead beyond a type assertion check.

## Supported Hooks

The initial hook set will include:

- `BeforeCreate`
- `AfterCreate`
- `BeforeUpdate`
- `AfterUpdate`
- `BeforeDelete`
- `AfterDelete`
- `AfterFind`

Future hook points may be added without breaking the existing interfaces.

## Interface Design

Each lifecycle event should be represented by its own interface.

Example:

```go
type BeforeCreateHook interface {
    BeforeCreate(ctx context.Context, tx *dorm.DB) error
}
```

The ORM should detect hook support by asserting the concrete model against the relevant interface at runtime.

## Execution Order

Hook execution must follow a deterministic order:

1. `BeforeCreate` / `BeforeUpdate` / `BeforeDelete`
2. ORM operation
3. `AfterCreate` / `AfterUpdate` / `AfterDelete`

`AfterFind` runs after query results have been scanned into destination values.

If a hook returns an error, lifecycle execution must stop immediately.

## Transactions

Hooks run inside the current transaction context.

If a hook fails:

- the current operation must stop
- the active transaction must be rolled back when the hook participates in a transactional flow
- the returned error must preserve the original cause

Hook implementations must receive the active transaction handle so they can participate in the same unit of work as the ORM operation.

## Context Propagation

Hooks must receive the same `context.Context` used by the ORM operation.

That context carries:

- Access Policy
- OpenTelemetry span context
- deadlines
- cancellation
- request-scoped values

Applications should not need to manually rebuild context for hooks.

## Observability

Lifecycle hook execution must integrate with ADR-021.

Hook execution should produce trace events and should be visible as part of the surrounding ORM span.

Hook failures must mark the active span as failed and preserve the underlying error value.

## Error Handling

Hook errors must integrate with ADR-027.

Errors returned from hooks should be wrapped with lifecycle context while preserving the original cause so callers can use `errors.Is()` and `errors.As()`.

Examples of useful wrapped context:

- `BeforeCreate(User)`
- `AfterUpdate(Product)`
- `AfterFind(Role)`

## Performance

Hook dispatch should avoid reflection during normal execution.

The ORM should prefer interface assertions and minimal allocation paths so models that do not implement hooks pay almost no additional cost.

## Batch Operations

Future batch operations must define predictable hook semantics.

The hook architecture should remain compatible with batch processing without forcing a redesign of the public hook interfaces.

## Future Compatibility

This architecture must remain compatible with future additions such as:

- relationship engine
- plugins
- validation framework
- event bus
- batch operations

No existing hook interface should need to change for these features to be added later.

## Repository Structure

Lifecycle hook implementation should live in the ORM runtime layer rather than being scattered across unrelated packages.

Suggested organization:

```text
orm/
├── hooks.go
├── hooks_relations.go
└── hooks_test.go
```

## Consequences

- Hook behavior becomes explicit and easy to reason about.
- Models can opt in only to the lifecycle events they need.
- The ORM preserves Go idioms by using interfaces instead of magic method names.
- Hook failures remain observable, inspectable, and compatible with standard error wrapping.
- The design remains extensible for future lifecycle events and related subsystems.

## Alternatives Considered

### Reflection-based method discovery

### Struct tags or configuration-driven hooks

### Global callback registry

### Framework-specific base model hooks only

## Why Alternatives Were Rejected

### Reflection-based method discovery

Rejected because it is less explicit, less discoverable, and more brittle than normal interface assertions.

### Struct tags or configuration-driven hooks

Rejected because hook semantics should live in code, not hidden metadata, and tags are a poor fit for lifecycle behavior.

### Global callback registry

Rejected because it makes hook behavior harder to trace, harder to compose, and more difficult to associate with a specific model.

### Framework-specific base model hooks only

Rejected because it would force unnecessary inheritance patterns and make hooks unavailable to models that should remain lightweight or compositional.
