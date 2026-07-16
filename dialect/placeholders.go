package dialect

import "strings"

// BindPlaceholders rewrites generic positional placeholders into dialect-specific placeholders.
func BindPlaceholders(query string, d Dialect) string {
	if query == "" || d == nil {
		return query
	}
	var b strings.Builder
	b.Grow(len(query))
	next := 1
	for _, r := range query {
		if r == '?' {
			b.WriteString(d.Placeholder(next))
			next++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
