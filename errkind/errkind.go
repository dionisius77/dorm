package errkind

import dormerrors "github.com/dionisius77/dorm/errors"

type Kind = dormerrors.Kind
type Error = dormerrors.Error

const (
	KindConfiguration        = dormerrors.KindConfiguration
	KindInvalidSchema        = dormerrors.KindInvalidSchema
	KindUnsupportedFeature   = dormerrors.KindUnsupportedFeature
	KindMigrationGeneration  = dormerrors.KindMigrationGeneration
	KindMigrationApplication = dormerrors.KindMigrationApplication
	KindRuntimeQuery         = dormerrors.KindRuntimeQuery
	KindAccessViolation      = dormerrors.KindAccessViolation
	KindRawSQLPolicyRequired = dormerrors.KindRawSQLPolicyRequired
)

var (
	ErrConfiguration        = dormerrors.ErrConfiguration
	ErrInvalidSchema        = dormerrors.ErrInvalidSchema
	ErrUnsupportedFeature   = dormerrors.ErrUnsupportedFeature
	ErrMigrationGeneration  = dormerrors.ErrMigrationGeneration
	ErrMigrationApplication = dormerrors.ErrMigrationApplication
	ErrRuntimeQuery         = dormerrors.ErrRuntimeQuery
	ErrAccessViolation      = dormerrors.ErrAccessViolation
	ErrRawSQLPolicyRequired = dormerrors.ErrRawSQLPolicyRequired
)

func New(kind Kind, msg string) error {
	return dormerrors.New(kind, msg)
}

func Wrap(kind Kind, msg string, err error) error {
	return dormerrors.Wrap(kind, msg, err)
}

func Is(err error, kind Kind) bool {
	return dormerrors.Is(err, &dormerrors.Error{Kind: kind})
}
