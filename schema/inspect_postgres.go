package schema

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type Inspector interface {
	Inspect(ctx context.Context, db *sql.DB, schemaName string) (*Schema, error)
}

type PostgresInspector struct{}

func (PostgresInspector) Inspect(ctx context.Context, db *sql.DB, schemaName string) (*Schema, error) {
	if db == nil {
		return nil, fmt.Errorf("schema: nil db")
	}
	if schemaName == "" {
		schemaName = "public"
	}
	s := &Schema{Name: schemaName}
	tables, err := readPostgresTables(ctx, db, schemaName)
	if err != nil {
		return nil, err
	}
	cols, err := readPostgresColumns(ctx, db, schemaName)
	if err != nil {
		return nil, err
	}
	idx, err := readPostgresIndexes(ctx, db, schemaName)
	if err != nil {
		return nil, err
	}
	for _, tableName := range tables {
		table := &Table{Name: tableName}
		table.Columns = cols[tableName]
		table.Indexes = idx[tableName]
		s.Tables = append(s.Tables, table)
	}
	s.Sort()
	return s, nil
}

func readPostgresTables(ctx context.Context, db *sql.DB, schemaName string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`, schemaName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func readPostgresColumns(ctx context.Context, db *sql.DB, schemaName string) (map[string][]*Column, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, column_name, is_nullable, data_type, udt_name, column_default
		FROM information_schema.columns
		WHERE table_schema = $1
		ORDER BY table_name, ordinal_position
	`, schemaName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]*Column{}
	for rows.Next() {
		var tableName, columnName, isNullable, dataType, udtName, columnDefault string
		if err := rows.Scan(&tableName, &columnName, &isNullable, &dataType, &udtName, &columnDefault); err != nil {
			return nil, err
		}
		typ := dataType
		if udtName != "" {
			typ = udtName
		}
		col := &Column{
			Name:     columnName,
			Type:     Type{Name: typ, Kind: typeKindFromName(typ)},
			Nullable: strings.EqualFold(isNullable, "YES"),
			Default:  columnDefault,
		}
		out[tableName] = append(out[tableName], col)
	}
	return out, rows.Err()
}

func readPostgresIndexes(ctx context.Context, db *sql.DB, schemaName string) (map[string][]*Index, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT tablename, indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = $1
		ORDER BY tablename, indexname
	`, schemaName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]*Index{}
	for rows.Next() {
		var tableName, indexName, indexDef string
		if err := rows.Scan(&tableName, &indexName, &indexDef); err != nil {
			return nil, err
		}
		out[tableName] = append(out[tableName], &Index{Name: indexName, Metadata: map[string]string{"definition": indexDef}})
	}
	for _, indexes := range out {
		sort.SliceStable(indexes, func(i, j int) bool { return indexes[i].Name < indexes[j].Name })
	}
	return out, rows.Err()
}
