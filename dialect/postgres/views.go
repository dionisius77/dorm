package postgres

import (
	"fmt"
	"strings"

	"github.com/dionisius77/dorm/schema"
)

func (d Dialect) RenderView(view *schema.View) (string, error) {
	if view == nil {
		return "", fmt.Errorf("postgres: nil view")
	}
	if view.Name == "" {
		return "", fmt.Errorf("postgres: view missing name")
	}
	sqlText := strings.TrimSpace(view.SQL)
	if sqlText == "" {
		return "", fmt.Errorf("postgres: view %s missing SQL", view.Name)
	}

	var b strings.Builder
	b.WriteString("CREATE ")
	if view.Materialized {
		b.WriteString("MATERIALIZED ")
	}
	b.WriteString("VIEW IF NOT EXISTS ")
	b.WriteString(d.QuoteIdent(view.Name))
	b.WriteString(" AS ")
	b.WriteString(sqlText)
	if !strings.HasSuffix(sqlText, ";") {
		b.WriteString(";")
	}
	return b.String(), nil
}

func (d Dialect) RenderDropView(view *schema.View) (string, error) {
	if view == nil {
		return "", fmt.Errorf("postgres: nil view")
	}
	if view.Name == "" {
		return "", fmt.Errorf("postgres: view missing name")
	}
	var b strings.Builder
	b.WriteString("DROP ")
	if view.Materialized {
		b.WriteString("MATERIALIZED ")
	}
	b.WriteString("VIEW IF EXISTS ")
	b.WriteString(d.QuoteIdent(view.Name))
	b.WriteString(";")
	return b.String(), nil
}

func (d Dialect) RenderExpression(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("postgres: empty expression")
	}
	return expr, nil
}

func (d Dialect) RenderPredicate(left, operator string, right string) (string, error) {
	left = strings.TrimSpace(left)
	operator = strings.TrimSpace(operator)
	right = strings.TrimSpace(right)
	if left == "" || operator == "" || right == "" {
		return "", fmt.Errorf("postgres: predicate requires left, operator, and right")
	}
	return left + " " + operator + " " + right, nil
}

func (d Dialect) RenderDefault(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("postgres: empty default expression")
	}
	return expr, nil
}
