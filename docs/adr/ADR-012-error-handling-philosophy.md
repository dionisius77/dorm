# ADR-012: Error Handling Philosophy

- **Status:** Accepted

## Context

This project operates in domains where failures are meaningful and should be explicit:

- invalid model definitions
- unsupported schema changes
- migration conflicts
- schema drift
- query execution errors
- access policy violations

Errors must be categorized so callers know whether they are facing a developer mistake, a runtime failure, a schema problem, or a migration problem.

## Decision

Error handling will follow these principles:

- Migration errors should be explicit and actionable.
- Query errors should preserve database details while remaining structured.
- Schema mismatch and drift errors should be distinct from execution errors.
- Invalid model definitions should fail early, preferably at build, generation, or startup time.
- Developer mistakes should be caught as soon as possible and reported with precise context.

The system should distinguish between:

- configuration errors
- invalid schemas
- unsupported features
- migration generation failures
- migration application failures
- runtime query failures
- access violations

## Consequences

- Callers can make safe decisions based on error class.
- Operational tooling can present useful diagnostics.
- Developers receive clearer feedback when they define invalid models or unsupported schema patterns.
- Production systems can separate transient database failures from structural problems.

## Alternatives Considered

### Single generic error type

Return one error value for all failures.

### Panic on most errors

Use panic for both programming errors and runtime failures.

### Database error passthrough only

Expose only raw PostgreSQL driver errors.

## Why Alternatives Were Rejected

### Single generic error type

Rejected because it prevents callers from handling different failures appropriately.

### Panic on most errors

Rejected because runtime database failures should be handled and reported, not treated as program crashes.

### Database error passthrough only

Rejected because it loses architectural meaning and makes the system harder to diagnose.

