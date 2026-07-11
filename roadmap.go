package dorm

// RoadmapModule describes a stable core capability or a planned future module.
type RoadmapModule struct {
	Name        string
	Description string
	Contract    APIContract
}

// Roadmap returns the stable core and the likely future modules for the framework.
func Roadmap() []RoadmapModule {
	return []RoadmapModule{
		{
			Name:        "core.model_schema",
			Description: "Model-driven schema definitions remain the stable foundation.",
			Contract:    StableAPI("core.model_schema"),
		},
		{
			Name:        "core.deterministic_migrations",
			Description: "Deterministic migrations remain part of the stable core.",
			Contract:    StableAPI("core.deterministic_migrations"),
		},
		{
			Name:        "core.schema_drift_detection",
			Description: "Schema drift detection remains part of the stable core.",
			Contract:    StableAPI("core.schema_drift_detection"),
		},
		{
			Name:        "core.postgresql_dialect",
			Description: "PostgreSQL-first behavior remains the correctness baseline.",
			Contract:    StableAPI("core.postgresql_dialect"),
		},
		{
			Name:        "core.explicit_access_control",
			Description: "Explicit access control remains part of the stable core.",
			Contract:    StableAPI("core.explicit_access_control"),
		},
		{
			Name:        "experimental.codegen.models",
			Description: "Model code generation may be added as an optional module.",
			Contract:    ExperimentalAPI("experimental.codegen.models"),
		},
		{
			Name:        "experimental.codegen.repositories",
			Description: "Typed repository generation may be added as an optional module.",
			Contract:    ExperimentalAPI("experimental.codegen.repositories"),
		},
		{
			Name:        "experimental.query_analyzer",
			Description: "Query analysis may be added as an optional module.",
			Contract:    ExperimentalAPI("experimental.query_analyzer"),
		},
		{
			Name:        "experimental.observability.otel",
			Description: "OpenTelemetry integration may be added as an optional module.",
			Contract:    ExperimentalAPI("experimental.observability.otel"),
		},
		{
			Name:        "experimental.observability.metrics",
			Description: "Metrics export may be added as an optional module.",
			Contract:    ExperimentalAPI("experimental.observability.metrics"),
		},
		{
			Name:        "experimental.observability.tracing",
			Description: "Tracing integration may be added as an optional module.",
			Contract:    ExperimentalAPI("experimental.observability.tracing"),
		},
		{
			Name:        "experimental.access.policy_engine",
			Description: "Richer access policy engines may be added as an optional module.",
			Contract:    ExperimentalAPI("experimental.access.policy_engine"),
		},
	}
}

// StableCore returns the stable core roadmap items only.
func StableCore() []RoadmapModule {
	roadmap := Roadmap()
	out := make([]RoadmapModule, 0, 5)
	for _, module := range roadmap {
		if module.Contract.IsStable() {
			out = append(out, module)
		}
	}
	return out
}

// ExperimentalRoadmap returns the planned future modules only.
func ExperimentalRoadmap() []RoadmapModule {
	roadmap := Roadmap()
	out := make([]RoadmapModule, 0, len(roadmap))
	for _, module := range roadmap {
		if module.Contract.Lifecycle == APILifecycleExperimental {
			out = append(out, module)
		}
	}
	return out
}
