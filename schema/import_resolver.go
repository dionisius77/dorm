package schema

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type embeddedTypeRef struct {
	Package string
	Name    string
}

type structTypeResolver struct {
	moduleRoot string
	modulePath string
	cache      sync.Map // map[string]map[string]*ast.StructType
}

func newStructTypeResolver(sourceRoot string) (*structTypeResolver, error) {
	root, modulePath, err := moduleSourceInfo(sourceRoot)
	if err != nil {
		root, modulePath, err = moduleSourceInfo("")
		if err != nil {
			return nil, err
		}
	}
	return &structTypeResolver{
		moduleRoot: root,
		modulePath: modulePath,
	}, nil
}

func moduleSourceInfo(startDir string) (string, string, error) {
	dir := startDir
	if dir == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			return "", "", errors.New("schema: unable to determine module source path")
		}
		dir = filepath.Dir(file)
	} else {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(gomod)
		if err == nil {
			modulePath := parseModulePath(data)
			if modulePath == "" {
				return "", "", errors.New("schema: unable to read module path")
			}
			return dir, modulePath, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", errors.New("schema: unable to locate go.mod")
}

func parseModulePath(data []byte) string {
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
		if line != "" && !strings.HasPrefix(line, "//") {
			break
		}
	}
	return ""
}

func parseEmbeddedTypeRef(expr ast.Expr) (embeddedTypeRef, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		return embeddedTypeRef{Name: t.Name}, true
	case *ast.StarExpr:
		return parseEmbeddedTypeRef(t.X)
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			return embeddedTypeRef{Package: pkg.Name, Name: t.Sel.Name}, true
		}
		return embeddedTypeRef{Name: t.Sel.Name}, true
	default:
		return embeddedTypeRef{}, false
	}
}

func collectFileImportMap(file *ast.File) map[string]string {
	out := map[string]string{}
	if file == nil {
		return out
	}
	for _, spec := range file.Imports {
		if spec == nil || spec.Path == nil {
			continue
		}
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath == "" {
			continue
		}
		alias := ""
		if spec.Name != nil {
			switch spec.Name.Name {
			case "_", ".":
				continue
			default:
				alias = spec.Name.Name
			}
		} else {
			alias = path.Base(importPath)
		}
		if alias == "" {
			continue
		}
		out[alias] = importPath
	}
	return out
}

func resolveEmbeddedStruct(ref embeddedTypeRef, local map[string]*ast.StructType, imports map[string]string, resolver *structTypeResolver) (*ast.StructType, map[string]*ast.StructType, bool) {
	if ref.Name == "" {
		return nil, nil, false
	}
	if ref.Package == "" {
		st, ok := local[ref.Name]
		return st, local, ok
	}
	importPath := ""
	if imports != nil {
		importPath = imports[ref.Package]
	}
	if importPath == "" || resolver == nil {
		return nil, nil, false
	}
	types, err := resolver.loadPackageStructTypes(importPath)
	if err != nil {
		return nil, nil, false
	}
	st, ok := types[ref.Name]
	return st, types, ok
}

func (r *structTypeResolver) loadPackageStructTypes(importPath string) (map[string]*ast.StructType, error) {
	if r == nil {
		return nil, errors.New("schema: nil resolver")
	}
	if cached, ok := r.cache.Load(importPath); ok {
		if types, ok := cached.(map[string]*ast.StructType); ok {
			return types, nil
		}
	}

	dir, err := r.resolveImportDir(importPath)
	if err != nil {
		return nil, err
	}

	types, err := r.parseStructTypes(dir)
	if err != nil {
		return nil, err
	}
	r.cache.Store(importPath, types)
	return types, nil
}

func (r *structTypeResolver) resolveImportDir(importPath string) (string, error) {
	if strings.HasPrefix(importPath, r.modulePath) {
		rel := strings.TrimPrefix(importPath, r.modulePath)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return r.moduleRoot, nil
		}
		return filepath.Join(r.moduleRoot, filepath.FromSlash(rel)), nil
	}

	cmd := exec.Command("go", "list", "-f", "{{.Dir}}", importPath)
	cmd.Dir = r.moduleRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	dir := strings.TrimSpace(string(bytes.TrimSpace(out)))
	if dir == "" {
		return "", errors.New("schema: go list returned empty package dir")
	}
	return dir, nil
}

func (r *structTypeResolver) parseStructTypes(dir string) (map[string]*ast.StructType, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	types := map[string]*ast.StructType{}
	for _, pkg := range pkgs {
		for name, st := range collectPackageStructTypes(pkg) {
			types[name] = st
		}
	}
	return types, nil
}
