# ADR-035: Query Options & Query Builder

- **Status:** Accepted

## Context

Applications need a consistent way to compose queries without forcing every read path into a bespoke builder or a complex fluent DSL.

The ORM must support common query modifiers such as:

- Select
- Where
- Join
- Group By
- Having
- Order By
- Limit
- Offset
- Distinct

These modifiers should be available across the ORM in a single, extensible mechanism. Query composition must remain declarative, predictable, and idiomatic to Go.

The architecture must also remain compatible with access policy injection, observability, dry run inspection, and future query-related features such as relationship loading and query analysis.

## Decision

The ORM will adopt a Query Option architecture.

Query modifiers are represented as lightweight QueryOptions that are accepted by query execution methods. Applications compose queries by passing zero or more QueryOptions into execution calls.

Example:

```go
err := db.WithContext(ctx).Find(
    &users,
    orm.Select("u.id, u.name"),
    orm.LeftJoin("roles r", "r.id = u.role_id"),
    orm.Where("u.status = ?", status),
    orm.OrderBy("u.created_at DESC"),
    orm.Limit(20),
)
```

The ORM execution layer remains responsible for translating the accumulated query state into SQL and for executing that SQL.

Query modifiers do not execute SQL themselves. They only describe query intent.

## Philosophy

Query composition should stay explicit.

The ORM should avoid:

- complex fluent pipelines
- hidden query mutation
- query-builder frameworks with large stateful APIs
- multiple parallel composition mechanisms

The preferred model is:

- declarative query modifiers
- a shared composition path
- terminal execution methods that remain easy to reason about

## Query Option Model

The ORM will define a common QueryOption abstraction inside the implementation.

Applications do not depend on the interface directly. They use helper functions such as:

- `Where(...)`
- `Select(...)`
- `Limit(...)`
- `Offset(...)`
- `OrderBy(...)`
- `GroupBy(...)`
- `Having(...)`
- `Distinct(...)`
- `LeftJoin(...)`
- `RightJoin(...)`
- `InnerJoin(...)`
- `CrossJoin(...)`

The implementation may use internal interfaces or unexported state types to collect and normalize these options, but the public API remains helper-based and idiomatic.

QueryOptions should be lightweight and should avoid unnecessary allocations or reflection during composition.

## Query Execution

Every read-oriented execution method should accept QueryOptions.

Examples include:

- `Find`
- `FindOne`
- `Count`
- `Exists`

Future read operations should reuse the same option mechanism rather than introducing separate composition APIs.

Execution methods are responsible for:

- collecting options
- applying policy-injected predicates
- ordering SQL clauses correctly
- rendering SQL through the active dialect
- executing the query

## Ordering

Callers may pass QueryOptions in any order.

The ORM determines clause ordering internally so that equivalent option sets produce equivalent SQL.

For example, these two calls must behave the same:

```go
orm.Limit(10),
orm.Where("status = ?", status),
orm.OrderBy("created_at DESC"),
```

and:

```go
orm.Where("status = ?", status),
orm.OrderBy("created_at DESC"),
orm.Limit(10),
```

This means the query builder normalizes state before rendering, rather than depending on argument order.

## Joins

JOINs are part of query composition and must be represented as QueryOptions.

Example:

```go
orm.LeftJoin("roles r", "r.id = users.role_id")
```

The ORM must not expose separate raw JOIN execution paths that bypass query composition.

JOIN clauses remain declarative input to the shared query builder.

## Access Policy

Query Options must integrate with ADR-023.

Access policy remains an independent concern and must be injected into the query state without requiring callers to encode policy predicates manually.

Automatically injected predicates should compose correctly with user-defined `WHERE` clauses and must not depend on the order in which QueryOptions were passed.

## Observability

Query Options must integrate with ADR-021.

The composed query state should contribute useful metadata to tracing and logging, including the final operation type and relevant query characteristics when available.

Observability must remain a cross-cutting concern of execution, not of individual QueryOptions.

## Error Handling

Query Options must integrate with ADR-027.

Invalid or incompatible QueryOptions should produce typed, actionable errors.

Examples include:

- invalid clause combinations
- unsupported option values
- empty required expressions
- inconsistent query state

Error reporting should help developers identify the bad option without exposing internal implementation details unnecessarily.

## Performance

Query Options should remain lightweight.

The architecture should avoid:

- reflection during ordinary option collection
- repeated cloning of query state
- heavy builder abstractions
- extra allocations that do not improve clarity

The shared composition path should stay fast enough for ordinary ORM use while remaining easy to extend.

## Extensibility

The Query Option architecture must support reusable options from applications and future plugins.

This allows higher-level features to build on the same query API without changing terminal execution methods.

Examples of future extensions include:

- reusable query presets
- tenant-aware scopes
- relationship-aware joins
- query advisor annotations
- dry-run tags

Extensions should compose with the existing builder instead of replacing it.

## Future Compatibility

The Query Option architecture should remain compatible with future ORM features, including:

- Relationship Engine
- Query Advisor
- Dry Run
- Plugin System

Those features may contribute additional metadata or options, but they should not require a different query execution model.

## Consequences

- Applications get one consistent way to describe query intent.
- Query composition stays declarative and readable.
- The ORM can normalize SQL clause ordering internally.
- Access policy, observability, and future extensions can share the same query state.
- The public API stays compact and idiomatic.

## Alternatives Considered

### Fluent chain-only builder

### String-based query assembly

### Separate builder types for each clause family

### Implicit query mutation through session state

## Why Alternatives Were Rejected

### Fluent chain-only builder

Rejected because long fluent pipelines become harder to compose, harder to reuse, and easier to obscure than explicit options.

### String-based query assembly

Rejected because it shifts SQL correctness and clause ordering onto callers and makes policy injection, observability, and validation much harder.

### Separate builder types for each clause family

Rejected because it fragments the API surface and makes the ORM harder to learn and extend.

### Implicit query mutation through session state

Rejected because hidden mutation makes query behavior harder to reason about and increases the risk of surprising interactions between helpers, policy injection, and terminal operations.
