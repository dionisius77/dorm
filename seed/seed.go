package seed

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dionisius77/dorm"
	"github.com/dionisius77/dorm/access"
	"github.com/dionisius77/dorm/errkind"
	"github.com/dionisius77/dorm/orm"
	"github.com/dionisius77/dorm/schema"
)

// Seeder defines a unit of seed work that can be executed by the runner.
type Seeder interface {
	Name() string
	Run(context.Context, *orm.Session) error
}

// DependencyProvider is optional for seeders that depend on other seeders.
type DependencyProvider interface {
	Dependencies() []string
}

// SessionProvider is the minimal database session surface required by Sync.
type SessionProvider interface {
	WithContext(context.Context) *orm.Session
	WithPolicy(access.Policy) *orm.Session
}

// SyncOption configures record synchronization.
type SyncOption func(*syncConfig)

type syncConfig struct {
	keys []string
}

// Key marks a field used to identify a desired record.
func Key(name string) SyncOption {
	return func(cfg *syncConfig) {
		if cfg == nil {
			return
		}
		cfg.keys = append(cfg.keys, strings.TrimSpace(name))
	}
}

// RunOption configures seeder execution.
type RunOption func(*runConfig)

type runConfig struct {
	singleTransaction bool
}

// WithSingleTransaction wraps the full seed run in one transaction.
func WithSingleTransaction() RunOption {
	return func(cfg *runConfig) {
		if cfg != nil {
			cfg.singleTransaction = true
		}
	}
}

// WithPerSeederTransaction keeps the default per-seeder transaction mode explicit.
func WithPerSeederTransaction() RunOption {
	return func(cfg *runConfig) {
		if cfg != nil {
			cfg.singleTransaction = false
		}
	}
}

var registry = struct {
	mu      sync.RWMutex
	order   []string
	seeders map[string]Seeder
}{
	seeders: map[string]Seeder{},
}

// Register adds seeders to the global registry.
func Register(seed ...Seeder) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, s := range seed {
		if s == nil || strings.TrimSpace(s.Name()) == "" {
			continue
		}
		name := s.Name()
		if _, ok := registry.seeders[name]; !ok {
			registry.order = append(registry.order, name)
		}
		registry.seeders[name] = s
	}
}

// Reset clears the global registry.
func Reset() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.order = nil
	registry.seeders = map[string]Seeder{}
}

// Registered returns the registered seeders in registration order.
func Registered() []Seeder {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	out := make([]Seeder, 0, len(registry.order))
	for _, name := range registry.order {
		if s, ok := registry.seeders[name]; ok {
			out = append(out, s)
		}
	}
	return out
}

// List returns the registered seeder names.
func List() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return append([]string(nil), registry.order...)
}

// Run executes registered seeders in dependency order.
func Run(ctx context.Context, db *dorm.DB, opts ...RunOption) error {
	if db == nil {
		return errkind.New(errkind.KindConfiguration, "seed: nil db")
	}
	cfg := runConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	ordered, err := resolveSeederOrder()
	if err != nil {
		return err
	}
	if cfg.singleTransaction {
		return db.Tx(ctx, func(tx *orm.Session) error {
			for _, seeder := range ordered {
				if err := seeder.Run(ctx, tx); err != nil {
					return wrapSeederError(seeder.Name(), err)
				}
			}
			return nil
		})
	}
	for _, seeder := range ordered {
		if err := db.Tx(ctx, func(tx *orm.Session) error {
			return seeder.Run(ctx, tx)
		}); err != nil {
			return wrapSeederError(seeder.Name(), err)
		}
	}
	return nil
}

// Sync reconciles one record or a collection of records against the database.
func Sync(ctx context.Context, db SessionProvider, value any, opts ...SyncOption) error {
	if db == nil {
		return errkind.New(errkind.KindConfiguration, "seed: nil session provider")
	}
	cfg := syncConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	if len(cfg.keys) == 0 {
		return errkind.New(errkind.KindConfiguration, "seed: at least one key is required")
	}
	values, err := expandSeedValues(value)
	if err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	session := db.WithContext(ctx).WithPolicy(access.IgnoreRLS())
	for _, item := range values {
		if err := syncOne(ctx, session, item, cfg.keys); err != nil {
			return err
		}
	}
	return nil
}

