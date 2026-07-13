package dorm

import "fmt"

const (
	// VersionMajor is the major release component for the current public API.
	VersionMajor = 0
	// VersionMinor is the minor release component for the current public API.
	VersionMinor = 1
	// VersionPatch is the patch release component for the current public API.
	VersionPatch = 6
)

// Version returns the current module version in semantic version format.
func Version() string {
	return fmt.Sprintf("v%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
}

// APILifecycle describes the lifecycle of a public symbol.
type APILifecycle string

const (
	// APILifecycleExperimental marks a symbol that may change without notice.
	APILifecycleExperimental APILifecycle = "experimental"
	// APILifecycleStable marks a supported public symbol.
	APILifecycleStable APILifecycle = "stable"
	// APILifecycleDeprecated marks a public symbol that should not be used for new code.
	APILifecycleDeprecated APILifecycle = "deprecated"
	// APILifecycleRemoved marks a symbol that has been removed from the public API.
	APILifecycleRemoved APILifecycle = "removed"
)

// APIContract describes the lifecycle state of a public symbol or package.
type APIContract struct {
	Name            string
	Lifecycle       APILifecycle
	DeprecatedSince string
	Replacement     string
}

// StableAPI creates a stable API contract record.
func StableAPI(name string) APIContract {
	return APIContract{Name: name, Lifecycle: APILifecycleStable}
}

// ExperimentalAPI creates an experimental API contract record.
func ExperimentalAPI(name string) APIContract {
	return APIContract{Name: name, Lifecycle: APILifecycleExperimental}
}

// DeprecatedAPI creates a deprecated API contract record.
func DeprecatedAPI(name, since, replacement string) APIContract {
	return APIContract{
		Name:            name,
		Lifecycle:       APILifecycleDeprecated,
		DeprecatedSince: since,
		Replacement:     replacement,
	}
}

// IsDeprecated reports whether the contract is deprecated.
func (c APIContract) IsDeprecated() bool {
	return c.Lifecycle == APILifecycleDeprecated
}

// IsStable reports whether the contract is stable.
func (c APIContract) IsStable() bool {
	return c.Lifecycle == APILifecycleStable
}
