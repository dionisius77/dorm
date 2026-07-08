package schema_test

import (
	"testing"

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
