package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitCreatesProjectStructure(t *testing.T) {
	root := t.TempDir()
	if err := cmdInit([]string{"--root", root}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "migrations"),
		filepath.Join(root, "schemas"),
		filepath.Join(root, "models"),
		filepath.Join(root, "orm.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
}
