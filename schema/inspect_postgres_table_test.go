package schema

import "testing"

func TestShouldSkipInspectorTable(t *testing.T) {
	if !shouldSkipInspectorTable("orm_migrations") {
		t.Fatal("expected orm_migrations to be skipped")
	}
	if !shouldSkipInspectorTable("ORM_MIGRATIONS") {
		t.Fatal("expected orm_migrations match to be case-insensitive")
	}
	if shouldSkipInspectorTable("users") {
		t.Fatal("expected users to be included")
	}
}
