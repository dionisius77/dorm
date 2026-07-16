package dialect

import (
	"strconv"
	"testing"

	"github.com/dionisius77/dorm/schema"
)

type placeholderTestDialect struct{}

func (placeholderTestDialect) Name() string               { return "placeholder-test" }
func (placeholderTestDialect) QuoteIdent(s string) string { return s }
func (placeholderTestDialect) Placeholder(i int) string   { return "@" + strconv.Itoa(i) }
func (placeholderTestDialect) Capabilities() Capabilities { return Capabilities{} }
func (placeholderTestDialect) ColumnDefinition(*schema.Column) (string, error) {
	return "", nil
}
func (placeholderTestDialect) RenderOperation(schema.Operation) (string, error) { return "", nil }
func (placeholderTestDialect) RenderMigration(*schema.Diff) ([]string, error)   { return nil, nil }
func (placeholderTestDialect) RenderSelect(string, []string, []string, []string, *int, *int) (string, error) {
	return "", nil
}
func (placeholderTestDialect) RenderInsert(string, []string, []string) (string, error) {
	return "", nil
}
func (placeholderTestDialect) RenderUpdate(string, []string, []string, []string) (string, error) {
	return "", nil
}
func (placeholderTestDialect) RenderDelete(string, []string, []string) (string, error) {
	return "", nil
}

func TestBindPlaceholdersConvertsQuestionMarks(t *testing.T) {
	got := BindPlaceholders("SELECT * FROM users WHERE email = ? AND status = ?", placeholderTestDialect{})
	want := "SELECT * FROM users WHERE email = @1 AND status = @2"
	if got != want {
		t.Fatalf("expected placeholder binding %q, got %q", want, got)
	}
}
