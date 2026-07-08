# ADR-020: Security Model

- **Status:** Accepted

## Context

An ORM sits on the boundary between application code and the database. If the framework makes unsafe assumptions, developers can accidentally introduce SQL injection, privilege escalation, broken tenant isolation, or weak audit trails.

Security must favor correctness over convenience.

## Decision

The security model will be built around defensive defaults and explicit boundaries.

SQL injection prevention:

- Use parameterized queries for values
- Never concatenate untrusted input into SQL values
- Treat identifiers, literals, and expressions differently

Prepared statement strategy:

- Prefer prepared statements for repeated or reusable queries
- Ensure parameter binding is preserved through the query builder

Identifier escaping:

- Escape table names, column names, and aliases through the dialect layer
- Never treat user-provided identifiers as raw SQL

Migration safety:

- Require explicit migration generation and execution
- Avoid automatic schema mutation
- Make destructive changes visible and reviewable

Context validation:

- Validate access-scoped context inputs before use
- Refuse to apply ambiguous or incomplete scope data where correctness depends on it

Access control boundaries:

- Apply company, tenant, user, and audit rules centrally
- Do not allow ad hoc query paths to bypass access injection silently

Tenant isolation:

- Enforce tenant predicates through the access layer when the model requires them
- Make tenant scope an invariant, not a convention

Company isolation:

- Enforce company predicates the same way as tenant predicates when declared on the model

Safe defaults:

- Exclude soft-deleted rows unless explicitly requested
- Prefer least-privilege behavior for reads and writes

Privilege escalation prevention:

- Do not expose APIs that let callers casually disable access filters in normal application paths
- Require explicit opt-in for privileged or bypass behavior

Secure query generation:

- Generate SQL from schema-aware structures, not from string interpolation
- Keep dialect-specific escaping and literal formatting isolated

Audit integrity:

- Populate audit fields through controlled context-aware mechanisms
- Prevent callers from silently forging audit metadata when the framework owns those fields

Bypass prevention:

- The framework should make safe behavior the default path
- Unsafe behavior, if it exists at all, must be explicit, documented, and difficult to misuse
- Repositories should not need to manually reimplement security rules for every query

## Consequences

- The framework reduces common classes of database security mistakes.
- Access rules remain visible and consistent across the application.
- Developers can still write explicit code, but unsafe shortcuts are harder to take accidentally.
- Some operations will require additional configuration or explicit opt-in because convenience is not prioritized over safety.

## Alternatives Considered

### Trust developers to apply security manually

Rely on repository authors to remember every safety rule.

### Convenience-first defaults

Make the framework easy to bypass in the name of flexibility.

### Security only at the database layer

Assume database permissions or row-level security alone are sufficient.

## Why Alternatives Were Rejected

### Trust developers to apply security manually

Rejected because repetitive security logic is easy to forget and hard to audit.

### Convenience-first defaults

Rejected because it would increase the chance of accidental data leaks or privilege escalation.

### Security only at the database layer

Rejected because the framework itself must enforce safe query generation and access boundaries instead of assuming the database alone will catch every misuse.

