package schema

import "testing"

func TestNormalizePostgresTypeName(t *testing.T) {
	tests := []struct {
		dataType string
		udtName  string
		want     string
	}{
		{dataType: "bigint", udtName: "int8", want: "bigint"},
		{dataType: "timestamp with time zone", udtName: "timestamptz", want: "timestamptz"},
		{dataType: "character varying", udtName: "varchar", want: "text"},
		{dataType: "uuid", udtName: "uuid", want: "uuid"},
	}
	for _, tt := range tests {
		if got := normalizePostgresTypeName(tt.dataType, tt.udtName); got != tt.want {
			t.Fatalf("normalizePostgresTypeName(%q, %q) = %q, want %q", tt.dataType, tt.udtName, got, tt.want)
		}
	}
}
