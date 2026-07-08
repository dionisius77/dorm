# ADR-011: Schema Builder

- **Status:** Accepted

## Context

The project needs a canonical, structured representation of schema that is independent of raw SQL text. This representation must support:

- migration generation
- drift detection
- schema inspection
- validation
- future tooling such as documentation or code generation

If each subsystem invents its own schema interpretation, the system will drift conceptually even when the database does not.

## Decision

The project will define an internal schema model as the shared contract for all major subsystems.

This schema representation will describe:

- tables
- columns
- data types
- nullability
- defaults
- primary keys
- unique constraints
- foreign keys
- indexes
- checks
- enums
- generated columns
- views and materialized views where supported
- model metadata for access control and soft delete behavior

All major subsystems will operate on this schema representation rather than on SQL text:

- `migrate` builds expected schema from models
- `schema` builds actual schema from the database
- `dialect` renders SQL from schema objects
- `orm` uses schema metadata for query planning and access behavior

## Consequences

- SQL is treated as a rendering target, not as the internal truth.
- The same structure can drive multiple workflows without duplication.
- Schema diffs become reliable because they compare objects, not text.
- The architecture can support future tooling without redesigning the core data model.

## Alternatives Considered

### SQL as the internal representation

Use SQL strings as the source for both migration and drift logic.

### Separate schema models per subsystem

Allow `migrate`, `schema`, and `orm` to each maintain their own structural representations.

### Minimal metadata only

Track only enough information for CRUD and infer everything else on demand.

## Why Alternatives Were Rejected

### SQL as the internal representation

Rejected because SQL text is harder to normalize, compare, and evolve than structured schema objects.

### Separate schema models per subsystem

Rejected because it creates duplication and inconsistencies between packages.

### Minimal metadata only

Rejected because it would be insufficient for migrations, drift detection, and PostgreSQL-specific features.

