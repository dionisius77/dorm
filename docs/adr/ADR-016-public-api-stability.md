# ADR-016: Public API Stability

- **Status:** Accepted

## Context

This project is intended to become a long-term open source framework. Users will build production systems on top of it, so they need clear expectations about how the public API evolves over time.

Without a stability policy, users cannot confidently upgrade, and maintainers cannot judge the impact of changes consistently.

## Decision

The project will follow Semantic Versioning for all public releases.

Public API policy:

- `major` version increments indicate breaking changes
- `minor` version increments may add backward-compatible functionality
- `patch` version increments may fix bugs without changing public contracts

Breaking changes include:

- removal of exported functions, types, methods, or constants
- signature changes on exported symbols
- semantic changes that invalidate existing behavior
- renamed public packages or symbols
- changed default behavior that is not backward compatible
- removal or change of supported configuration fields
- changes that require users to rewrite migrations or queries

Public vs internal boundaries:

- Packages intended for stable consumption will be documented as public API
- Internal packages remain implementation details and may change without notice
- Internal code must not be depended on by consumers

Deprecation policy:

- Deprecated APIs must be marked clearly in documentation and code comments
- Deprecation should include a replacement path when possible
- Deprecated APIs should remain available for at least one minor release cycle, and longer when practical
- Removal of deprecated APIs requires a major version bump if they are part of the public API

Backward compatibility strategy:

- Preserve behavior for existing public APIs unless there is a strong reason to break compatibility
- Add new APIs instead of repurposing existing ones when possible
- Introduce adapters or shims where needed to ease transitions

API lifecycle:

- Experimental
- Stable
- Deprecated
- Removed

Experimental APIs:

- May exist under clearly labeled experimental packages or symbols
- May change without notice
- Must not be the default path for production-safe core behavior unless explicitly documented

Release policy:

- Public releases must include release notes describing API additions, deprecations, and breaking changes
- Breaking changes require a major release and migration guidance
- Minor releases should remain backward compatible

## Consequences

- Users can depend on the framework with clearer expectations.
- Maintainers have a consistent standard for reviewing changes.
- Experimental work can proceed without polluting the stable API.
- Versioning becomes a communication tool, not just a tag.

## Alternatives Considered

### No explicit stability policy

Allow the API to evolve informally.

### Strict freeze

Lock the API permanently after the first release.

### All APIs experimental forever

Avoid committing to stability guarantees.

## Why Alternatives Were Rejected

### No explicit stability policy

Rejected because production users need predictable upgrade behavior.

### Strict freeze

Rejected because the project must evolve to remain useful.

### All APIs experimental forever

Rejected because it would prevent the project from becoming a trusted framework.

