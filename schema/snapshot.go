package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dionisius77/dorm/errkind"
)

type Snapshot struct {
	GeneratedAt time.Time `json:"generated_at"`
	Schema      *Schema   `json:"schema"`
}

func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fingerprint := fingerprintBytes(data)
	if cached, ok := loadCachedSnapshot(path, fingerprint); ok {
		return cached, nil
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if snap.Schema != nil {
		snap.Schema.Sort()
	}
	storeCachedSnapshot(path, fingerprint, &snap)
	return snap.Clone(), nil
}

func SaveSnapshot(path string, snap *Snapshot) error {
	if snap == nil {
		return errkind.New(errkind.KindConfiguration, "schema: nil snapshot")
	}
	if snap.Schema != nil {
		snap.Schema.Sort()
		if err := snap.Schema.Validate(); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	storeCachedSnapshot(path, fingerprintBytes(data), snap)
	return nil
}
