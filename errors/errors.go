package errors

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Kind classifies shared ORM errors.
type Kind string

const (
	KindNotFound             Kind = "not_found"
	KindAlreadyExists        Kind = "already_exists"
	KindConflict             Kind = "conflict"
	KindInvalidModel         Kind = "invalid_model"
	KindInvalidRelationship  Kind = "invalid_relationship"
	KindMigrationRequired    Kind = "migration_required"
	KindSchemaDrift          Kind = "schema_drift"
	KindInvalidContext       Kind = "invalid_context"
	KindMissingCompany       Kind = "missing_company"
	KindPolicyDenied         Kind = "policy_denied"
	KindSeedConflict         Kind = "seed_conflict"
	KindDriverNotRegistered  Kind = "driver_not_registered"
	KindUnsupportedDialect   Kind = "unsupported_dialect"
	KindTransactionClosed    Kind = "transaction_closed"
	KindCommitFailed         Kind = "commit_failed"
	KindRollbackFailed       Kind = "rollback_failed"
	KindOptimisticLockFailed Kind = "optimistic_lock_failed"

	// Compatibility kinds retained for the existing codebase.
	KindConfiguration        Kind = "configuration"
	KindInvalidSchema        Kind = "invalid_schema"
	KindUnsupportedFeature   Kind = "unsupported_feature"
	KindMigrationGeneration  Kind = "migration_generation"
	KindMigrationApplication Kind = "migration_application"
	KindRuntimeQuery         Kind = "runtime_query"
	KindAccessViolation      Kind = "access_violation"
)

// Error is the shared classified error wrapper.
type Error struct {
	Kind    Kind
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Message != "" && e.Err != nil:
		return e.Message + ": " + e.Err.Error()
	case e.Message != "":
		return e.Message
	case e.Err != nil:
		return string(e.Kind) + ": " + e.Err.Error()
	default:
		return string(e.Kind)
	}
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e != nil && t != nil && e.Kind == t.Kind
}

// New creates a classified error.
func New(kind Kind, msg string) error {
	return &Error{Kind: kind, Message: msg}
}

// Wrap creates a classified error with a wrapped cause.
func Wrap(kind Kind, msg string, err error) error {
	if err == nil {
		return &Error{Kind: kind, Message: msg}
	}
	return &Error{Kind: kind, Message: msg, Err: err}
}

// Is reports whether err matches target.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As reports whether err can be assigned to target.
func As(err error, target any) bool {
	return errors.As(err, target)
}

func sentinel(kind Kind) *Error {
	return &Error{Kind: kind}
}

var (
	ErrNotFound             = sentinel(KindNotFound)
	ErrAlreadyExists        = sentinel(KindAlreadyExists)
	ErrConflict             = sentinel(KindConflict)
	ErrInvalidModel         = sentinel(KindInvalidModel)
	ErrInvalidRelationship  = sentinel(KindInvalidRelationship)
	ErrMigrationRequired    = sentinel(KindMigrationRequired)
	ErrSchemaDrift          = sentinel(KindSchemaDrift)
	ErrInvalidContext       = sentinel(KindInvalidContext)
	ErrMissingCompany       = sentinel(KindMissingCompany)
	ErrPolicyDenied         = sentinel(KindPolicyDenied)
	ErrSeedConflict         = sentinel(KindSeedConflict)
	ErrDriverNotRegistered  = sentinel(KindDriverNotRegistered)
	ErrUnsupportedDialect   = sentinel(KindUnsupportedDialect)
	ErrTransactionClosed    = sentinel(KindTransactionClosed)
	ErrCommitFailed         = sentinel(KindCommitFailed)
	ErrRollbackFailed       = sentinel(KindRollbackFailed)
	ErrOptimisticLockFailed = sentinel(KindOptimisticLockFailed)

	// Compatibility sentinels used by the existing codebase.
	ErrConfiguration        = sentinel(KindConfiguration)
	ErrInvalidSchema        = sentinel(KindInvalidSchema)
	ErrUnsupportedFeature   = sentinel(KindUnsupportedFeature)
	ErrMigrationGeneration  = sentinel(KindMigrationGeneration)
	ErrMigrationApplication = sentinel(KindMigrationApplication)
	ErrRuntimeQuery         = sentinel(KindRuntimeQuery)
	ErrAccessViolation      = sentinel(KindAccessViolation)
)

