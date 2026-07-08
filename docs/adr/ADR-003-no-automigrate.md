# ADR-003: No AutoMigrate

- **Status:** Accepted

## Context

Automatic schema mutation is convenient, but it is unsafe in production. It can create tables, alter columns, drop constraints, or change defaults without a reviewable artifact. That behavior is incompatible with deterministic migrations, schema drift visibility, and strong change control.

The project goal is production safety, not convenience.

## Decision

The ORM will not provide AutoMigrate or any feature that automatically mutates the schema in response to runtime model changes.

All schema changes must go through explicit migrations generated from model diffs and executed through controlled migration commands.

## Consequences

- Every schema change is reviewable before deployment.
- Migration history becomes a durable record of database evolution.
- Production surprises are reduced because runtime code cannot silently alter schema.
- Schema drift can be detected as an error instead of being "fixed" implicitly.
- Developers must be disciplined about changing models and generating migrations as separate steps.

## Alternatives Considered

### AutoMigrate

Automatically alter the database based on model changes.

### Hybrid mode

Allow automatic schema changes in development but require migrations in production.

### Interactive schema reconciliation

Prompt the user to approve individual schema changes at runtime.

## Why Alternatives Were Rejected

### AutoMigrate

Rejected because it hides risk, makes production behavior less predictable, and undermines the core promise of explicit schema management.

### Hybrid mode

Rejected because it introduces two behaviors for the same code path and encourages teams to rely on unsafe development shortcuts.

### Interactive schema reconciliation

Rejected because runtime prompts do not scale, do not fit automated deployments, and still create implicit change paths.

