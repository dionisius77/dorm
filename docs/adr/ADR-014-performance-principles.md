# ADR-014: Performance Principles

- **Status:** Accepted

## Context

The ORM must be production-ready, which means it cannot treat performance as an afterthought. At the same time, premature optimization should not compromise clarity or determinism.

The key performance risks are repetitive reflection, repeated metadata parsing, unnecessary SQL reconstruction, and avoidable database round trips.

## Decision

Performance will be improved through the following principles:

- cache parsed model metadata
- cache schema snapshots where appropriate
- cache reflection-derived information
- support prepared statements for repeated queries
- keep query building lightweight and deterministic
- avoid rebuilding immutable metadata on every request

Reflection and AST parsing should be minimized in the hot path. Schema parsing and generation should occur at build-time or command-time whenever possible, not during request handling.

## Consequences

- Query execution can remain efficient even with rich ORM features.
- Startup costs are higher where appropriate, but request-path costs are reduced.
- The architecture remains predictable because expensive work is moved out of the hot path.
- Caching policies can be tuned independently for metadata, statements, and schema inspection.

## Alternatives Considered

### No caching

Compute all metadata and SQL structures on demand.

### Aggressive runtime optimization everywhere

Introduce complex caches and compiled query plans for every path from the start.

### Reflection-centric hot path

Use runtime reflection for all CRUD and schema logic, accepting the cost.

## Why Alternatives Were Rejected

### No caching

Rejected because it would waste work repeatedly and impose avoidable latency.

### Aggressive runtime optimization everywhere

Rejected because it would complicate the architecture before there is evidence of need.

### Reflection-centric hot path

Rejected because it is less predictable and undermines the project's minimal-reflection goal.

