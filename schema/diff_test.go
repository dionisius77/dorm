package schema_test

import (
	"errors"
	"testing"

	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/schema"
)

func TestCompareDetectsAddedColumn(t *testing.T) {
	expected := &schema.Schema{
		Tables: []*schema.Table{
			{
				Name: "products",
				Columns: []*schema.Column{
					{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true},
				},
			},
		},
	}
	actual := &schema.Schema{
		Tables: []*schema.Table{
			{
				Name: "products",
				Columns: []*schema.Column{
					{Name: "id", Type: schema.Type{Name: "uuid", Kind: schema.TypeUUID}, PrimaryKey: true},
					{Name: "name", Type: schema.Type{Name: "text", Kind: schema.TypeString}},
				},
			},
		},
	}
	diff, err := schema.Compare(expected, actual)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Empty() {
		t.Fatalf("expected diff")
	}
}

func TestCompareReturnsInvalidSchemaError(t *testing.T) {
	_, err := schema.Compare(nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errkind.ErrInvalidSchema) {
		t.Fatalf("expected invalid schema error, got %T %v", err, err)
	}
}
