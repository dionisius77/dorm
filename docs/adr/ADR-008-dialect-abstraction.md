# ADR-008: Dialect Abstraction

- **Status:** Accepted

## Context

The project must support PostgreSQL deeply while still leaving room for future SQL dialects. A dialect abstraction is necessary, but it must not flatten away PostgreSQL-specific power or force the core into generic SQL features only.

## Decision

SQL generation and database-specific behavior will be isolated behind a dialect layer.

The dialect abstraction is responsible for:

- quoting identifiers
- rendering SQL fragments
- mapping schema types to database types
- exposing feature and capability flags
- translating schema objects into DDL
- rendering expressions, predicates, defaults, and constraints

The PostgreSQL dialect will be the first and primary implementation. It will support PostgreSQL-native capabilities such as:

- UUID
- JSONB
- ARRAY
- ENUM
- generated columns
- materialized views
- partial indexes
- expression indexes
- GIN and GiST indexes
- identity columns
- composite keys and indexes

## Consequences

- Core packages can operate on abstract schema objects instead of SQL text.
- Database-specific behavior is localized and testable.
- Future dialects can be added without rewriting the ORM runtime.
- PostgreSQL capabilities can be modeled precisely rather than approximated.

## Alternatives Considered

### Hard-coded PostgreSQL SQL in core packages

Keep SQL rendering directly in `orm` and `migrate`.

### SQL string templates only

Use simple templates rather than a structured dialect interface.

### Very broad universal dialect abstraction

Design a lowest-common-denominator interface that hides database-specific behavior.

## Why Alternatives Were Rejected

### Hard-coded PostgreSQL SQL in core packages

Rejected because it would spread database-specific logic across the codebase and make future dialects expensive.

### SQL string templates only

Rejected because templates are difficult to normalize, validate, and extend for complex schema operations.

### Very broad universal dialect abstraction

Rejected because it would weaken PostgreSQL support and make advanced features difficult to express cleanly.

