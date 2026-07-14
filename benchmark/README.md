# Benchmark Suite

This directory contains production-style benchmarks for the ORM.

Run them with:

```bash
go test ./benchmark -run '^$' -bench=. -benchmem
```

Benchmark philosophy:

- measure the ORM, not the network
- keep setup deterministic
- reuse the same PostgreSQL-backed fixture for comparable runs
- separate SQL generation from SQL execution
- keep the numbers useful for regression detection rather than ORM-to-ORM comparisons

To compare future runs:

```bash
go test ./benchmark -run '^$' -bench=. -benchmem > before.txt
go test ./benchmark -run '^$' -bench=. -benchmem > after.txt
benchstat before.txt after.txt
```
