# ADR-013: Extensibility

- **Status:** Accepted

## Context

The project should be able to grow without breaking compatibility or forcing redesign. Planned growth areas include hooks, custom types, plugins, and additional dialects.

Extensibility must be deliberate, not accidental. Unbounded extension points can destabilize core guarantees such as determinism and safety.

## Decision

Extensibility will be provided through narrowly defined extension points:

- plugins for optional integrations or tooling
- hooks for lifecycle behavior
- custom types for type mapping and serialization
- additional dialects through the dialect interface

The core schema model and migration rules will remain stable. Extensions must adapt to the schema contract rather than bypass it.

## Consequences

- New capabilities can be added without rewriting the core.
- The project can support ecosystem growth while preserving architectural consistency.
- Optional features can be isolated from the minimal safe core.
- Compatibility is easier to maintain because extension points are explicit.

## Alternatives Considered

### Unlimited plugin freedom

Allow extensions to mutate core behavior arbitrarily.

### No extension points

Keep the system closed and hard-coded.

### Runtime monkey patching style customization

Allow users to replace core behaviors dynamically.

## Why Alternatives Were Rejected

### Unlimited plugin freedom

Rejected because it would make determinism, safety, and supportability difficult to guarantee.

### No extension points

Rejected because it would make the project too rigid and limit adoption.

### Runtime monkey patching style customization

Rejected because it would create hard-to-debug behavior and undermine maintainability.

