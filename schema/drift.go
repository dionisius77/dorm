package schema

import (
	"context"
	"database/sql"
	"os"

	"github.com/dionisius77/dorm/errkind"
)

type DriftReport struct {
	Expected     *Schema
	Snapshot     *Snapshot
	Actual       *Schema
	ExpectedDiff *Diff
	SnapshotDiff *Diff
}

// HasDrift reports whether the live schema differs from the expected schema.
func (r *DriftReport) HasDrift() bool {
	return r != nil && r.ExpectedDiff != nil && !r.ExpectedDiff.Empty()
}

// HasSnapshotDrift reports whether the live schema differs from the snapshot.
func (r *DriftReport) HasSnapshotDrift() bool {
	return r != nil && r.SnapshotDiff != nil && !r.SnapshotDiff.Empty()
}

// Clean reports whether no drift was detected.
func (r *DriftReport) Clean() bool {
	return !r.HasDrift() && !r.HasSnapshotDrift()
}

// DetectDrift compares two schemas and returns a drift report.
func DetectDrift(expected, actual *Schema) (*DriftReport, error) {
	return detectDrift(expected, actual)
}

// DetectDriftWithSnapshot compares a live schema against an expected schema and snapshot.
func DetectDriftWithSnapshot(expected *Schema, snapshot *Snapshot, actual *Schema) (*DriftReport, error) {
	return detectDriftWithSnapshot(expected, snapshot, actual)
}

// DetectDriftFromSource builds the expected schema from source and compares it with the live database.
func DetectDriftFromSource(ctx context.Context, root string, inspector Inspector, db *sql.DB, schemaName, snapshotPath string) (*DriftReport, error) {
	var report *DriftReport
	err := traceOperation(ctx, "db.schema.check", func(ctx context.Context) error {
		if root == "" {
			return errkind.New(errkind.KindConfiguration, "schema: empty root")
		}
		builder := NewBuilder(root)
		expected, err := builder.Build(ctx)
		if err != nil {
			return err
		}
		if inspector == nil {
			inspector = PostgresInspector{}
		}
		actual, err := inspector.Inspect(ctx, db, schemaName)
		if err != nil {
			return err
		}
		var snapshot *Snapshot
		if snapshotPath != "" {
			if snap, err := LoadSnapshot(snapshotPath); err == nil {
				snapshot = snap
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		diff, err := Compare(expected, actual)
		if err != nil {
			return err
		}
		report = &DriftReport{
			Expected:     expected.Clone(),
			Actual:       actual.Clone(),
			ExpectedDiff: diff,
			Snapshot:     cloneSnapshot(snapshot),
		}
		if snapshot != nil && snapshot.Schema != nil && actual != nil {
			snapshotDiff, err := Compare(snapshot.Schema, actual)
			if err != nil {
				return err
			}
			report.SnapshotDiff = snapshotDiff
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func detectDrift(expected, actual *Schema) (*DriftReport, error) {
	var report *DriftReport
	err := traceOperation(context.Background(), "db.schema.check", func(context.Context) error {
		if expected == nil || actual == nil {
			return errkind.New(errkind.KindInvalidSchema, "schema: drift detection requires non-nil schemas")
		}
		diff, err := Compare(expected, actual)
		if err != nil {
			return err
		}
		report = &DriftReport{
			Expected:     expected.Clone(),
			Actual:       actual.Clone(),
			ExpectedDiff: diff,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func detectDriftWithSnapshot(expected *Schema, snapshot *Snapshot, actual *Schema) (*DriftReport, error) {
	var report *DriftReport
	err := traceOperation(context.Background(), "db.schema.check", func(context.Context) error {
		next, err := detectDrift(expected, actual)
		if err != nil {
			return err
		}
		next.Snapshot = cloneSnapshot(snapshot)
		if snapshot != nil && snapshot.Schema != nil && actual != nil {
			snapshotDiff, err := Compare(snapshot.Schema, actual)
			if err != nil {
				return err
			}
			next.SnapshotDiff = snapshotDiff
		}
		report = next
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func cloneSnapshot(snap *Snapshot) *Snapshot {
	if snap == nil {
		return nil
	}
	out := *snap
	if snap.Schema != nil {
		out.Schema = snap.Schema.Clone()
	}
	return &out
}