// ValidationError describes invalid input or model state.
type ValidationError struct {
	Base   *Error
	Model  string
	Field  string
	Reason string
}

func NewValidationError(model, field, reason string, err error) error {
	return &ValidationError{
		Base:   &Error{Kind: KindInvalidModel, Message: buildValidationMessage(model, field, reason), Err: err},
		Model:  model,
		Field:  field,
		Reason: reason,
	}
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Base != nil && e.Base.Message != "" {
		return e.Base.Error()
	}
	return buildValidationMessage(e.Model, e.Field, e.Reason)
}

func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Base
}

// SchemaError describes expected/actual/difference mismatches.
type SchemaError struct {
	Base     *Error
	Expected string
	Actual   string
	Diff     string
}

func NewSchemaError(kind Kind, expected, actual, diff string, err error) error {
	if kind == "" {
		kind = KindSchemaDrift
	}
	return &SchemaError{
		Base:     &Error{Kind: kind, Message: buildSchemaMessage(expected, actual, diff), Err: err},
		Expected: expected,
		Actual:   actual,
		Diff:     diff,
	}
}

func (e *SchemaError) Error() string {
	if e == nil {
		return ""
	}
	if e.Base != nil && e.Base.Message != "" {
		return e.Base.Error()
	}
	return buildSchemaMessage(e.Expected, e.Actual, e.Diff)
}

func (e *SchemaError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Base
}

// MigrationError describes migration generation or application failures.
type MigrationError struct {
	Base      *Error
	File      string
	Statement string
	Line      int
}

func NewMigrationError(kind Kind, file, statement string, line int, err error) error {
	if kind == "" {
		kind = KindMigrationApplication
	}
	return &MigrationError{
		Base:      &Error{Kind: kind, Message: buildMigrationMessage(file, statement, line), Err: err},
		File:      file,
		Statement: statement,
		Line:      line,
	}
}

func (e *MigrationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Base != nil && e.Base.Message != "" {
		return e.Base.Error()
	}
	return buildMigrationMessage(e.File, e.Statement, e.Line)
}

func (e *MigrationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Base
}

// DriverError describes connection-layer failures.
type DriverError struct {
	Base      *Error
	Driver    string
	Operation string
}

func NewDriverError(kind Kind, driver, operation string, err error) error {
	if kind == "" {
		kind = KindConfiguration
	}
	return &DriverError{
		Base:      &Error{Kind: kind, Message: buildDriverMessage(driver, operation), Err: err},
		Driver:    driver,
		Operation: operation,
	}
}

func (e *DriverError) Error() string {
	if e == nil {
		return ""
	}
	if e.Base != nil && e.Base.Message != "" {
		return e.Base.Error()
	}
	return buildDriverMessage(e.Driver, e.Operation)
}

func (e *DriverError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Base
}

// AccessError describes access policy failures.
type AccessError struct {
	Base      *Error
	Operation string
	Policy    string
	Field     string
	Reason    string
}

func NewAccessError(kind Kind, operation, policy, field, reason string, err error) error {
	if kind == "" {
		kind = KindPolicyDenied
	}
	return &AccessError{
		Base:      &Error{Kind: kind, Message: buildAccessMessage(operation, policy, field, reason), Err: err},
		Operation: operation,
		Policy:    policy,
		Field:     field,
		Reason:    reason,
	}
}

func (e *AccessError) Error() string {
	if e == nil {
		return ""
	}
	if e.Base != nil && e.Base.Message != "" {
		return e.Base.Error()
	}
	return buildAccessMessage(e.Operation, e.Policy, e.Field, e.Reason)
}

func (e *AccessError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Base
}

