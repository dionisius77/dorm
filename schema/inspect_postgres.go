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
	constraints, excludedIndexes, err := readPostgresConstraints(ctx, db, schemaName)
	if err != nil {
		return err
	}
	applyConstraintMetadata(cols, constraints)
	idx, err := readPostgresIndexes(ctx, db, schemaName, excludedIndexes)
	if err != nil {
		return err
	}
	for _, tableName := range tables {
		table := &Table{Name: tableName}
		table.Columns = cols[tableName]
		table.Constraints = constraints[tableName]
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
		if shouldSkipInspectorTable(name) {
			continue
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		return nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres tables", err)
	}
	return out, nil
}

func shouldSkipInspectorTable(name string) bool {
	return strings.EqualFold(name, "orm_migrations")
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
		typ := normalizePostgresTypeName(dataType, udtName)
		nullable := strings.EqualFold(isNullable, "YES")
		col := &Column{
			Name:     columnName,
			Type:     Type{Name: typ, Kind: typeKindFromName(typ), Nullable: nullable},
			Nullable: nullable,
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

func normalizePostgresTypeName(dataType, udtName string) string {
	name := strings.TrimSpace(strings.ToLower(dataType))
	switch name {
	case "integer", "int4":
		return "integer"
	case "smallint", "int2":
		return "smallint"
	case "bigint", "int8":
		return "bigint"
	case "timestamp with time zone", "timestamptz":
		return "timestamptz"
	case "timestamp without time zone", "timestamp":
		return "timestamp"
	case "character varying", "varchar":
		return "text"
	case "character", "bpchar":
		return "text"
	case "boolean", "bool":
		return "boolean"
	case "uuid":
		return "uuid"
	case "text":
		return "text"
	}
	name = strings.TrimSpace(strings.ToLower(udtName))
	switch name {
	case "int2":
		return "smallint"
	case "int4":
		return "integer"
	case "int8":
		return "bigint"
	case "timestamptz":
		return "timestamptz"
	case "timestamp":
		return "timestamp"
	case "varchar":
		return "text"
	case "bpchar":
		return "text"
	case "uuid":
		return "uuid"
	case "bool":
		return "boolean"
	case "text":
		return "text"
	}
	if dataType != "" {
		return strings.TrimSpace(dataType)
	}
	return strings.TrimSpace(udtName)
}

func readPostgresConstraints(ctx context.Context, db *sql.DB, schemaName string) (map[string][]*Constraint, map[string]struct{}, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			c.conrelid::regclass::text AS table_name,
			c.conname,
			c.contype,
			c.conindid::regclass::text AS index_name,
			string_agg(a.attname, ',' ORDER BY x.ord) AS columns
		FROM pg_constraint c
		JOIN pg_namespace n ON n.oid = c.connamespace
		JOIN unnest(c.conkey) WITH ORDINALITY AS x(attnum, ord) ON true
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = x.attnum
		WHERE n.nspname = $1 AND c.contype IN ('p', 'u')
		GROUP BY c.conrelid, c.conname, c.contype, c.conindid
		ORDER BY table_name, c.conname
	`, schemaName)
	if err != nil {
		return nil, nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres constraints", err)
	}
	defer rows.Close()
	out := map[string][]*Constraint{}
	excludedIndexes := map[string]struct{}{}
	for rows.Next() {
		var tableName, conName, conType, indexName, columns string
		if err := rows.Scan(&tableName, &conName, &conType, &indexName, &columns); err != nil {
			return nil, nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres constraints", err)
		}
		if indexName != "" && indexName != "-" {
			excludedIndexes[indexName] = struct{}{}
		}
		constraint := &Constraint{
			Name:    conName,
			Columns: splitConstraintColumns(columns),
		}
		switch conType {
		case "p":
			constraint.Kind = ConstraintPrimaryKey
		case "u":
			constraint.Kind = ConstraintUnique
		default:
			continue
		}
		out[tableName] = append(out[tableName], constraint)
	}
	for _, constraints := range out {
		sort.SliceStable(constraints, func(i, j int) bool { return constraints[i].Name < constraints[j].Name })
	}
	if err := rows.Err(); err != nil {
		return nil, nil, errkind.Wrap(errkind.KindRuntimeQuery, "schema: read postgres constraints", err)
	}
	return out, excludedIndexes, nil
}

func applyConstraintMetadata(cols map[string][]*Column, constraints map[string][]*Constraint) {
	for tableName, tableConstraints := range constraints {
		tableCols := cols[tableName]
		for _, constraint := range tableConstraints {
			if constraint == nil {
				continue
			}
			for _, colName := range constraint.Columns {
				for _, col := range tableCols {
					if col == nil || col.Name != colName {
						continue
					}
					switch constraint.Kind {
					case ConstraintPrimaryKey:
						col.PrimaryKey = true
						col.Unique = true
					case ConstraintUnique:
						col.Unique = true
					}
				}
			}
		}
	}
}

func splitConstraintColumns(columns string) []string {
	if columns == "" {
		return nil
	}
	parts := strings.Split(columns, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func readPostgresIndexes(ctx context.Context, db *sql.DB, schemaName string, excluded map[string]struct{}) (map[string][]*Index, error) {
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
		if _, skip := excluded[indexName]; skip {
			continue
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
