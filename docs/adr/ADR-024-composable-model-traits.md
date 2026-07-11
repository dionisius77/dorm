# ADR-024: Composable Model Traits

- **Status:** Accepted

## Context

The ORM needs to support tables with different capabilities without forcing every model into the same shape. Some tables require audit fields, some require company isolation, some require optimistic locking, and some should remain completely raw.

Historically, ORMs often solve this with a mandatory `BaseModel` or a shared inheritance-style parent. That approach reduces boilerplate, but it also introduces fields that some tables do not need, makes schema evolution less explicit, and pushes unrelated behavior into every model.

This project follows Go's composition-first style. Model design should feel idiomatic to Go developers and should use embedding to express reusable capabilities rather than inheritance-like coupling.

## Decision

The ORM will adopt a composable trait architecture.

Models will be assembled by embedding reusable traits. No mandatory `BaseModel` will exist.

The core trait set will include:

- `Company`, containing `CompanyID`
- `Audit`, containing `CreatedAt`, `CreatedBy`, `UpdatedAt`, `UpdatedBy`, `DeletedAt`, and `DeletedBy`
- `Entity`, combining `Company` and `Audit` for common managed models

Models will only include the traits they actually need. A raw model may declare only explicit fields and receive no automatic audit behavior.

The ORM will inspect embedded traits during schema building and runtime behavior selection. Trait presence will drive behavior, not hardcoded field-name assumptions.

## Trait Semantics

### Company

The `Company` trait represents company-scoped data. When present, the ORM may apply company isolation policies and company-aware field handling.

### Audit

The `Audit` trait represents lifecycle metadata. When present, the ORM will automatically populate audit fields during write operations where appropriate.

If `Audit` is absent, no audit logic will run for that model.

### Entity

The `Entity` trait is the convenience option for common managed entities. It combines `Company` and `Audit` so that most application models can opt into a complete managed baseline without duplicating fields.

## Schema Builder

Embedded traits will be flattened into the generated schema.

From the schema builder's perspective, trait fields must behave exactly as if they were declared directly on the model. Migration generation and schema drift detection must therefore understand embedded traits as first-class schema contributors.

## Reflection Strategy

Trait discovery will be cached to avoid repeated runtime reflection.

The implementation should prefer compile-time parsing or precomputed metadata where possible, while still preserving correct runtime behavior for dynamic usage and tests.

The architecture should avoid paying reflection costs on every query or write.

## Extensibility

Third-party packages may define their own traits.

Examples include:

- `GeoLocation`
- `Ownership`
- `Archive`
- `Versioning`

Custom traits should integrate naturally with the schema builder, migration engine, and access engine without requiring changes to ORM core packages.

## Naming Philosophy

The ORM will not require inheritance-style base models.

Composition is the default architectural model. Go embedding is the mechanism used to compose capabilities into a model while keeping the result explicit, readable, and easy to reason about.

## Consequences

- Models stay small and only include the capabilities they need.
- Common fields such as audit metadata can be reused without a mandatory base type.
- Schema generation becomes trait-aware and can flatten embedded behavior consistently.
- Access control and write-time automation can key off capabilities instead of brittle field-name conventions.
- Third-party packages can extend model behavior through traits without modifying ORM core.
- Some behavior will require metadata discovery, which makes caching and reflection strategy important for performance.

## Alternatives Considered

### Mandatory BaseModel

Use a shared base type that every model must embed or inherit conceptually.

### Manual field duplication

Require each model to declare all fields directly without reusable traits.

### Field-name conventions only

Infer behavior solely from specific field names without recognizing traits.

### Runtime reflection only

Discover traits on every operation using reflection without caching or precomputation.

## Why Alternatives Were Rejected

### Mandatory BaseModel

Rejected because it forces unnecessary fields and behavior onto models that do not need them. It also makes the model layer less idiomatic for Go, where composition is preferred over inheritance-like structures.

### Manual field duplication

Rejected because it increases boilerplate, duplicates schema knowledge, and makes consistency harder to maintain across the codebase.

### Field-name conventions only

Rejected because hardcoded names are brittle and do not express architectural intent. Trait-driven behavior is clearer and easier to extend.

### Runtime reflection only

Rejected because repeated reflection would add avoidable overhead and make runtime behavior harder to optimize. Caching and precomputation provide a better long-term foundation.
