# Todo Example

This example shows a small CRUD workflow with the public ORM API.

It demonstrates:

- `Create`
- `Find`
- `Update`
- `Delete`
- listing records with query options

Run it with:

```bash
export DORM_EXAMPLE_DSN='host=localhost port=5432 dbname=dorm user=postgres password=postgres sslmode=disable'
go run ./examples/todo
```
