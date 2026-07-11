package dorm

import "testing"

func TestRoadmapIncludesStableCoreAndExperimentalModules(t *testing.T) {
	roadmap := Roadmap()
	if len(roadmap) < 8 {
		t.Fatalf("expected roadmap entries, got %d", len(roadmap))
	}
	if len(StableCore()) != 5 {
		t.Fatalf("expected 5 stable core items, got %d", len(StableCore()))
	}
	if len(ExperimentalRoadmap()) == 0 {
		t.Fatalf("expected experimental roadmap items")
	}
}

func TestRoadmapCoreRemainsStable(t *testing.T) {
	for _, module := range StableCore() {
		if !module.Contract.IsStable() {
			t.Fatalf("expected %s to be stable", module.Name)
		}
	}
}
