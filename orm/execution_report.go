package orm

import (
	"context"

	"github.com/dionisius77/dorm/schema"
)

// ExecutionStatus describes what happened at the terminal execution step.
type ExecutionStatus string

const (
	// ExecutionStatusSkipped indicates that SQL execution was intentionally skipped.
	ExecutionStatusSkipped ExecutionStatus = "Skipped"
)

// ExecutionReport captures the observable behavior of an ORM operation.
type ExecutionReport struct {
	Version           int
	Operation         string
	Table             string
	SQL               string
	Parameters        []any
	Statements        []ExecutionStatement
	AccessPolicies    []AccessPolicyEvent
	AuditActions      []AuditAction
	LifecycleHooks    []LifecycleHookEvent
	QueryAdvisor      []QueryAdvisorFinding
	OptimisticLocking *OptimisticLockingInfo
	ExecutionStatus   ExecutionStatus
	Metadata          map[string]any
}

// ExecutionStatement captures a single generated SQL statement.
type ExecutionStatement struct {
	Operation  string
	SQL        string
	Parameters []any
}

// AccessPolicyEventKind classifies an access-policy action in the execution report.
type AccessPolicyEventKind string

const (
	AccessPolicyEventInjectedPredicate AccessPolicyEventKind = "injected_predicate"
	AccessPolicyEventInjectedField     AccessPolicyEventKind = "injected_field"
	AccessPolicyEventInheritedPolicy   AccessPolicyEventKind = "inherited_policy"
	AccessPolicyEventPolicyOverride    AccessPolicyEventKind = "policy_override"
	AccessPolicyEventSoftDelete        AccessPolicyEventKind = "soft_delete"
)

// AccessPolicyEvent describes a security-related policy action.
type AccessPolicyEvent struct {
	Kind        AccessPolicyEventKind
	Field       string
	SQL         string
	Arguments   []any
	Policy      string
	Description string
}

// AuditAction describes an automatic audit field assignment.
type AuditAction struct {
	Field       string
	Value       any
	Kind        string
	Description string
}

// LifecycleHookEvent describes a lifecycle hook invocation.
type LifecycleHookEvent struct {
	Order int
	Name  string
	Model string
}

// QueryAdvisorFinding describes a recommendation from the query advisor.
type QueryAdvisorFinding struct {
	Code           string
	Rule           string
	Severity       string
	Title          string
	Details        string
	Recommendation string
	Table          string
	Columns        []string
}

// QueryAdvisorInput describes a query advisor request.
type QueryAdvisorInput struct {
	Operation string
	Table     string
	SQL       string
	Schema    *schema.Schema
}

// QueryAdvisorReport captures query advisor output in a neutral form.
type QueryAdvisorReport struct {
	Table    string
	SQL      string
	Kind     string
	Findings []QueryAdvisorFinding
}

// OptimisticLockingInfo describes optimistic-lock metadata for inspection and tracing.
type OptimisticLockingInfo struct {
	Enabled  bool
	Column   string
	Current  any
	Next     any
	Conflict bool
}

// QueryAdvisor evaluates a generated statement and returns advisory findings.
type QueryAdvisor interface {
	Inspect(context.Context, QueryAdvisorInput) (QueryAdvisorReport, error)
}
