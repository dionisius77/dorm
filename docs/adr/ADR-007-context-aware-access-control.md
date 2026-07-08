# ADR-007: Context-Aware Access Control

- **Status:** Accepted

## Context

Many production systems need implicit access rules:

- company scoping
- tenant scoping
- user ownership
- audit fields
- soft delete

If these concerns are written manually in every repository, the code becomes repetitive, inconsistent, and easy to bypass. The risk is not just developer inconvenience; it is security and data isolation.

## Decision

Access-related behavior will be driven through request Context and model metadata instead of being manually implemented in each repository.

The access layer will inspect model tags and context values to determine:

- which scope predicates must be injected into queries
- which fields must be filled on insert
- which audit fields must be filled on update or delete
- whether soft-deleted rows should be included or excluded

## Consequences

- Access behavior is centralized and consistent.
- Repositories remain focused on domain access patterns rather than policy boilerplate.
- Query execution becomes safer because scope conditions are enforced by the ORM layer.
- Audit fields and soft-delete behavior are applied uniformly across the application.
- The system can support future concepts like organization, workspace, or warehouse scope without redesigning every repository.

## Alternatives Considered

### Manual repository implementation

Require every repository to inject access logic by hand.

### Global process state

Store scope values in package-level state instead of request Context.

### Database triggers or RLS only

Push access concerns entirely into PostgreSQL features.

## Why Alternatives Were Rejected

### Manual repository implementation

Rejected because it is repetitive, error-prone, and easy to forget in edge cases.

### Global process state

Rejected because it is unsafe in concurrent systems and breaks request isolation.

### Database triggers or RLS only

Rejected because access logic still needs to be visible, testable, and consistent in application code. Database features may complement the ORM, but they should not be the only enforcement layer in this architecture.

