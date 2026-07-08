# ADR-009: Reflection Strategy

- **Status:** Accepted

## Context

Reflection is useful in Go for runtime inspection, but it comes with trade-offs:

- slower execution
- more runtime failure modes
- less predictability
- harder-to-test behavior

This project needs enough introspection to support runtime CRUD and access control, but it should avoid relying on reflection as the primary source of schema truth when compile-time parsing can do the job better.

## Decision

Reflection will be used sparingly and intentionally.

Allowed uses:

- runtime scanning of destination structs
- inferring field addresses for query result mapping
- inspecting model values when necessary for inserts or updates
- fallback behavior when generated metadata is unavailable

Avoided uses:

- primary schema definition
- migration generation
- deterministic schema snapshot construction
- complex relationship inference when AST parsing can provide the same information earlier

Whenever possible, compile-time parsing of Go source will be preferred for schema and migration concerns.

## Consequences

- Schema generation becomes more deterministic.
- Runtime performance improves because metadata can be cached or precomputed.
- The ORM can still operate ergonomically with Go structs.
- Contributors must understand which parts of the system are runtime-oriented and which are source-oriented.

## Alternatives Considered

### Reflection everywhere

Use runtime reflection for schema parsing, migrations, and query mapping.

### No reflection at all

Require generated code for every interaction.

### Mixed approach without rules

Use reflection and AST parsing interchangeably depending on convenience.

## Why Alternatives Were Rejected

### Reflection everywhere

Rejected because it increases runtime cost and makes deterministic schema tooling harder.

### No reflection at all

Rejected because it would make the developer experience too rigid for the initial architecture.

### Mixed approach without rules

Rejected because it would blur the line between source truth and runtime convenience, producing inconsistent behavior.

