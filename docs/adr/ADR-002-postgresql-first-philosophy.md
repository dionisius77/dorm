# ADR-002: PostgreSQL-first Philosophy

- **Status:** Accepted

## Context

The ORM is intended to support advanced schema features such as UUIDs, JSONB, ARRAY types, partial indexes, expression indexes, generated columns, identity columns, composite keys, and strong schema introspection. These features are not equally available or equally expressive across all relational databases.

Trying to design for the lowest common denominator would weaken the product and force the project to ignore the database capabilities that make PostgreSQL valuable in production systems.

## Decision

PostgreSQL will be the primary and fully supported target.

Other databases may be added later through dialects, but they are not equal citizens in the initial architecture. The core schema model, migration system, and drift detection logic will be designed around PostgreSQL semantics first.

## Consequences

- The project can use PostgreSQL features naturally instead of hiding them behind an overly generic abstraction.
- Migration and drift logic can be precise because the expected behavior is anchored to one well-defined database.
- The initial implementation can be simpler, safer, and more testable.
- Additional dialects will need to conform to the project's schema model rather than shaping it from the beginning.

## Alternatives Considered

### Multi-dialect from day one

Support PostgreSQL, MySQL, and SQL Server equally from the start.

### Lowest-common-denominator abstraction

Design around only the SQL features common to all major databases.

### ORM as database-neutral API with backend-specific quirks hidden

Present one uniform API and map everything to each database at runtime.

## Why Alternatives Were Rejected

### Multi-dialect from day one

Rejected because it multiplies design complexity, increases test matrix size, and forces compromises in schema representation before PostgreSQL behavior is proven.

### Lowest-common-denominator abstraction

Rejected because it would discard many of the features that make PostgreSQL compelling and would produce a weaker ORM.

### ORM as database-neutral API with backend-specific quirks hidden

Rejected because it encourages leaky abstractions and makes advanced PostgreSQL features awkward or impossible to express.

