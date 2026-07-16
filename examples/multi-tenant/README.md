# Multi-Tenant Example

This example shows how access policies affect queries in a company-aware app.

It demonstrates:

- automatic company filtering
- `IgnoreCompany`
- `IgnoreRLS`
- `System`
- Raw SQL as an explicit escape hatch with `WithoutPolicy()`

Raw SQL still requires an explicit policy decision and uses the active dialect to convert `?` placeholders.

Run it with:

```bash
export DORM_EXAMPLE_DSN='host=localhost port=5432 dbname=dorm user=postgres password=postgres sslmode=disable'
go run ./examples/multi-tenant
```
