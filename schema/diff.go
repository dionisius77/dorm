package schema

import (
	"fmt"
	"sort"

	dormerrors "github.com/dionisius77/dorm/errors"
)

type OperationKind string

const (
	OpCreateTable      OperationKind = "create_table"
	OpDropTable        OperationKind = "drop_table"
	OpAddColumn        OperationKind = "add_column"
	OpDropColumn       OperationKind = "drop_column"
	OpAlterColumn      OperationKind = "alter_column"
	OpCreateIndex      OperationKind = "create_index"
	OpDropIndex        OperationKind = "drop_index"
	OpCreateConstraint OperationKind = "create_constraint"
	OpDropConstraint   OperationKind = "drop_constraint"
)

type Operation struct {
	Kind       OperationKind
	Table      string
	TableDef   *Table
	Column     *Column
	Previous   *Column
	Index      *Index
	Constraint *Constraint
}

type Diff struct {
	Operations []Operation
}

func (d Diff) Empty() bool { return len(d.Operations) == 0 }

func Compare(expected, actual *Schema) (*Diff, error) {
	if expected == nil || actual == nil {
		return nil, dormerrors.NewSchemaError(dormerrors.KindInvalidSchema, schemaLabel(expected), schemaLabel(actual), "compare requires non-nil schemas", nil)
	}
	expected = expected.Clone()
	actual = actual.Clone()
	expected.Sort()
	actual.Sort()
	diff := &Diff{}
	expTables := tableMap(expected.Tables)
	actTables := tableMap(actual.Tables)

	expNames := sortedKeys(expTables)
	for _, name := range expNames {
		expT := expTables[name]
		actT, ok := actTables[name]
		if !ok {
			diff.Operations = append(diff.Operations, Operation{Kind: OpCreateTable, Table: name, TableDef: expT})
			continue
		}
		diff.Operations = append(diff.Operations, compareTable(expT, actT)...)
	}
	for _, name := range sortedKeys(actTables) {
		if _, ok := expTables[name]; !ok {
			diff.Operations = append(diff.Operations, Operation{Kind: OpDropTable, Table: name, TableDef: actTables[name]})
		}
	}
	sort.SliceStable(diff.Operations, func(i, j int) bool {
		if diff.Operations[i].Table == diff.Operations[j].Table {
			return diff.Operations[i].Kind < diff.Operations[j].Kind
		}
		return diff.Operations[i].Table < diff.Operations[j].Table
	})
	return diff, nil
}

func schemaLabel(s *Schema) string {
	if s == nil {
		return "nil"
	}
	if s.Name != "" {
		return s.Name
	}
	return "unknown"
}

func compareTable(expected, actual *Table) []Operation {
	var out []Operation
	expCols := columnMap(expected.Columns)
	actCols := columnMap(actual.Columns)
	for _, name := range sortedKeys(expCols) {
		exp := expCols[name]
		act, ok := actCols[name]
		if !ok {
			out = append(out, Operation{Kind: OpAddColumn, Table: expected.Name, Column: exp})
			continue
		}
		if !columnsEqual(exp, act) {
			out = append(out, Operation{Kind: OpAlterColumn, Table: expected.Name, Column: exp, Previous: act})
		}
	}
	for _, name := range sortedKeys(actCols) {
		if _, ok := expCols[name]; !ok {
			out = append(out, Operation{Kind: OpDropColumn, Table: expected.Name, Column: actCols[name]})
		}
	}

	expIndexes := indexMap(expected.Indexes)
	actIndexes := indexMap(actual.Indexes)
	for _, name := range sortedKeys(expIndexes) {
		exp := expIndexes[name]
		act, ok := actIndexes[name]
		if !ok || !indexesEqual(exp, act) {
			out = append(out, Operation{Kind: OpCreateIndex, Table: expected.Name, Index: exp})
		}
	}
	for _, name := range sortedKeys(actIndexes) {
		if _, ok := expIndexes[name]; !ok {
			out = append(out, Operation{Kind: OpDropIndex, Table: expected.Name, Index: actIndexes[name]})
		}
	}

	expConstraints := constraintMap(expected.Constraints)
	actConstraints := constraintMap(actual.Constraints)
	for _, name := range sortedKeys(expConstraints) {
		exp := expConstraints[name]
		act, ok := actConstraints[name]
		if !ok || !constraintsEqual(exp, act) {
			out = append(out, Operation{Kind: OpCreateConstraint, Table: expected.Name, Constraint: exp})
		}
	}
	for _, name := range sortedKeys(actConstraints) {
		if _, ok := expConstraints[name]; !ok {
			out = append(out, Operation{Kind: OpDropConstraint, Table: expected.Name, Constraint: actConstraints[name]})
		}
	}
	return out
}

func columnsEqual(a, b *Column) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Type.Name == b.Type.Name &&
		a.Type.Kind == b.Type.Kind &&
		a.Type.Nullable == b.Type.Nullable &&
		a.Nullable == b.Nullable &&
		a.PrimaryKey == b.PrimaryKey &&
		a.Unique == b.Unique &&
		a.Identity == b.Identity &&
		a.Default == b.Default &&
		a.Generated == b.Generated
}

func indexesEqual(a, b *Index) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Unique == b.Unique &&
		a.Expression == b.Expression &&
		a.Where == b.Where &&
		a.Method == b.Method &&
		joinSorted(a.Columns) == joinSorted(b.Columns)
}

func constraintsEqual(a, b *Constraint) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Kind == b.Kind &&
		joinSorted(a.Columns) == joinSorted(b.Columns) &&
		joinSorted(a.ReferencedColumns) == joinSorted(b.ReferencedColumns) &&
		a.ReferencedTable == b.ReferencedTable &&
		a.OnDelete == b.OnDelete &&
		a.OnUpdate == b.OnUpdate &&
		a.Expression == b.Expression
}

func tableMap(tables []*Table) map[string]*Table {
	out := make(map[string]*Table, len(tables))
	for _, t := range tables {
		out[t.Name] = t
	}
	return out
}

func columnMap(cols []*Column) map[string]*Column {
	out := make(map[string]*Column, len(cols))
	for _, c := range cols {
		out[c.Name] = c
	}
	return out
}

func indexMap(cols []*Index) map[string]*Index {
	out := make(map[string]*Index, len(cols))
	for _, i := range cols {
		out[i.Name] = i
	}
	return out
}

func constraintMap(cols []*Constraint) map[string]*Constraint {
	out := make(map[string]*Constraint, len(cols))
	for _, c := range cols {
		out[c.Name] = c
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func joinSorted(vals []string) string {
	cp := append([]string(nil), vals...)
	sort.Strings(cp)
	return fmt.Sprint(cp)
}

func (d Diff) String() string {
	var parts []string
	for _, op := range d.Operations {
		parts = append(parts, string(op.Kind)+":"+op.Table)
	}
	return fmt.Sprint(parts)
}
