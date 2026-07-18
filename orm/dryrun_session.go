package orm

import (
	"context"
	"fmt"
)

func (d *DryRunSession) Find(ctx context.Context, dest any, opts ...QueryOption) (ExecutionReport, error) {
	return d.run(ctx, "find", func(s *Session) error {
		return s.Find(dest, opts...)
	})
}

func (d *DryRunSession) FindOne(ctx context.Context, dest any, opts ...QueryOption) (ExecutionReport, error) {
	return d.run(ctx, "find_one", func(s *Session) error {
		return s.Find(dest, append(opts, Limit(1))...)
	})
}

func (d *DryRunSession) Count(ctx context.Context, model any, opts ...QueryOption) (ExecutionReport, int64, error) {
	var count int64
	report, err := d.run(ctx, "count", func(s *Session) error {
		var innerErr error
		count, innerErr = s.Count(model, opts...)
		return innerErr
	})
	return report, count, err
}

func (d *DryRunSession) Create(ctx context.Context, model any) (ExecutionReport, error) {
	return d.run(ctx, "create", func(s *Session) error {
		return s.Create(model)
	})
}

func (d *DryRunSession) Update(ctx context.Context, model any) (ExecutionReport, error) {
	return d.run(ctx, "update", func(s *Session) error {
		return s.Update(model)
	})
}

func (d *DryRunSession) UpdateWhere(ctx context.Context, model any, opts ...QueryOption) (ExecutionReport, error) {
	return d.run(ctx, "update_where", func(s *Session) error {
		return s.UpdateWhere(model, opts...)
	})
}

func (d *DryRunSession) Delete(ctx context.Context, model any) (ExecutionReport, error) {
	return d.run(ctx, "delete", func(s *Session) error {
		return s.Delete(model)
	})
}

func (d *DryRunSession) SoftDelete(ctx context.Context, model any) (ExecutionReport, error) {
	return d.run(ctx, "soft_delete", func(s *Session) error {
		return s.SoftDelete(model)
	})
}

func (d *DryRunSession) Upsert(ctx context.Context, model any) (ExecutionReport, error) {
	return d.run(ctx, "upsert", func(s *Session) error {
		return s.Upsert(model)
	})
}

func (d *DryRunSession) run(ctx context.Context, operation string, fn func(*Session) error) (ExecutionReport, error) {
	if d == nil || d.db == nil {
		return ExecutionReport{ExecutionStatus: ExecutionStatusSkipped}, fmt.Errorf("orm: nil db")
	}
	clone, err := d.db.cloneForDryRun()
	if err != nil {
		return ExecutionReport{ExecutionStatus: ExecutionStatusSkipped}, err
	}
	defer func() {
		_ = clone.Close()
	}()
	if ctx == nil {
		ctx = d.ctx
	}
	if ctx == nil {
		ctx = clone.currentContext()
	}
	session := &Session{
		db:          clone,
		ctx:         ctx,
		withDeleted: d.withDeleted,
	}
	err = fn(session)
	report := clone.dryRun.finalize()
	report.Operation = operation
	if clone.queryAdvisor != nil && report.SQL != "" {
		if advised, adviseErr := clone.queryAdvisor.Inspect(ctx, QueryAdvisorInput{
			Operation: operation,
			Table:     report.Table,
			SQL:       report.SQL,
			Schema:    clone.schema,
		}); adviseErr != nil {
			return report, adviseErr
		} else if len(advised.Findings) > 0 {
			report.QueryAdvisor = append([]QueryAdvisorFinding(nil), advised.Findings...)
			clone.dryRun.recordAdvisor(advised.Findings)
		}
	}
	return report, err
}
