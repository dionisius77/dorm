package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	if snap.Schema != nil {
		snap.Schema.Sort()
	}
	return &snap, nil
}

func SaveSnapshot(path string, snap *Snapshot) error {
	if snap == nil {
		return fmt.Errorf("schema: nil snapshot")
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
	return os.Rename(tmp, path)
}
