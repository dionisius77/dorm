package dorm

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/dionisius77/dorm/errkind"
)

const (
	// MinimumSupportedGoVersion is the minimum Go toolchain version supported by this release line.
	MinimumSupportedGoVersion = "1.26"
)

var (
	// SupportedOS lists the platform targets considered supported by the framework.
	SupportedOS = []string{"linux", "darwin", "windows"}
	// SupportedArch lists the CPU architectures considered supported by the framework.
	SupportedArch = []string{"amd64", "arm64"}
	// SupportedPostgresMajorVersions lists the PostgreSQL major versions validated by this release line.
	SupportedPostgresMajorVersions = []int{13, 14, 15, 16, 17}
)

// CompatibilityPolicy describes supported runtime and database environments.
type CompatibilityPolicy struct {
	MinimumGoVersion string
	OperatingSystems  []string
	Architectures     []string
	PostgresMajors    []int
}

// DefaultCompatibilityPolicy returns the framework compatibility policy for this release line.
func DefaultCompatibilityPolicy() CompatibilityPolicy {
	return CompatibilityPolicy{
		MinimumGoVersion: MinimumSupportedGoVersion,
		OperatingSystems: append([]string(nil), SupportedOS...),
		Architectures:     append([]string(nil), SupportedArch...),
		PostgresMajors:    append([]int(nil), SupportedPostgresMajorVersions...),
	}
}

// ValidateRuntime checks whether the current Go runtime and platform are supported.
func (p CompatibilityPolicy) ValidateRuntime() error {
	if p.MinimumGoVersion == "" {
		p.MinimumGoVersion = MinimumSupportedGoVersion
	}
	if len(p.OperatingSystems) == 0 {
		p.OperatingSystems = append([]string(nil), SupportedOS...)
	}
	if len(p.Architectures) == 0 {
		p.Architectures = append([]string(nil), SupportedArch...)
	}
	if !containsString(p.OperatingSystems, runtime.GOOS) {
		return errkind.New(errkind.KindUnsupportedFeature, fmt.Sprintf("dorm: unsupported operating system %q", runtime.GOOS))
	}
	if !containsString(p.Architectures, runtime.GOARCH) {
		return errkind.New(errkind.KindUnsupportedFeature, fmt.Sprintf("dorm: unsupported architecture %q", runtime.GOARCH))
	}
	minMajor, minMinor, err := parseGoVersion(p.MinimumGoVersion)
	if err != nil {
		return err
	}
	major, minor, err := parseGoVersion(runtime.Version())
	if err != nil {
		return errkind.Wrap(errkind.KindUnsupportedFeature, "dorm: parse runtime version", err)
	}
	if major < minMajor || (major == minMajor && minor < minMinor) {
		return errkind.New(errkind.KindUnsupportedFeature, fmt.Sprintf("dorm: requires Go %s or newer", p.MinimumGoVersion))
	}
	return nil
}

// SupportsPostgreSQLMajor reports whether the provided PostgreSQL major version is supported.
func (p CompatibilityPolicy) SupportsPostgreSQLMajor(major int) bool {
	if len(p.PostgresMajors) == 0 {
		p.PostgresMajors = append([]int(nil), SupportedPostgresMajorVersions...)
	}
	return containsInt(p.PostgresMajors, major)
}

// Summary returns a stable human-readable summary of the compatibility policy.
func (p CompatibilityPolicy) Summary() string {
	if p.MinimumGoVersion == "" {
		p.MinimumGoVersion = MinimumSupportedGoVersion
	}
	return fmt.Sprintf(
		"Go >= %s; OS=%s; ARCH=%s; PostgreSQL majors=%s",
		p.MinimumGoVersion,
		strings.Join(p.OperatingSystems, ","),
		strings.Join(p.Architectures, ","),
		intSliceToString(p.PostgresMajors),
	)
}

func parseGoVersion(v string) (int, int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, 0, errkind.New(errkind.KindConfiguration, "dorm: empty go version")
	}
	v = strings.TrimPrefix(v, "go")
	v = strings.TrimPrefix(v, "devel ")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, errkind.New(errkind.KindConfiguration, fmt.Sprintf("dorm: invalid go version %q", v))
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, errkind.Wrap(errkind.KindConfiguration, "dorm: parse go version major", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, errkind.Wrap(errkind.KindConfiguration, "dorm: parse go version minor", err)
	}
	return major, minor, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func intSliceToString(values []int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(value))
	}
	return strings.Join(parts, ",")
}
