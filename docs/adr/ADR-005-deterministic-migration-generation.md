# ADR-005: Deterministic Migration Generation

- **Status:** Accepted

## Context

Migration files must be stable, reviewable, and reproducible. Non-deterministic generation creates noisy diffs, undermines trust in the tool, and makes code review harder.

The project needs a generation process that is independent of live database state.

## Decision

Migrations will be generated from:

Previous Snapshot

to

Current Models

The generator will not compare current models to a live database when producing migration SQL.

The generation pipeline will:

1. Parse current model definitions.
2. Build the current schema representation.
3. Load the previous schema snapshot.
4. Compute a structured diff.
5. Render deterministic SQL from the diff.
6. Persist the new snapshot after successful generation.

## Consequences

- Generated migrations are repeatable on any machine with the same inputs.
- Reviewers can reason about schema changes without depending on database state.
- The output is stable across runs because the diff is based on normalized schema objects.
- The system can detect accidental model changes even when the database has drift.

## Alternatives Considered

### Compare current models against live database

Generate SQL by inspecting the current database and model set.

### Compare migration files against live database

Infer changes from the migration history and reconcile the database state directly.

### Regenerate full schema every time

Drop and recreate the expected schema output from scratch on each generation.

## Why Alternatives Were Rejected

### Compare current models against live database

Rejected because live databases can differ by environment, introduce non-determinism, and hide intentional versus accidental differences.

### Compare migration files against live database

Rejected because migration history is not the same as current intended schema and can be expensive to reconcile.

### Regenerate full schema every time

Rejected because it would produce excessive churn and make diffs less useful.

