# ADR-019: Naming and Coding Conventions

- **Status:** Accepted

## Context

Long-lived frameworks depend on consistency. Naming and structure affect readability, discoverability, contributor onboarding, and maintenance cost.

Without project-wide conventions, the codebase will fragment as it grows.

## Decision

Package naming:

- Use short, lowercase package names
- Prefer names that describe responsibility, such as `orm`, `migrate`, `schema`, `access`, and `dialect`

File naming:

- Use lowercase, descriptive file names
- Prefer one responsibility per file where practical

Interface naming:

- Use noun or capability-based names
- Avoid prefixing interfaces with `I`
- Name interfaces for behavior, not implementation

Model naming:

- Use singular, domain-based struct names
- Export model types intended for application use

Migration naming:

- Use deterministic numbering or timestamp-plus-slug naming
- Ensure migration filenames sort naturally

CLI naming:

- Use lowercase, explicit subcommands
- Prefer verbs for commands such as `init`, `generate`, `run`, `revert`, and `check`

Directory structure:

- Separate public surface area from internal implementation
- Keep domain-specific code grouped by package responsibility

Tag conventions:

- Use a single, documented tag namespace for ORM metadata
- Prefer explicit tags such as `orm:"pk"` or `orm:"soft_delete"`
- Reject ambiguous or conflicting tags

Error naming:

- Use descriptive error values that reflect domain meaning
- Name sentinel errors for well-defined conditions only

Internal package organization:

- Use internal subpackages to isolate implementation details
- Keep parsing, diffing, rendering, and inspection logic separate

Code organization principles:

- One responsibility per package where practical
- Keep public APIs small and explicit
- Favor declarative metadata over hidden convention

Documentation conventions:

- Document exported packages, types, functions, and commands
- Keep ADRs and developer docs aligned with the codebase
- Describe behavior in terms of user outcomes and guarantees

Example naming patterns:

- `orm.New`
- `migrate.Generate`
- `schema.DriftReport`
- `access.WithContext`
- `dialect/postgres`
- `User`, `Product`, `Invoice`
- `0001_initial.up.sql`
- `orm migrate generate`

## Consequences

- Contributors have a shared standard for readable code.
- Generated files and migrations remain easy to navigate.
- Public APIs stay predictable and discoverable.
- The project is easier to scale across many contributors.

## Alternatives Considered

### Ad hoc naming

Let each contributor choose their own style.

### Highly prescriptive code style without documentation

Rely only on linters and formatting tools.

### Overly clever conventions

Use compact or abbreviated names to save space.

## Why Alternatives Were Rejected

### Ad hoc naming

Rejected because it creates inconsistency and raises maintenance cost.

### Highly prescriptive code style without documentation

Rejected because tooling alone does not explain architectural intent.

### Overly clever conventions

Rejected because clarity is more valuable than brevity in a framework intended for long-term use.

