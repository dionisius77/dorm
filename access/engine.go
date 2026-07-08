package access

import (
	"context"
	"fmt"

	"github.com/dionisius77/dorm/schema"
)

type Operation string

const (
	OpQuery  Operation = "query"
	OpInsert Operation = "insert"
	OpUpdate Operation = "update"
	OpDelete Operation = "delete"
)

type Engine struct{}

func NewEngine() Engine { return Engine{} }

type Predicate struct {
	SQL  string
	Args []any
}

type FieldValue struct {
	Field string
	Value any
}

func (Engine) Apply(ctx context.Context, table *schema.Table, op Operation, values map[string]any) ([]Predicate, []FieldValue, error) {
	ac, _ := FromContext(ctx)
	var predicates []Predicate
	var writes []FieldValue
	if table == nil {
		return nil, nil, fmt.Errorf("access: nil table")
	}
	for _, col := range table.Columns {
		switch col.Scope {
		case schema.ScopeCompany:
			if ac.CompanyID == nil {
				continue
			}
			if op == OpQuery {
				predicates = append(predicates, Predicate{SQL: col.Name + " = ?", Args: []any{ac.CompanyID}})
			}
			if op == OpInsert {
				writes = append(writes, FieldValue{Field: col.Name, Value: ac.CompanyID})
			}
		case schema.ScopeTenant:
			if ac.TenantID == nil {
				continue
			}
			if op == OpQuery {
				predicates = append(predicates, Predicate{SQL: col.Name + " = ?", Args: []any{ac.TenantID}})
			}
			if op == OpInsert {
				writes = append(writes, FieldValue{Field: col.Name, Value: ac.TenantID})
			}
		case schema.ScopeOrganization:
			if ac.OrganizationID == nil {
				continue
			}
			if op == OpQuery {
				predicates = append(predicates, Predicate{SQL: col.Name + " = ?", Args: []any{ac.OrganizationID}})
			}
			if op == OpInsert {
				writes = append(writes, FieldValue{Field: col.Name, Value: ac.OrganizationID})
			}
		case schema.ScopeWorkspace:
			if ac.WorkspaceID == nil {
				continue
			}
			if op == OpQuery {
				predicates = append(predicates, Predicate{SQL: col.Name + " = ?", Args: []any{ac.WorkspaceID}})
			}
			if op == OpInsert {
				writes = append(writes, FieldValue{Field: col.Name, Value: ac.WorkspaceID})
			}
		case schema.ScopeWarehouse:
			if ac.WarehouseID == nil {
				continue
			}
			if op == OpQuery {
				predicates = append(predicates, Predicate{SQL: col.Name + " = ?", Args: []any{ac.WarehouseID}})
			}
			if op == OpInsert {
				writes = append(writes, FieldValue{Field: col.Name, Value: ac.WarehouseID})
			}
		case schema.ScopeUser:
			if ac.UserID == nil {
				continue
			}
			if op == OpQuery {
				predicates = append(predicates, Predicate{SQL: col.Name + " = ?", Args: []any{ac.UserID}})
			}
			if op == OpInsert {
				writes = append(writes, FieldValue{Field: col.Name, Value: ac.UserID})
			}
		}

		if op == OpInsert {
			switch {
			case col.CreatedBy:
				if ac.UserID != nil {
					writes = append(writes, FieldValue{Field: col.Name, Value: ac.UserID})
				}
			case col.UpdatedBy:
				if ac.UserID != nil {
					writes = append(writes, FieldValue{Field: col.Name, Value: ac.UserID})
				}
			case col.DeletedBy:
				if ac.UserID != nil {
					writes = append(writes, FieldValue{Field: col.Name, Value: ac.UserID})
				}
			}
		}
		if op == OpUpdate {
			switch {
			case col.UpdatedBy:
				if ac.UserID != nil {
					writes = append(writes, FieldValue{Field: col.Name, Value: ac.UserID})
				}
			}
		}
	}
	return predicates, writes, nil
}
