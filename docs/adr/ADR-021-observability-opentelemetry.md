# ADR-021: Observability & OpenTelemetry

- **Status:** Accepted

## Context

This project is a production-grade PostgreSQL-first ORM for Go. In production systems, database access must be observable without forcing every application to add custom instrumentation around each repository call.

Developers need to understand:

- which SQL statements are executed
- how long queries take
- transaction duration and outcome
- migration execution behavior
- schema drift detection results
- hook execution
- retries and failures

Observability is not a nice-to-have feature. It is part of the safety and operability contract of the framework.

## Decision

OpenTelemetry will be a first-class feature of the ORM and related tooling.

Instrumentation will be built into the core runtime and tooling paths rather than layered on externally as middleware or optional wrappers.

Tracing will propagate through Go `context.Context`, and normal ORM usage will require no manual instrumentation beyond passing a context value.

Observability will cover:

- query execution
- transactions
- migration generation and execution
- schema inspection and drift detection
- hooks
- prepared statements
- custom SQL execution

## Consequences

- Users get distributed tracing, metrics, and diagnostics with minimal setup.
- The ORM can emit useful telemetry across the full data lifecycle, not just query execution.
- Application teams can adopt the framework without writing custom spans around each repository call.
- Observability behavior becomes deterministic and part of the public architecture contract.
- The codebase gains a required cross-cutting concern that must be kept lightweight and consistent.

## Alternatives Considered

### External middleware only

Expose hook points and ask application developers to add tracing and metrics themselves.

### Plugin-only instrumentation

Allow observability through optional plugins rather than the core.

### No built-in observability

Leave tracing, metrics, and logging entirely to consuming applications.

## Why Alternatives Were Rejected

### External middleware only

Rejected because it creates inconsistent instrumentation, misses internal operations like migrations and schema checks, and forces every user to solve the same problem independently.

### Plugin-only instrumentation

Rejected because observability is too important to depend on optional integration code that may not be enabled in production.

### No built-in observability

Rejected because it conflicts with the project goal of being production-safe, diagnosable, and easy to operate.

## Tracing

The ORM will create spans automatically for these operations:

- connect
- ping
- query
- select
- insert
- update
- delete
- upsert
- batch insert
- transactions
- commit
- rollback
- prepared statements
- schema inspection
- schema drift detection
- migration generation
- migration execution
- hooks
- custom SQL execution

Nested spans are required. A request span should be able to contain repository spans, which in turn may contain ORM spans and driver spans.

Span naming should be consistent and low-cardinality. The preferred style is short, stable, and operation-oriented:

- `db.query`
- `db.insert`
- `db.update`
- `db.delete`
- `db.upsert`
- `db.transaction`
- `db.commit`
- `db.rollback`
- `db.migration`
- `db.schema.check`
- `db.schema.inspect`
- `db.connect`

Tracing should follow OpenTelemetry semantic conventions where they fit naturally.

## Span Attributes

Standard OpenTelemetry database attributes should be populated where applicable:

- `db.system`
- `db.name`
- `db.namespace`
- `db.operation`
- `db.statement`
- `server.address`
- `server.port`

The ORM may also attach framework-specific attributes to improve diagnostics:

- `orm.model`
- `orm.operation`
- `orm.table`
- `orm.rows`
- `orm.duration`
- `orm.transaction`
- `orm.batch_size`
- `orm.soft_delete`
- `orm.schema_version`

Custom attributes must remain clearly distinguishable from standard OpenTelemetry attributes.

## SQL Logging

The framework will support configurable SQL logging modes:

- disabled
- errors only
- slow queries
- debug
- trace

Each log entry should include:

- SQL text
- bound parameters when allowed
- execution duration
- affected rows
- execution timestamp

Logging should integrate with tracing so a single operation can be diagnosed through spans and logs together.

## Sensitive Data

The ORM must not expose sensitive parameter values by default.

Default masking should include common secrets such as:

- password
- access_token
- refresh_token
- authorization
- cookie
- secret
- api_key
- jwt

Developers must be able to register additional fields for masking.

The system must also support disabling parameter logging entirely.

Security takes precedence over debugging convenience.

## Slow Query Detection

Slow query detection will be built in and threshold-based.

Example thresholds:

- 200ms
- 500ms
- 1000ms

When a slow query is detected, the ORM should:

- mark the span
- emit a structured log entry
- expose metrics

## Metrics

The observability layer should expose metrics that work naturally with OpenTelemetry and Prometheus-style backends.

Representative metrics include:

- query count
- failed queries
- transaction count
- rollback count
- migration duration
- migration failures
- schema drift count
- connection count
- prepared statement count
- rows returned
- rows affected

## Transactions

Transaction spans should represent the transaction boundary, with child spans for work performed inside that transaction.

Example flow:

- transaction span
- child insert span
- child insert span
- child commit span

If a transaction rolls back, the rollback event should be recorded on the transaction span and as its own operation span when appropriate.

## Error Recording

Database and migration errors should be recorded on spans automatically.

The system should:

- mark the span as errored
- attach exception attributes
- preserve the original error value
- avoid hiding database-specific details

Telemetry must not replace error handling. It should enrich it.

## Performance

Observability must be designed for minimal overhead.

When tracing is disabled:

- avoid allocations that are only needed for spans
- avoid reflection for telemetry plumbing
- avoid string formatting for unused attributes
- avoid building spans or log payloads unnecessarily

The observability layer should approach near-zero overhead when disabled.

## Configuration

Observability configuration should remain simple and idiomatic.

Required capabilities include:

- enable tracing
- enable metrics
- enable SQL logging
- configure slow query threshold
- configure parameter masking
- inject custom logger
- inject custom tracer provider
- inject custom meter provider

## Future Extensibility

The observability design should allow integration with common ecosystem tools without changing the ORM API:

- Grafana
- Jaeger
- Zipkin
- Tempo
- Prometheus
- OpenSearch
- Elastic
- Honeycomb

The framework should expose standard OpenTelemetry hooks so backend selection stays external to the core API.

