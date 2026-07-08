# ADR-010: CLI Philosophy

- **Status:** Accepted

## Context

This project is not just a runtime library. It also manages schema generation, migration execution, drift detection, and operational diagnostics. Those capabilities need explicit command-line workflows.

The CLI must reinforce the project's core principles:

- schema changes are explicit
- migrations are deterministic
- drift is visible
- production safety matters more than convenience

## Decision

The project will provide a CLI with explicit subcommands for each schema-related workflow:

- `orm init`
- `orm migrate generate`
- `orm migrate run`
- `orm migrate revert`
- `orm migrate status`
- `orm schema check`
- `orm doctor`

Responsibilities:

- `init` scaffolds project structure and configuration
- `migrate generate` creates deterministic migration files from model diffs
- `migrate run` applies pending migrations in order
- `migrate revert` rolls back a migration when supported
- `migrate status` reports applied and pending migrations
- `schema check` compares expected and actual schema for drift
- `doctor` validates configuration, connectivity, snapshot integrity, and dialect support

## Consequences

- Schema operations are discoverable and reviewable.
- Users are encouraged to treat schema changes as deliberate actions.
- CI/CD can automate verification through explicit commands.
- Tooling remains separate from runtime request handling.

## Alternatives Considered

### Library-only approach

Expose everything through Go APIs and avoid a CLI.

### Hidden background commands

Perform migration generation or drift checks automatically in app startup.

### Single all-purpose command

Provide one command with many flags instead of clear subcommands.

## Why Alternatives Were Rejected

### Library-only approach

Rejected because the project needs first-class operational workflows for migrations and drift management.

### Hidden background commands

Rejected because they obscure behavior and conflict with the explicit-change philosophy.

### Single all-purpose command

Rejected because it becomes difficult to understand, document, and safely automate.

