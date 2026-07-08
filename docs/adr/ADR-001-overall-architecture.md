# ADR-001: Overall Architecture

- **Status:** Accepted

## Context

The project is a PostgreSQL-first ORM for Go with two broad responsibilities:

1. Runtime data access, query execution, and context-aware behavior.
2. Schema lifecycle management, including deterministic migrations and drift detection.

These responsibilities are related, but they are not the same concern. A single large package would blur boundaries, make testing harder, and encourage accidental coupling between query execution and schema management.

The project also needs room for future dialects, access policies, and optional tooling without turning the core into a monolith.

## Decision

The codebase will be divided into modular packages with clear ownership:

- `orm` for runtime CRUD, query building, transactions, hooks, and relation loading.
- `migrate` for model parsing, schema comparison, migration generation, and migration execution.
- `schema` for inspecting the database, reconstructing actual schema state, and detecting drift.
- `access` for context-aware ownership, tenant, company, user, audit, and soft-delete rules.
- `dialect` for SQL rendering, type mapping, and database-specific capabilities.

Each package will expose a narrow API and depend on a shared schema representation rather than on SQL text or package internals.

## Consequences

- The system is easier to understand because each package has a single primary responsibility.
- Schema tooling can evolve independently from runtime query execution.
- Testing is simpler because schema generation, drift detection, access control, and query building can be validated in isolation.
- Future support for additional SQL dialects can be added without rewriting the runtime ORM.
- The project avoids the "god package" problem and reduces the risk of hidden coupling.

## Alternatives Considered

### Single monolithic package

Keep all ORM, migration, schema, access, and dialect logic in one package.

### Two-package split

Separate runtime ORM from migrations, but keep schema, access, and dialect concerns embedded within them.

### Fully layered monolith with internal subpackages only

Use internal folders but present a single public package.

## Why Alternatives Were Rejected

### Single monolithic package

Rejected because it makes the system harder to reason about and increases the risk that small runtime changes will destabilize migration and drift logic. It also encourages a large, implicit API surface.

### Two-package split

Rejected because it still mixes multiple concepts together and does not provide enough isolation for deterministic schema processing, database inspection, and access policy enforcement.

### Fully layered monolith with internal subpackages only

Rejected because it still presents a conceptual monolith to users and makes it harder for contributors to maintain clear boundaries between concerns.

