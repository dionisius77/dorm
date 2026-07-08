package schema

import (
	"strings"
	"unicode"
)

func ToSnakeCase(in string) string {
	if in == "" {
		return ""
	}
	var out strings.Builder
	var prevLower bool
	for i, r := range in {
		if unicode.IsUpper(r) {
			if i > 0 && prevLower {
				out.WriteByte('_')
			}
			out.WriteRune(unicode.ToLower(r))
			prevLower = false
			continue
		}
		if r == '-' || r == ' ' {
			out.WriteByte('_')
			prevLower = false
			continue
		}
		out.WriteRune(r)
		prevLower = unicode.IsLower(r) || unicode.IsDigit(r)
	}
	return out.String()
}

func Pluralize(name string) string {
	if name == "" {
		return ""
	}
	if strings.HasSuffix(name, "s") || strings.HasSuffix(name, "x") || strings.HasSuffix(name, "z") || strings.HasSuffix(name, "ch") || strings.HasSuffix(name, "sh") {
		return name + "es"
	}
	if strings.HasSuffix(name, "y") && len(name) > 1 {
		return name[:len(name)-1] + "ies"
	}
	return name + "s"
}

func QualifyPackage(pkgPath string) string {
	if pkgPath == "" {
		return ""
	}
	parts := strings.Split(pkgPath, "/")
	return parts[len(parts)-1]
}
