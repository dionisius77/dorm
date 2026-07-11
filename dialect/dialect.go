package dialect

import "github.com/dionisius77/dorm/schema"

// Capabilities describes features supported by a SQL dialect.
type Capabilities struct {
	UUID               bool
	JSONB              bool
	ARRAY              bool
	ENUM               bool
	GeneratedColumns   bool
	MaterializedViews  bool
	PartialIndexes     bool
	ExpressionIndexes  bool
	Gin                bool
	Gist               bool
	IdentityColumns    bool
	CompositeKeys      bool
	CompositeIndexes   bool
	PreparedStatements bool
}

// Dialect renders SQL for a specific database flavor.
type Dialect interface {
	Name() string
	QuoteIdent(string) string
	Placeholder(int) string
	Capabilities() Capabilities
	ColumnDefinition(*schema.Column) (string, error)
	RenderOperation(schema.Operation) (string, error)
	RenderMigration(*schema.Diff) ([]string, error)
	RenderSelect(table string, columns []string, where []string, orderBy []string, limit, offset *int) (string, error)
	RenderInsert(table string, columns []string, returning []string) (string, error)
	RenderUpdate(table string, set []string, where []string, returning []string) (string, error)
	RenderDelete(table string, where []string, returning []string) (string, error)
}
