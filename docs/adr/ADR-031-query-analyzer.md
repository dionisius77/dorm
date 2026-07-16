# ADR-031: Query Analyzer

- **Status:** Accepted

## Context

Modern applications often suffer from inefficient queries that only become visible after they reach production.

The ORM already understands enough about the application to provide useful diagnostics:

- model metadata
- generated SQL
- schema metadata
- access policy predicates

That combination allows the ORM to help developers understand query quality before database performance issues become operational problems.

The Query Analyzer is intended to improve developer experience. It is not a replacement for PostgreSQL query planning tools.

## Decision

The ORM will include a built-in Query Analyzer that evaluates generated SQL together with schema metadata.

The analyzer will focus on actionable recommendations instead of raw execution plans.

The analyzer will be explicit and opt-in. It will only run when requested and will not affect normal query execution.

## Philosophy

The analyzer should explain:

- why a query may be slow
- what risk is present
- what improvement is likely to help

Diagnostics should be concise and readable. The goal is to guide developers toward fixes, not to overwhelm them with low-level planner output.

## Analysis Areas

The analyzer should detect common query quality issues, including:

- Sequential Scan
- Missing Index
- Missing WHERE
- Full Table Scan
- Inefficient ORDER BY
- Missing Composite Index
- Potential N+1
- Cartesian Join
- Large OFFSET usage

Future rules may be added without changing the public API.

## Access Policy Awareness

The analyzer must understand Access Policy behavior.

Automatically injected predicates such as:

- `company_id`
- `deleted_at`
- `tenant_id`

must participate in analysis so the resulting guidance reflects the actual query shape seen by PostgreSQL.

## Output

The analyzer should return human-readable diagnostics.

Example:

Sequential Scan detected.

Recommendation:

Create an index on:

```text
(company_id, deleted_at)
```

By default, the analyzer should avoid exposing raw PostgreSQL output.

## CLI

The CLI will expose an analysis command:

```text
dorm analyze
```

The command should analyze generated SQL and print readable recommendations.

## Observability

The analyzer must integrate naturally with ADR-021.

Analysis execution should be observable, and long-running analysis should generate traces.

## Error Handling

The analyzer must integrate with ADR-027.

Failures should produce actionable errors that explain what could not be analyzed and why.

## Performance

The analyzer must add no overhead to normal query execution.

It should run only when explicitly invoked.

## Future Compatibility

The design should support future capabilities without changing the public API, including:

- Explain integration
- Cost estimation
- Automatic index recommendations
- Query fingerprinting
- Slow query reports

## Consequences

- Developers gain an explicit feedback loop for query quality.
- The ORM can surface useful diagnostics before performance issues reach production.
- Access policy predicates remain part of analysis, so the guidance matches real runtime behavior.
- The feature stays opt-in and does not affect normal query latency.

## Alternatives Considered

### Relying only on PostgreSQL EXPLAIN

### Running analysis automatically on every query

### Exposing raw planner output directly

### Building query analysis only in the CLI

## Why Alternatives Were Rejected

### Relying only on PostgreSQL EXPLAIN

Rejected because the ORM already has enough schema and policy context to provide more targeted, higher-level recommendations.

### Running analysis automatically on every query

Rejected because analysis should not add runtime overhead to normal execution paths.

### Exposing raw planner output directly

Rejected because the goal is developer guidance, not planner verbosity.

### Building query analysis only in the CLI

Rejected because the analysis capability should be available as a reusable architectural feature, not just a command-line utility.
