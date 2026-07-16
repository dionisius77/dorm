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
- a simple lifecycle hook
- request-scoped context usage

Run it with:

```bash
export DORM_EXAMPLE_DSN='host=localhost port=5432 dbname=dorm user=postgres password=postgres sslmode=disable'
go run ./examples/basic
```
