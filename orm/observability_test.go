package orm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultObservabilityConfig(t *testing.T) {
	cfg := DefaultObservabilityConfig()
	if cfg.SQLLogging != SQLLogDisabled {
		t.Fatalf("expected disabled logging, got %q", cfg.SQLLogging)
	}
	if cfg.Enabled() {
		t.Fatalf("expected disabled config")
	}
}

func TestObservabilityConfigValidate(t *testing.T) {
	cfg := ObservabilityConfig{SQLLogging: SQLLogTrace, SlowQueryThreshold: time.Second}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
	if err := (ObservabilityConfig{SQLLogging: SQLLogMode("bad")}).Validate(); err == nil {
		t.Fatalf("expected invalid mode error")
	}
	if err := (ObservabilityConfig{SlowQueryThreshold: -time.Second}).Validate(); err == nil {
		t.Fatalf("expected negative threshold error")
	}
}

func TestObservabilityConfigMasking(t *testing.T) {
	cfg := ObservabilityConfig{MaskedFields: []string{"custom_secret"}}.Normalized()
	if !cfg.ShouldMask("password") {
		t.Fatalf("expected password to be masked")
	}
	if !cfg.ShouldMask("custom_secret") {
		t.Fatalf("expected custom_secret to be masked")
	}
	if cfg.ShouldMask("harmless") {
		t.Fatalf("did not expect harmless to be masked")
	}
}

func TestNewNormalizesObservabilityConfig(t *testing.T) {
	db := New(Config{
		Observability: ObservabilityConfig{
			MaskedFields: []string{"custom_secret"},
		},
	})
	if db == nil {
		t.Fatalf("expected db")
	}
	if !db.observability.ShouldMask("password") {
		t.Fatalf("expected default mask list to be present")
	}
	if !db.observability.ShouldMask("custom_secret") {
		t.Fatalf("expected custom mask to be present")
	}
}

func TestORMPackageDoesNotImportOpenTelemetry(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "opentelemetry") {
			t.Fatalf("public orm package should not import open telemetry directly: %s", name)
		}
	}
}
