package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

type cachedSchema struct {
	fingerprint string
	schema      *Schema
}

type cachedSnapshot struct {
	fingerprint string
	snapshot    *Snapshot
}

var (
	schemaBuildCache  sync.Map // map[string]cachedSchema
	snapshotLoadCache sync.Map // map[string]cachedSnapshot
)

func loadCachedSchema(root, fingerprint string) (*Schema, bool) {
	if cached, ok := schemaBuildCache.Load(root); ok {
		entry, ok := cached.(cachedSchema)
		if ok && entry.fingerprint == fingerprint && entry.schema != nil {
			return entry.schema.Clone(), true
		}
	}
	return nil, false
}

func storeCachedSchema(root, fingerprint string, schema *Schema) {
	if root == "" || schema == nil {
		return
	}
	schemaBuildCache.Store(root, cachedSchema{
		fingerprint: fingerprint,
		schema:      schema.Clone(),
	})
}

func loadCachedSnapshot(path, fingerprint string) (*Snapshot, bool) {
	if cached, ok := snapshotLoadCache.Load(path); ok {
		entry, ok := cached.(cachedSnapshot)
		if ok && entry.fingerprint == fingerprint && entry.snapshot != nil {
			return entry.snapshot.Clone(), true
		}
	}
	return nil, false
}

func storeCachedSnapshot(path, fingerprint string, snapshot *Snapshot) {
	if path == "" || snapshot == nil {
		return
	}
	snapshotLoadCache.Store(path, cachedSnapshot{
		fingerprint: fingerprint,
		snapshot:    snapshot.Clone(),
	})
}

func sourceFingerprint(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var parts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%d", name, info.Size(), info.ModTime().UnixNano()))
	}
	sort.Strings(parts)
	return fingerprintParts(parts...), nil
}

func fingerprintBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fingerprintParts(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return nil
	}
	out := *s
	if s.Schema != nil {
		out.Schema = s.Schema.Clone()
	}
	return &out
}
