package schema

import (
	"reflect"
	"strings"
	"sync"
)

type CustomTypeResolver func(reflect.Type) (Type, bool)

var (
	customTypeMu        sync.RWMutex
	customTypeByName    = map[string]Type{}
	customTypeResolvers []CustomTypeResolver
)

func RegisterCustomType(name string, typ Type) {
	name = normalizeCustomTypeName(name)
	if name == "" {
		return
	}
	customTypeMu.Lock()
	defer customTypeMu.Unlock()
	customTypeByName[name] = typ
}

func RegisterCustomTypeResolver(resolver CustomTypeResolver) {
	if resolver == nil {
		return
	}
	customTypeMu.Lock()
	defer customTypeMu.Unlock()
	customTypeResolvers = append(customTypeResolvers, resolver)
}

func ResolveCustomType(t reflect.Type) (Type, bool) {
	if t == nil {
		return Type{}, false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	keys := []string{normalizeCustomTypeName(t.Name())}
	if pkg := strings.TrimSpace(t.PkgPath()); pkg != "" && t.Name() != "" {
		keys = append(keys, normalizeCustomTypeName(pkg+"."+t.Name()))
	}

	customTypeMu.RLock()
	defer customTypeMu.RUnlock()
	for _, key := range keys {
		if typ, ok := customTypeByName[key]; ok {
			return typ, true
		}
	}
	for _, resolver := range customTypeResolvers {
		if typ, ok := resolver(t); ok {
			return typ, true
		}
	}
	return Type{}, false
}

func normalizeCustomTypeName(name string) string {
	return strings.TrimSpace(strings.ToLower(name))
}
