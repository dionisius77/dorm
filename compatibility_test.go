package dorm

import "testing"

func TestDefaultCompatibilityPolicyAllowsCurrentRuntime(t *testing.T) {
	if err := DefaultCompatibilityPolicy().ValidateRuntime(); err != nil {
		t.Fatalf("expected current runtime to be supported: %v", err)
	}
}

func TestCompatibilityPolicySupportsPostgreSQLMajor(t *testing.T) {
	policy := DefaultCompatibilityPolicy()
	if !policy.SupportsPostgreSQLMajor(13) {
		t.Fatalf("expected PostgreSQL 13 to be supported")
	}
	if policy.SupportsPostgreSQLMajor(99) {
		t.Fatalf("did not expect PostgreSQL 99 to be supported")
	}
}
