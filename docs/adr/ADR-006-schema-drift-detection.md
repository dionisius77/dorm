# ADR-006: Schema Drift Detection

- **Status:** Accepted

## Context

Even with explicit migrations, databases can drift because of manual changes, failed deployments, ad hoc fixes, or out-of-band administration. The ORM needs a way to detect when actual PostgreSQL state no longer matches the expected schema.

This check is especially important in production systems where silent drift can become a long-term reliability or security issue.

## Decision

Schema drift detection will run during startup or application initialization, and it will compare:

- Expected schema from models and snapshots
- Actual schema from PostgreSQL catalog inspection

Behavior will depend on environment and configuration:

- Development mode: fail fast, typically by panicking or returning a fatal error
- Production mode: configurable severity, but drift must still be explicit and observable

## Consequences

- Drift is discovered early rather than after query failures or data corruption.
- Developers get immediate feedback when models and database state diverge.
- Production operators can decide whether to fail closed, warn, or gate startup based on policy.
- The system provides a consistent diagnostic path for schema health.

## Alternatives Considered

### No drift detection

Rely entirely on migrations and assume the database is correct.

### Periodic background drift checks only

Run drift detection on a schedule instead of at startup.

### Automatic drift repair

Attempt to reconcile the database automatically when drift is detected.

## Why Alternatives Were Rejected

### No drift detection

Rejected because silent drift is one of the most dangerous failure modes in production data systems.

### Periodic background drift checks only

Rejected because problems should be known before the application starts serving traffic.

### Automatic drift repair

Rejected because repair is equivalent to automated schema mutation and violates the no-AutoMigrate principle.