// SeedError describes synchronization conflicts.
type SeedError struct {
	Base     *Error
	Model    string
	Key      string
	Conflict map[string]any
}

// TransactionError describes begin/commit/rollback failures.
type TransactionError struct {
	Base      *Error
	Operation string
	Savepoint string
}

func NewTransactionError(kind Kind, operation, savepoint string, err error) error {
	if kind == "" {
		kind = KindTransactionClosed
	}
	return &TransactionError{
		Base:      &Error{Kind: kind, Message: buildTransactionMessage(operation, savepoint), Err: err},
		Operation: operation,
		Savepoint: savepoint,
	}
}

func (e *TransactionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Base != nil && e.Base.Message != "" {
		return e.Base.Error()
	}
	return buildTransactionMessage(e.Operation, e.Savepoint)
}

func (e *TransactionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Base
}

func NewSeedError(kind Kind, model, key string, conflict map[string]any, err error) error {
	if kind == "" {
		kind = KindSeedConflict
	}
	return &SeedError{
		Base:     &Error{Kind: kind, Message: buildSeedMessage(model, key, conflict), Err: err},
		Model:    model,
		Key:      key,
		Conflict: cloneAnyMap(conflict),
	}
}

func (e *SeedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Base != nil && e.Base.Message != "" {
		return e.Base.Error()
	}
	return buildSeedMessage(e.Model, e.Key, e.Conflict)
}

func (e *SeedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Base
}

func buildValidationMessage(model, field, reason string) string {
	parts := []string{"validation failed"}
	if model != "" {
		parts = append(parts, model)
	}
	if field != "" {
		parts = append(parts, field)
	}
	if reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, ": ")
}

func buildSchemaMessage(expected, actual, diff string) string {
	parts := []string{"schema error"}
	if expected != "" {
		parts = append(parts, "expected="+expected)
	}
	if actual != "" {
		parts = append(parts, "actual="+actual)
	}
	if diff != "" {
		parts = append(parts, "diff="+diff)
	}
	return strings.Join(parts, ": ")
}

func buildMigrationMessage(file, statement string, line int) string {
	parts := []string{"migration error"}
	if file != "" {
		parts = append(parts, "file="+file)
	}
	if line > 0 {
		parts = append(parts, fmt.Sprintf("line=%d", line))
	}
	if statement != "" {
		parts = append(parts, "statement="+statement)
	}
	return strings.Join(parts, ": ")
}

func buildDriverMessage(driver, operation string) string {
	parts := []string{"driver error"}
	if driver != "" {
		parts = append(parts, "driver="+driver)
	}
	if operation != "" {
		parts = append(parts, "operation="+operation)
	}
	return strings.Join(parts, ": ")
}

func buildAccessMessage(operation, policy, field, reason string) string {
	parts := []string{"access error"}
	if operation != "" {
		parts = append(parts, "operation="+operation)
	}
	if policy != "" {
		parts = append(parts, "policy="+policy)
	}
	if field != "" {
		parts = append(parts, "field="+field)
	}
	if reason != "" {
		parts = append(parts, "reason="+reason)
	}
	return strings.Join(parts, ": ")
}

func buildSeedMessage(model, key string, conflict map[string]any) string {
	parts := []string{"seed conflict"}
	if model != "" {
		parts = append(parts, "model="+model)
	}
	if key != "" {
		parts = append(parts, "key="+key)
	}
	if len(conflict) > 0 {
		keys := make([]string, 0, len(conflict))
		for k := range conflict {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var values []string
		for _, k := range keys {
			values = append(values, fmt.Sprintf("%s=%v", k, conflict[k]))
		}
		parts = append(parts, strings.Join(values, ","))
	}
	return strings.Join(parts, ": ")
}

func buildTransactionMessage(operation, savepoint string) string {
	parts := []string{"transaction error"}
	if operation != "" {
		parts = append(parts, "operation="+operation)
	}
	if savepoint != "" {
		parts = append(parts, "savepoint="+savepoint)
	}
	return strings.Join(parts, ": ")
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
