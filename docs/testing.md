# Testing

Unit tests run with:

```bash
go test ./...
```

PostgreSQL integration tests are enabled automatically when one of these
environment variables is set:

- `DORM_TEST_POSTGRES_DSN`
- `DATABASE_URL`
- `POSTGRES_DSN`

Example:

```bash
export DORM_TEST_POSTGRES_DSN='host=localhost port=5432 dbname=dorm user=postgres password=postgres sslmode=disable'
go test ./...
```

The integration suite creates an isolated schema per test and cleans it up
automatically.
