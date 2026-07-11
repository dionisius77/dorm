# ADR-023: Access Policy & Security Boundaries

- **Status:** Accepted

## Context

Most production applications need default data isolation, not just ad hoc filtering in repositories. Common boundaries include:

- company
- tenant
- workspace
- organization
- warehouse

If every query must remember to add the right predicates manually, the result is repetitive code, inconsistent enforcement, and a real risk of accidental data leakage. Security-sensitive behavior should be centralized, predictable, and difficult to bypass accidentally.

The ORM already uses `context.Context` for request-scoped data. Access policy must build on that same model so the active user, scope, and policy choice are always available to the runtime without relying on hidden global state.

## Decision

The ORM will introduce a Policy Engine that evaluates access policy before SQL generation.

Policy evaluation will be context-driven and will occur as part of the query planning path, not as a post-processing step on generated SQL.

The default behavior will be secure by default:

- company isolation is enabled by default
- soft delete filtering is enabled by default
- audit field injection occurs automatically during write operations
- additional isolation scopes can be enforced through context-aware metadata

The active policy will be attached to the request context and consumed by the access layer before query generation.

## Policy Levels

The ORM will support four policy levels:

### Level 1: Default

Default policy enforcement is active.

This includes:

- company isolation
- tenant isolation when configured
- workspace isolation when configured
- organization isolation when configured
- soft delete filtering

### Level 2: Ignore Company

Company filtering is disabled.

All other active policies remain enabled.

### Level 3: Ignore RLS

All row-level isolation policies are disabled.

Soft delete filtering remains enabled.

### Level 4: System Mode

Every policy is disabled.

This mode is reserved for:

- migrations
- maintenance operations
- background jobs
- administrative tasks

## Public API

Policy changes will be explicit and semantic rather than query-specific.

Recommended usage:

```go
db.WithPolicy(access.Default())
db.WithPolicy(access.IgnoreCompany())
db.WithPolicy(access.IgnoreRLS())
db.WithPolicy(access.System())
```

Query modifiers that obscure policy intent are rejected as an architectural direction. The API should communicate that the policy boundary is being changed, not that a single query is being tweaked.

## Context Model

The request context remains the single source of truth for:

- user
- company
- tenant
- workspace
- policy selection

The access layer will read the active policy and scope values from context before SQL is rendered. Policy evaluation must not depend on mutable process-global state.

## Query Generation

Policy enforcement occurs before SQL generation:

1. Resolve context
2. Evaluate active policy
3. Inject required predicates and field values
4. Generate SQL
5. Execute SQL

Policies must not be applied as a rewrite after SQL generation. That would make behavior harder to reason about and could weaken guarantees around determinism and observability.

## Observability

Policy overrides must be observable.

The following policy states should be visible in tracing and logging:

- `policy.default`
- `policy.ignore_company`
- `policy.ignore_rls`
- `policy.system`

Policy changes should integrate naturally with the existing observability architecture so that privileged operations are discoverable during debugging and audit review.

## Extensibility

The Policy Engine must remain generic so that future policies can be added without rewriting ORM core behavior.

Examples of future policy dimensions:

- region
- department
- business unit
- project
- warehouse

New policy dimensions should fit the same context-driven evaluation model and should be translated into schema-aware predicates or write-time field injection where appropriate.

## Consequences

- Data isolation becomes the default rather than an opt-in convention.
- Query code remains simpler because repositories no longer need to repeat access predicates.
- Privileged code paths stay explicit, which reduces accidental bypass risk.
- Policy behavior becomes easier to test because it is centralized and deterministic.
- Additional isolation dimensions can be introduced without redesigning every repository.
- Observability can explain when and why a policy override occurred.

## Alternatives Considered

### Manual repository filtering

Require each repository to apply company, tenant, and soft-delete filtering by hand.

### Database-only enforcement

Use database triggers or PostgreSQL RLS as the only enforcement mechanism.

### Hidden policy bypasses

Allow implicit overrides based on runtime conditions or global state.

## Why Alternatives Were Rejected

### Manual repository filtering

Rejected because it is repetitive, easy to forget, and too easy to implement inconsistently across the codebase.

### Database-only enforcement

Rejected because the ORM still needs an application-level policy contract for portability, testing, and explicit privileged workflows.

### Hidden policy bypasses

Rejected because security boundaries must be obvious in code. Accidental bypasses are unacceptable in multi-tenant systems.

