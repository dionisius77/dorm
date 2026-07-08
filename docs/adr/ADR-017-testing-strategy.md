# ADR-017: Testing Strategy

- **Status:** Accepted

## Context

This project has multiple high-risk areas:

- deterministic migration generation
- schema diffing
- schema drift detection
- SQL generation
- context-aware access control
- CLI behavior

These areas must be validated with more than unit tests alone. Because the framework is production-oriented, test coverage must be strongest where failures would be most costly.

## Decision

The project will use a layered testing strategy:

Unit tests:

- Validate small pieces of logic in isolation
- Cover schema normalization, diff rules, access policy decisions, dialect helpers, and error classification

Integration tests:

- Validate interactions between packages
- Exercise migration generation, schema parsing, drift detection, and query execution together

PostgreSQL integration testing:

- Use a real PostgreSQL instance for schema, migration, and drift workflows
- Validate catalog inspection, DDL generation, and runtime query behavior against actual PostgreSQL semantics

Docker-based testing:

- Use Docker to provide reproducible PostgreSQL test environments
- Prefer containerized test execution in CI for consistency

Migration testing:

- Verify generated SQL against known expected output
- Confirm migrations apply and revert when supported
- Check that generated output is deterministic

Schema drift testing:

- Confirm that expected and actual schema comparisons detect missing, extra, or mismatched objects
- Validate both normal and failure paths

CLI testing:

- Validate command behavior, exit codes, and output formatting
- Cover `init`, `migrate`, `schema check`, and `doctor`

Golden file testing:

- Use golden files for deterministic SQL and CLI output where stable text matters

Snapshot testing:

- Use snapshots for model-derived schema graphs and diff output when the structure is the important assertion

Performance regression testing:

- Track costs for query building, metadata access, and schema diff generation
- Fail builds if performance regresses beyond acceptable thresholds

Benchmarking:

- Maintain benchmarks for critical hot paths such as query construction and metadata resolution
- Use benchmarks for trend visibility rather than as sole correctness checks

Coverage expectations:

- Very high test coverage is required for the migration engine, diff engine, schema builder, and SQL generator
- These components should be treated as core correctness boundaries

CI expectations:

- Run unit tests on every change
- Run integration tests and PostgreSQL-backed tests in CI
- Run golden and snapshot comparisons in CI
- Run benchmarks and performance checks on a defined cadence or on performance-sensitive changes
- Fail CI on schema generation drift, invalid snapshots, or migration output changes that are not intentional

## Consequences

- The most critical parts of the framework are protected by multiple layers of validation.
- Determinism becomes enforceable rather than aspirational.
- CI is more expensive, but failures are caught earlier and more reliably.
- Contributors can trust that changes are exercised in real database conditions.

## Alternatives Considered

### Unit tests only

Rely on isolated tests for all behavior.

### No dedicated PostgreSQL integration testing

Mock the database or use only in-memory substitutes.

### Ad hoc manual verification

Test migrations and schema behavior manually when needed.

## Why Alternatives Were Rejected

### Unit tests only

Rejected because the framework depends on real PostgreSQL semantics that mocks cannot fully capture.

### No dedicated PostgreSQL integration testing

Rejected because dialect-specific behavior and schema drift logic must be validated against a real database.

### Ad hoc manual verification

Rejected because it does not scale, is not repeatable, and is too risky for production infrastructure.

