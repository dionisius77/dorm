package access

import (
	"context"

	"github.com/dionisius77/dorm/schema"
)

// PolicyLevel identifies a row-level access policy mode.
type PolicyLevel string

const (
	PolicyLevelDefault      PolicyLevel = "default"
	PolicyLevelIgnoreCompany PolicyLevel = "ignore_company"
	PolicyLevelIgnoreRLS     PolicyLevel = "ignore_rls"
	PolicyLevelSystem        PolicyLevel = "system"
)

// Policy describes the active access policy for a request.
type Policy struct {
	Level PolicyLevel
}

// Default returns the default access policy.
func Default() Policy {
	return Policy{Level: PolicyLevelDefault}
}

// IgnoreCompany disables company-level filtering.
func IgnoreCompany() Policy {
	return Policy{Level: PolicyLevelIgnoreCompany}
}

// IgnoreRLS disables row-level security policies except soft delete.
func IgnoreRLS() Policy {
	return Policy{Level: PolicyLevelIgnoreRLS}
}

// System disables all access policies.
func System() Policy {
	return Policy{Level: PolicyLevelSystem}
}

// Normalize ensures the policy has a default level.
func (p Policy) Normalize() Policy {
	if p.Level == "" {
		return Default()
	}
	return p
}

// Name returns the observable policy name.
func (p Policy) Name() string {
	return "policy." + string(p.Normalize().Level)
}

// IsDefault reports whether the policy is the default policy.
func (p Policy) IsDefault() bool {
	return p.Normalize().Level == PolicyLevelDefault
}

// IsSystem reports whether the policy disables all policy enforcement.
func (p Policy) IsSystem() bool {
	return p.Normalize().Level == PolicyLevelSystem
}

// EnforcesSoftDelete reports whether soft delete remains active.
func (p Policy) EnforcesSoftDelete() bool {
	return !p.IsSystem()
}

// EnforcesAudit reports whether audit field injection remains active.
func (p Policy) EnforcesAudit() bool {
	return !p.IsSystem()
}

// AllowsScope reports whether a scoped column is active under the policy.
func (p Policy) AllowsScope(scope schema.ScopeKind) bool {
	switch p.Normalize().Level {
	case PolicyLevelDefault:
		return true
	case PolicyLevelIgnoreCompany:
		return scope != schema.ScopeCompany
	case PolicyLevelIgnoreRLS, PolicyLevelSystem:
		return false
	default:
		return true
	}
}

// WithPolicy stores the policy in a parent context.
func WithPolicy(parent context.Context, policy Policy) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	ac, ok := FromContext(parent)
	if !ok {
		ac = Context{}
	}
	ac.Policy = policy.Normalize()
	return WithContext(parent, ac)
}

// PolicyFromContext extracts the current policy from a context.
func PolicyFromContext(ctx context.Context) Policy {
	ac, _ := FromContext(ctx)
	return ac.Policy.Normalize()
}
