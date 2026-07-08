# ADR-004: Models as the Source of Truth

- **Status:** Accepted

## Context

The ORM needs a stable definition of intended schema state. If the database is treated as the source of truth, schema generation becomes reactive, drift can be normalized accidentally, and model changes lose priority in the development workflow.

The project requires explicit schema ownership in code.

## Decision

Go models define the expected schema.

The model parser will extract fields, tags, constraints, indexes, relationships, defaults, and special behaviors into a normalized schema representation. That representation becomes the expected schema for both migration generation and drift detection.

Schema snapshots will be persisted as versioned artifacts. A snapshot represents the last known expected schema state and is used as the baseline for deterministic diff generation.

## Consequences

- The codebase has a single authoritative definition of intent.
- Schema changes are visible in version control.
- Migration generation can be deterministic because it compares two schema snapshots, not two live databases.
- Drift detection can distinguish between intended state and actual state.
- The project can support tooling like schema checks, migration previews, and docs generation from the same model metadata.

## Alternatives Considered

### Database as source of truth

Infer model expectations from the live PostgreSQL schema.

### Dual source of truth

Treat both models and database state as equally authoritative.

### Migration files as source of truth

Use historical migrations as the primary schema definition and infer model state from them.

## Why Alternatives Were Rejected

### Database as source of truth

Rejected because it makes the database authoritative over code, weakens deterministic generation, and complicates drift handling.

### Dual source of truth

Rejected because conflicting authorities create ambiguity whenever models and schema disagree.

### Migration files as source of truth

Rejected because migrations describe how schema evolved, not necessarily the current modeling intent, and they are harder to use as a live developer contract.

