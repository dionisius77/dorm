# ADR-032: Execution Inspection & Dry Run

- **Status:** Accepted

## Context

Developers frequently need to understand how the ORM is processing an operation before it reaches the database.

Generated SQL is important, but it is only the final output of a larger execution pipeline that may include:

- query builder expansion
- access policy injection
- automatic audit fields
- lifecycle hooks
- query advisor findings
- execution orchestration

Inspecting SQL alone is not sufficient because it does not explain why the ORM produced that SQL or what other framework behaviors were applied before execution.

The project needs an inspection mode that shows the complete ORM execution pipeline without sending SQL to the database.

## Decision

The ORM will introduce an Execution Inspection architecture.

Dry Run will become an execution mode that runs the complete ORM pipeline and replaces only the final SQL execution step with an inspection report.

In Dry Run mode:

- the ORM still performs query construction
- access policies are still applied
- audit actions are still resolved
- lifecycle hooks are still executed
- query advisor findings are still produced
- SQL is not sent to the database
- no transaction is committed
- no data is modified

The execution engine will remain the single source of truth for all execution modes.

## Philosophy

Dry Run should answer:

> What would the ORM do?

It should not be reduced to:

> What SQL would be generated?

The goal is to make automatic framework behavior visible so developers can reason about the full operation, not just its final statement text.

## Public API

Dry Run should be exposed through an idiomatic builder-style API that composes naturally with existing ORM calls.

Example:

```go
report, err := db.
    DryRun().
    Find(ctx, &users)
```

The API should avoid boolean flags and should express execution mode directly.

Dry Run must remain compatible with the normal ORM API surface so that callers can opt into inspection without rewriting their data access logic.

## Execution Pipeline

Dry Run must reuse the same internal pipeline used for normal execution.

Example flow:

```text
Access Policy

->

Audit

->

Lifecycle Hooks

->

Query Builder

->

Query Advisor

->

Execution (Skipped)
```

The only behavioral difference from normal execution is that the final database round trip is skipped.

No separate query-building path should be introduced for Dry Run.

## Execution Report

Dry Run must return a structured execution report.

The report should include:

- generated SQL
- query parameters
- applied access policies
- audit actions
- executed lifecycle hooks
- query advisor findings
- execution status

Execution status must clearly indicate that execution was skipped.

Future versions may add more execution metadata without changing the core contract.

## Access Policy

Access Policy is a first-class part of the inspection report.

The report must clearly display:

- injected predicates
- inherited policies
- policy overrides

This makes automatic security behavior visible and auditable during development.

## Audit

Dry Run should display automatically generated audit actions.

Examples include:

- `created_by`
- `updated_by`
- `deleted_by`
- timestamps

The inspection report should show these actions as applied framework behavior, not as hidden side effects.

## Lifecycle Hooks

Dry Run should display executed lifecycle hooks in deterministic order.

The report should show which hooks ran and in what sequence.

Future versions may include hook duration or additional lifecycle metadata.

## Query Advisor

Dry Run must integrate with ADR-031.

Query advisor findings become part of the execution report.

Example findings include:

- Missing Index
- Sequential Scan Risk
- Large OFFSET
- Missing Composite Index

The report should present concise recommendations alongside the execution details that produced them.

## CLI

The CLI should provide an inspection mode.

Example:

```text
dorm dry-run
```

The CLI should render a human-readable execution report.

Recommended layout:

```text
Access Policy

[OK] company_id injected

[OK] deleted_at injected

Generated SQL

SELECT ...

Parameters

$1 = ...

Query Advisor

[WARN] Missing composite index

(company_id, deleted_at)

Execution

Skipped
```

The CLI output should be concise enough for terminal use while still exposing the complete execution story.

## Observability

Dry Run must integrate with ADR-021.

Inspection should generate traces, and inspection spans must be distinguishable from normal database execution spans.

This allows teams to observe inspection usage without conflating it with real database activity.

## Error Handling

Dry Run must integrate with ADR-027.

Inspection must preserve wrapped errors and must not hide underlying generation failures.

If any step in the shared execution pipeline fails, Dry Run should return the original cause with appropriate execution context.

## Performance

Dry Run should reuse the existing execution pipeline.

The architecture must avoid duplicate query-generation logic.

The execution engine should remain the single source of truth for:

- policy injection
- audit resolution
- hook execution
- query construction
- advisor integration

Only the final SQL execution step should differ between normal execution and Dry Run.

## Future Compatibility

The execution inspection architecture should support future execution modes without breaking the public API.

Examples include:

- Explain
- Analyze
- Benchmark
- SQL Export
- IDE Integration

These modes should reuse the same shared execution pipeline and differ only in the final terminal step or output formatter.

## Repository Structure

An internal execution package is recommended to keep shared orchestration isolated and reusable.

Example layout:

```text
execution/
|-- pipeline/
|-- executor/
|   |-- normal/
|   `-- dryrun/
`-- report/
```

The pipeline should be shared across all execution modes.

The executor layer should select the terminal behavior for the active mode.

The report package should define the structured inspection output consumed by CLI and API callers.

## Consequences

- Developers can inspect the full ORM behavior without executing SQL.
- Access policy, audit, hooks, and advisor output become visible in a single report.
- Dry Run becomes a reusable architectural mode rather than a special-case SQL formatter.
- The ORM keeps one execution pipeline, reducing drift between normal execution and inspection.
- Future execution modes can build on the same architecture instead of introducing parallel implementations.

## Alternatives Considered

### Generate SQL only

### Execute a separate inspection pipeline

### Add a boolean `DryRun` flag to configuration

### Skip all framework processing during inspection

## Why Alternatives Were Rejected

### Generate SQL only

Rejected because SQL alone does not explain access policy injection, audit behavior, lifecycle hooks, or advisor output.

### Execute a separate inspection pipeline

Rejected because a second pipeline would inevitably drift from normal execution and produce reports that do not match real behavior.

### Add a boolean `DryRun` flag to configuration

Rejected because boolean configuration does not express execution mode clearly and does not compose cleanly with the existing ORM API.

### Skip all framework processing during inspection

Rejected because the purpose of Dry Run is to show what the ORM would do, which requires running the same internal processing steps up to the final execution boundary.