func resolveSeederOrder() ([]Seeder, error) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if len(registry.order) == 0 {
		return nil, nil
	}
	nodes := make(map[string]Seeder, len(registry.seeders))
	for name, seeder := range registry.seeders {
		nodes[name] = seeder
	}
	indegree := map[string]int{}
	graph := map[string][]string{}
	for name, seeder := range nodes {
		indegree[name] = 0
		deps := dependenciesOf(seeder)
		for _, dep := range deps {
			if _, ok := nodes[dep]; !ok {
				return nil, errkind.New(errkind.KindConfiguration, fmt.Sprintf("seed: %s depends on unknown seeder %q", name, dep))
			}
			graph[dep] = append(graph[dep], name)
			indegree[name]++
		}
	}
	var queue []string
	for _, name := range registry.order {
		if indegree[name] == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue)
	var ordered []Seeder
	for len(queue) > 0 {
		sort.Strings(queue)
		name := queue[0]
		queue = queue[1:]
		ordered = append(ordered, nodes[name])
		for _, next := range graph[name] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if len(ordered) != len(nodes) {
		return nil, errkind.New(errkind.KindConfiguration, "seed: circular dependency detected")
	}
	return ordered, nil
}

func dependenciesOf(seeder Seeder) []string {
	if seeder == nil {
		return nil
	}
	if dep, ok := seeder.(DependencyProvider); ok {
		return dep.Dependencies()
	}
	return nil
}

func wrapSeederError(name string, err error) error {
	if err == nil {
		return nil
	}
	return errkind.Wrap(errkind.KindRuntimeQuery, fmt.Sprintf("seed: %s", name), err)
}

func expandSeedValues(value any) ([]reflect.Value, error) {
	if value == nil {
		return nil, errkind.New(errkind.KindConfiguration, "seed: nil value")
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, errkind.New(errkind.KindConfiguration, "seed: nil value")
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		return []reflect.Value{rv}, nil
	case reflect.Slice, reflect.Array:
		out := make([]reflect.Value, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i)
			for item.Kind() == reflect.Pointer {
				if item.IsNil() {
					return nil, errkind.New(errkind.KindConfiguration, "seed: nil slice element")
				}
				item = item.Elem()
			}
			if item.Kind() != reflect.Struct {
				return nil, errkind.New(errkind.KindInvalidSchema, "seed: slice elements must be structs")
			}
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, errkind.New(errkind.KindInvalidSchema, "seed: value must be a struct or slice of structs")
	}
}

func syncOne(ctx context.Context, session *orm.Session, item reflect.Value, keys []string) error {
	if session == nil {
		return errkind.New(errkind.KindConfiguration, "seed: nil session")
	}
	record := reflect.New(item.Type()).Elem()
	record.Set(item)
	whereOpts, err := buildSeedFilters(record, keys)
	if err != nil {
		return err
	}
	existing, err := fetchExisting(session, record.Type(), whereOpts)
	if err != nil {
		return err
	}
	switch existing.Len() {
	case 0:
		return session.Create(record.Addr().Interface())
	case 1:
		current := existing.Index(0)
		if seedValuesEqual(current, record) {
			return nil
		}
		mergeSeedAuditFields(record, current)
		return session.UpdateWhere(record.Addr().Interface(), whereOpts...)
	default:
		return errkind.New(errkind.KindInvalidSchema, fmt.Sprintf("seed: key matched multiple rows for %s", record.Type().Name()))
	}
}

func buildSeedFilters(record reflect.Value, keys []string) ([]orm.QueryOption, error) {
	opts := make([]orm.QueryOption, 0, len(keys))
	for _, key := range keys {
		field, ok := lookupSeedField(record, key)
		if !ok {
			return nil, errkind.New(errkind.KindConfiguration, fmt.Sprintf("seed: missing key field %q", key))
		}
		if isZeroValue(field.Interface()) {
			return nil, errkind.New(errkind.KindConfiguration, fmt.Sprintf("seed: key field %q is zero", key))
		}
		opts = append(opts, orm.Where(schema.ToSnakeCase(key)+" = ?", field.Interface()))
	}
	return opts, nil
}

