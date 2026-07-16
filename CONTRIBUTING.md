# Contributing to DORM

Thank you for your interest in contributing to DORM!

Whether you're fixing a typo, reporting a bug, improving documentation, or implementing a new feature, every contribution is appreciated.

---

## Before You Start

Please:

- Read the README.
- Read the relevant ADRs.
- Search existing issues before creating a new one.
- Keep discussions respectful.

---

## Ways to Contribute

You can contribute by:

- Reporting bugs
- Suggesting new features
- Improving documentation
- Writing examples
- Fixing bugs
- Improving performance
- Adding tests

---

## Reporting Bugs

Please include:

- DORM version
- Go version
- PostgreSQL version
- Minimal reproducible example
- Expected behavior
- Actual behavior

---

## Suggesting Features

For small features:

Open an issue describing:

- Problem
- Proposed solution
- Alternatives considered

For large features:

Start a discussion before opening a pull request.

Examples:

- New database dialect
- Migration engine changes
- Query builder redesign
- Access policy changes

---

## Development Setup

Clone repository

Install Go

Run

go test ./...

Run examples

Run benchmarks

(isi nanti sesuai project)

---

## Pull Request Guidelines

Every PR should:

- Focus on one change
- Include tests
- Update documentation when necessary
- Pass all CI checks
- Keep backward compatibility whenever possible

Avoid mixing:

- Refactoring
- New features
- Bug fixes

into a single PR.

---

## Architecture Changes

Architecture changes require discussion first.

Examples:

- Public API changes
- Migration behavior
- Transaction API
- Query generation
- Driver abstraction

These changes may require:

- Issue
- Discussion
- ADR

before implementation.

---

## Coding Style

General principles:

- Follow idiomatic Go.
- Keep APIs small.
- Avoid unnecessary abstractions.
- Prefer composition over inheritance-like patterns.
- Keep allocations minimal.
- Write benchmark when performance is affected.

---

## Testing

New functionality should include:

- Unit tests
- Integration tests (when applicable)

Performance-sensitive changes should include:

- Benchmarks

Run:

go test ./...

go test ./benchmark -bench=. -benchmem

before submitting.

---

## Commit Messages

Recommended format:

feat:
fix:
refactor:
docs:
test:
perf:

Example
feat(transaction): support nested savepoints

---

## Documentation

Please update documentation if you change:

- Public API
- CLI
- Migration behavior
- Error model
- Access policy

---

## Questions

If you're unsure about an implementation, open a discussion first.

We'd rather discuss early than review a large pull request that goes in the wrong direction.

Thank you for helping improve DORM ❤️