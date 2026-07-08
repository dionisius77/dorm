package access

import "dorm/schema"

func SoftDeleteColumn(table *schema.Table) *schema.Column {
	if table == nil {
		return nil
	}
	for _, col := range table.Columns {
		if col.SoftDelete || col.DeletedAt {
			return col
		}
	}
	return nil
}