func fetchExisting(session *orm.Session, typ reflect.Type, opts []orm.QueryOption) (reflect.Value, error) {
	slice := reflect.New(reflect.SliceOf(typ))
	queryOpts := append([]orm.QueryOption{}, opts...)
	queryOpts = append(queryOpts, orm.Limit(2))
	if err := session.Find(slice.Interface(), queryOpts...); err != nil {
		return reflect.Value{}, err
	}
	return slice.Elem(), nil
}

func lookupSeedField(record reflect.Value, name string) (reflect.Value, bool) {
	if record.Kind() == reflect.Pointer {
		record = record.Elem()
	}
	indexMap := schema.StructFieldIndexMap(record.Type())
	if indexMap == nil {
		return reflect.Value{}, false
	}
	if idx, ok := indexMap[strings.ToLower(schema.ToSnakeCase(name))]; ok {
		return record.FieldByIndex(idx), true
	}
	return reflect.Value{}, false
}

func applySeedTimestamps(record reflect.Value, isInsert bool, now time.Time) error {
	if record.Kind() == reflect.Pointer {
		record = record.Elem()
	}
	if record.Kind() != reflect.Struct {
		return errkind.New(errkind.KindInvalidSchema, "seed: record must be a struct")
	}
	for i := 0; i < record.NumField(); i++ {
		field := record.Type().Field(i)
		if !record.Field(i).CanSet() {
			continue
		}
		switch {
		case strings.EqualFold(field.Name, "CreatedAt") && isInsert && isZeroValue(record.Field(i).Interface()):
			record.Field(i).Set(reflect.ValueOf(now).Convert(record.Field(i).Type()))
		case strings.EqualFold(field.Name, "UpdatedAt"):
			record.Field(i).Set(reflect.ValueOf(now).Convert(record.Field(i).Type()))
		}
	}
	return nil
}

func mergeSeedAuditFields(dst, src reflect.Value) {
	if dst.Kind() == reflect.Pointer {
		dst = dst.Elem()
	}
	if src.Kind() == reflect.Pointer {
		src = src.Elem()
	}
	if dst.Kind() != reflect.Struct || src.Kind() != reflect.Struct {
		return
	}
	for _, name := range []string{
		"CreatedAt",
		"CreatedBy",
		"UpdatedBy",
		"DeletedAt",
		"DeletedBy",
	} {
		dstField, ok := lookupSeedField(dst, name)
		if !ok || !dstField.CanSet() {
			continue
		}
		srcField, ok := lookupSeedField(src, name)
		if !ok || !srcField.IsValid() {
			continue
		}
		if isZeroValue(dstField.Interface()) {
			dstField.Set(srcField)
			continue
		}
		if strings.EqualFold(name, "CreatedAt") || strings.EqualFold(name, "CreatedBy") || strings.EqualFold(name, "UpdatedBy") || strings.EqualFold(name, "DeletedAt") || strings.EqualFold(name, "DeletedBy") {
			dstField.Set(srcField)
		}
	}
}

func seedValuesEqual(a, b reflect.Value) bool {
	return reflect.DeepEqual(normalizeSeedComparable(a), normalizeSeedComparable(b))
}

func normalizeSeedComparable(v reflect.Value) any {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return v.Interface()
	}
	cp := reflect.New(v.Type()).Elem()
	cp.Set(v)
	for i := 0; i < cp.NumField(); i++ {
		field := cp.Type().Field(i)
		if !cp.Field(i).CanSet() {
			continue
		}
		switch {
		case strings.EqualFold(field.Name, "CreatedAt"):
			cp.Field(i).Set(reflect.Zero(cp.Field(i).Type()))
		case strings.EqualFold(field.Name, "UpdatedAt"):
			cp.Field(i).Set(reflect.Zero(cp.Field(i).Type()))
		case strings.EqualFold(field.Name, "DeletedAt"):
			cp.Field(i).Set(reflect.Zero(cp.Field(i).Type()))
		case strings.EqualFold(field.Name, "CreatedBy"):
			cp.Field(i).Set(reflect.Zero(cp.Field(i).Type()))
		case strings.EqualFold(field.Name, "UpdatedBy"):
			cp.Field(i).Set(reflect.Zero(cp.Field(i).Type()))
		case strings.EqualFold(field.Name, "DeletedBy"):
			cp.Field(i).Set(reflect.Zero(cp.Field(i).Type()))
		}
	}
	return cp.Interface()
}

func isZeroValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	default:
		return rv.IsZero()
	}
}
