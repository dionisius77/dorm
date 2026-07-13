package schema

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"github.com/dionisius77/dorm/errkind"
)

type Inspector interface {
	Inspect(ctx context.Context, db *sql.DB, schemaName string) (*Schema, error)
}

// PostgresInspector inspects PostgreSQL metadata into a Schema.
type PostgresInspector struct{}

// Inspect loads PostgreSQL tables, columns, and indexes for a schema.
func (PostgresInspector) Inspect(ctx context.Context, db *sql.DB, schemaName string) (*Schema, error) {
	var result *Schema
	err := traceOperation(ctx, "db.schema.inspect", func(ctx context.Context) error {
		if db == nil {
			return errkind.New(errkind.KindConfiguration, "schema: nil db")
		}
		if schemaName == "" {
			schemaName = "public"
		}
		s := &Schema{Name: schemaName}
		tables, err := readPostgresTables(ctx, db, schemaName)
		if err != nil {
			return err
		}
		cols, err := readPostgresColumns(ctx, db, schemaName)
		if err != nil {
			return err
		}
		idx, err := readPostgresIndexes(ctx, db, schemaName)
		if err != nil {
			return err
		}
		for _, tableName := range tables {
			table := &Table{Name: tableName}
			table.Columns = cols[tableName]
			table.Indexes = idx[tableName]
			s.Tables = append(s.Tables, table)
		}
		s.Sort()
		result = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func readPostgresTables(ctx context.Context, db *sql.DB, schemaName string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`, schemaName)
	if err != nil {
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres tables", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres tables", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres tables", err)
	}
	return out, nil
}

func readPostgresColumns(ctx context.Context, db *sql.DB, schemaName string) (map[string][]*Column, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, column_name, is_nullable, data_type, udt_name, column_default
		FROM information_schema.columns
		WHERE table_schema = $1
		ORDER BY table_name, ordinal_position
	`, schemaName)
	if err != nil {
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres columns", err)
	}
	defer rows.Close()
	out := map[string][]*Column{}
	for rows.Next() {
		var tableName, columnName, isNullable, dataType, udtName string
		var columnDefault sql.NullString
		if err := rows.Scan(&tableName, &columnName, &isNullable, &dataType, &udtName, &columnDefault); err != nil {
			return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres columns", err)
		}
		typ := dataType
		if udtName != "" {
			typ = udtName
		}
		col := &Column{
			Name:     columnName,
			Type:     Type{Name: typ, Kind: typeKindFromName(typ)},
			Nullable: strings.EqualFold(isNullable, "YES"),
		}
		if columnDefault.Valid {
			col.Default = columnDefault.String
		}
		out[tableName] = append(out[tableName], col)
	}
	if err := rows.Err(); err != nil {
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres columns", err)
	}
	return out, nil
}

func readPostgresIndexes(ctx context.Context, db *sql.DB, schemaName string) (map[string][]*Index, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT tablename, indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = $1
		ORDER BY tablename, indexname
	`, schemaName)
	if err != nil {
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres indexes", err)
	}
	defer rows.Close()
	out := map[string][]*Index{}
	for rows.Next() {
		var tableName, indexName, indexDef string
		if err := rows.Scan(&tableName, &indexName, &indexDef); err != nil {
			return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres indexes", err)
		}
		out[tableName] = append(out[tableName], &Index{Name: indexName, Metadata: map[string]string{"definition": indexDef}})
	}
	for _, indexes := range out {
		sort.SliceStable(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })
	}
	if err := rows.Err(); err != nil {
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres indexes", err)
	}
	return out, nil
}
