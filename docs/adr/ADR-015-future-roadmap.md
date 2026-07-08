# ADR-015: Future Roadmap

- **Status:** Accepted

## Context

The initial version of the project must remain focused, but the architecture should leave room for substantial future growth. If the foundation is too narrow, adding observability, code generation, or policy engines later will require breaking changes.

## Decision

The long-term architecture will evolve toward a platform with optional modules around the stable core schema and ORM layers.

Likely future modules include:

- code generation for models and repositories
- query analyzer for explaining query plans and detecting inefficient access patterns
- repository generator for explicit, typed data access layers
- OpenTelemetry integration
- metrics export
- tracing integration
- access policy engine for richer scope and authorization rules

The core architecture will remain centered on:

- model-driven schema definitions
- deterministic migrations
- schema drift detection
- PostgreSQL-first dialect support
- explicit access control

## Consequences

- The system can grow into a broader data platform without redesigning its foundation.
- New operational and developer-experience features can be added as optional layers.
- Contributors can extend the project while preserving the architectural contract.

## Alternatives Considered

### Fix the scope permanently

Keep the project minimal and prohibit major future expansion.

### Add everything immediately

Design the initial version as if all future features must ship now.

### Leave the future undefined

Avoid making architectural commitments about long-term direction.

## Why Alternatives Were Rejected

### Fix the scope permanently

Rejected because it would limit the project's ability to become a durable platform.

### Add everything immediately

Rejected because it would create unnecessary complexity before the core behaviors are proven.

### Leave the future undefined

Rejected because architectural direction matters for stable decisions today.

