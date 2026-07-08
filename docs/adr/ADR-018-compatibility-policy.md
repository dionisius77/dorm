# ADR-018: Compatibility Policy

- **Status:** Accepted

## Context

Open source frameworks live or die by compatibility guarantees. Users need to know which Go versions, PostgreSQL versions, and operating systems are supported, and what happens when older versions are no longer maintained.

The project must also define how future dialect support fits into the compatibility contract.

## Decision

Minimum supported Go version:

- The project will support a clearly documented minimum Go version and update it intentionally as the project evolves
- The minimum version must be stated in release documentation and enforced in CI

Supported PostgreSQL versions:

- The project will support a documented range of PostgreSQL versions
- The supported range must be tested against real PostgreSQL instances in CI
- New PostgreSQL features may require version-specific capability checks

Supported operating systems:

- Linux, macOS, and Windows should be considered platform targets for the Go package and CLI unless a specific feature depends on platform behavior

ARM64 support:

- ARM64 is a first-class target and must be treated as supported on any platform where the Go toolchain and PostgreSQL environment permit it

Windows support:

- Windows support should be maintained for the Go library and CLI where practical
- If a subsystem is not fully supported on Windows, the limitation must be documented clearly

Future dialect support policy:

- Additional dialects may be added only if they conform to the core schema contract
- PostgreSQL remains the baseline dialect for correctness and feature reference
- Other dialects must document feature gaps explicitly

End-of-life policy:

- Old supported versions may be deprecated with advance notice
- Removal of support must be tied to a major or otherwise clearly documented compatibility update

Upgrade strategy:

- Minor upgrades should be backward compatible for supported APIs and migrations
- Major upgrades may introduce breaking changes but must provide migration guidance

Migration compatibility guarantees:

- Generated migrations must remain valid for the supported PostgreSQL version range unless explicitly documented otherwise
- Migration files should be forward-applied in order and not depend on undefined runtime behavior
- Re-running schema checks against supported versions should produce consistent results

## Consequences

- Users can plan upgrades with clearer expectations.
- The project has a formal way to retire old environments.
- CI and release engineering can enforce compatibility boundaries.
- Future dialects do not weaken PostgreSQL-first guarantees.

## Alternatives Considered

### Unspecified compatibility

Support whatever versions happen to work.

### Support all versions indefinitely

Avoid retiring old Go or PostgreSQL versions.

### Lock to one exact environment

Support only a single Go and PostgreSQL combination.

## Why Alternatives Were Rejected

### Unspecified compatibility

Rejected because users need stable support expectations.

### Support all versions indefinitely

Rejected because it would increase maintenance burden and slow evolution.

### Lock to one exact environment

Rejected because it would make the framework impractical for real production adoption.

