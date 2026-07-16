# ADR-027: Error Model & Error Handling

- **Status:** Accepted

## Context

The project currently returns errors from multiple subsystems, including the ORM, driver, migration engine, schema tooling, seeding, CLI, access engine, query builder, and observability layers.

As the codebase grows, inconsistent error handling would make applications harder to debug, harder to test, and harder to integrate with Go's standard error inspection patterns.

The project needs a unified error model before the API surface becomes more stable.

## Decision

The ORM ecosystem will use idiomatic Go error handling everywhere.

Errors must support:

- `errors.Is()`
- `errors.As()`
- wrapping with `%w`
- contextual information

String comparison must never be required for normal error handling.

## Principles

Errors should be:

- explicit
- typed where appropriate
- composable
- easy to inspect
- easy to wrap

The project will avoid:

- magic error codes
- exceptions
- hidden failures

## Sentinel Errors

Common sentinel errors will be exposed as package-level variables from a dedicated error package.

Examples include:

- `ErrNotFound`
- `ErrAlreadyExists`
- `ErrConflict`
- `ErrInvalidModel`
- `ErrInvalidRelationship`
- `ErrMigrationRequired`
- `ErrSchemaDrift`
- `ErrInvalidContext`
- `ErrMissingCompany`
- `ErrPolicyDenied`
- `ErrSeedConflict`
- `ErrDriverNotRegistered`
- `ErrUnsupportedDialect`
- `ErrTransactionClosed`
- `ErrOptimisticLockFailed` for future use

Applications should be able to write:

```go
errors.Is(err, dorm.ErrNotFound)
```

## Typed Errors

Some failures require richer context than a sentinel error alone can provide.

The project should define typed errors for cases such as:

- `MigrationError`
- `SchemaError`
- `ValidationError`
- `DriverError`
- `AccessError`

Typed errors should expose useful fields while still working with `errors.As()`.

## Error Wrapping

Errors must preserve the original cause.

Example:

```go
return fmt.Errorf("create user: %w", err)
```

The ORM should not discard underlying driver errors, and it should not replace PostgreSQL errors unnecessarily.

## Driver Errors

Drivers should wrap database driver errors rather than flattening them.

The original PostgreSQL error must remain accessible through `errors.As()`, so applications can inspect vendor-specific details when needed.

## CLI Errors

CLI commands should return actionable errors.

The message should explain:

- what happened
- why it happened
- how to fix it when possible

For example, a migration failure should not only say `migration failed`; it should also describe the failing statement or file and provide a hint when recovery is likely.

## Migration Errors

Migration failures should include, when available:

- migration file
- statement
- line number
- underlying database error

## Schema Errors

Schema drift errors should clearly describe:

- expected state
- actual state
- the difference

Generic mismatch messages are not sufficient for drift diagnostics.

## Access Policy Errors

Access-related failures should be distinguishable.

Examples include:

- missing company context
- invalid access context
- policy denied
- unsupported policy

Applications should be able to inspect these failures without parsing strings.

## Seed Errors

Seed synchronization errors should indicate:

- model
- key
- conflicting values

The error should avoid exposing unnecessary implementation details.

## Observability

Errors must integrate naturally with ADR-021.

When an operation fails, the tracing layer should:

- mark the span as failed
- record the error event
- preserve stack context where possible

## Logging

Errors should remain structured values, not formatted log output.

Logging and error construction must remain separate responsibilities.

## Future Compatibility

The error model should remain compatible with future features such as:

- batch operations
- relationship engine
- plugin system
- additional SQL dialects

This must be done without requiring breaking API changes.

## Repository Structure

The project should prefer a dedicated error package rather than scattering sentinel values across unrelated packages.

Example layout:

```text
errors/
├── sentinel.go
├── migration.go
├── schema.go
├── access.go
├── driver.go
├── seed.go
└── helpers.go
```

## Consequences

- Applications can use standard Go error inspection consistently across packages.
- The ORM can preserve lower-level database details without exposing implementation-specific string matching as a contract.
- Typed errors give callers enough context to react programmatically to migration, schema, access, and seed failures.
- A dedicated error package creates a clear home for shared sentinels and helper types.
- The architecture gains a stable foundation for future features without forcing a redesign of error handling.

## Alternatives Considered

### Ad hoc string errors in each package

### A global numeric error code system

### Panic-based flow control for unrecoverable states

### A single generic error type for every failure

## Why Alternatives Were Rejected

### Ad hoc string errors in each package

Rejected because string matching is fragile and makes application code difficult to maintain.

### A global numeric error code system

Rejected because it is not idiomatic Go and adds another layer that callers must translate before understanding the failure.

### Panic-based flow control for unrecoverable states

Rejected because the ORM ecosystem should expose recoverable failures as errors, not force exception-like control flow.

### A single generic error type for every failure

Rejected because it loses domain-specific context that callers need for migration, schema, access, seed, and driver failures.
