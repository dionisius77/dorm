package errkind

import "errors"

type Kind string

const (
	KindConfiguration        Kind = "configuration"
	KindInvalidSchema        Kind = "invalid_schema"
	KindUnsupportedFeature   Kind = "unsupported_feature"
	KindMigrationGeneration  Kind = "migration_generation"
	KindMigrationApplication Kind = "migration_application"
	KindRuntimeQuery         Kind = "runtime_query"
	KindAccessViolation      Kind = "access_violation"
)

type Error struct {
	Kind    Kind
	Message string
	Err     error
}

var (
	ErrConfiguration        = &Error{Kind: KindConfiguration}
	ErrInvalidSchema        = &Error{Kind: KindInvalidSchema}
	ErrUnsupportedFeature   = &Error{Kind: KindUnsupportedFeature}
	ErrMigrationGeneration  = &Error{Kind: KindMigrationGeneration}
	ErrMigrationApplication = &Error{Kind: KindMigrationApplication}
	ErrRuntimeQuery         = &Error{Kind: KindRuntimeQuery}
	ErrAccessViolation      = &Error{Kind: KindAccessViolation}
)

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

func New(kind Kind, msg string) error {
	return &Error{Kind: kind, Message: msg}
}

func Wrap(kind Kind, msg string, err error) error {
	if err == nil {
		return &Error{Kind: kind, Message: msg}
	}
	return &Error{Kind: kind, Message: msg, Err: err}
}

func Is(err error, kind Kind) bool {
	return errors.Is(err, &Error{Kind: kind})
}
