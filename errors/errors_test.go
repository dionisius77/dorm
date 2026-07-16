package errors_test

import (
	"errors"
	"testing"

	dormerrors "github.com/dionisius77/dorm/errors"
)

func TestSentinelIs(t *testing.T) {
	err := dormerrors.Wrap(dormerrors.KindInvalidModel, "validate model", errors.New("boom"))
	if !errors.Is(err, dormerrors.ErrInvalidModel) {
		t.Fatalf("expected invalid model sentinel, got %T %v", err, err)
	}
}

func TestTypedAccessErrorSupportsAs(t *testing.T) {
	err := dormerrors.NewAccessError(dormerrors.KindMissingCompany, "query", "default", "CompanyID", "company context required", errors.New("boom"))
	var accessErr *dormerrors.AccessError
	if !errors.As(err, &accessErr) {
		t.Fatalf("expected access error, got %T %v", err, err)
	}
	if accessErr == nil || accessErr.Field != "CompanyID" {
		t.Fatalf("unexpected access error: %#v", accessErr)
	}
	if !errors.Is(err, dormerrors.ErrMissingCompany) {
		t.Fatalf("expected missing company sentinel, got %T %v", err, err)
	}
}

func TestTypedSchemaErrorSupportsAs(t *testing.T) {
	err := dormerrors.NewSchemaError(dormerrors.KindSchemaDrift, "expected", "actual", "diff", errors.New("boom"))
	var schemaErr *dormerrors.SchemaError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("expected schema error, got %T %v", err, err)
	}
	if schemaErr == nil || schemaErr.Diff != "diff" {
		t.Fatalf("unexpected schema error: %#v", schemaErr)
	}
	if !errors.Is(err, dormerrors.ErrSchemaDrift) {
		t.Fatalf("expected schema drift sentinel, got %T %v", err, err)
	}
}
