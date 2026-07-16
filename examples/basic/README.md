# Basic Example

This example introduces the ORM in a few minutes.

It shows:

- `Open()`
- `Ping()`
- `Close()`
- `Create`
- `Find`
- `Update`
- `Delete`
- explicit Raw SQL via `WithoutPolicy()`
- a simple lifecycle hook
- request-scoped context usage

Raw SQL uses generic `?` placeholders and participates in the current transaction automatically:

```go
err := db.Raw(ctx, `SELECT * FROM users WHERE email = ?`, email).
    WithoutPolicy().
    Scan(&users)
```

Run it with:

```bash
export DORM_EXAMPLE_DSN='host=localhost port=5432 dbname=dorm user=postgres password=postgres sslmode=disable'
go run ./examples/basic
```
