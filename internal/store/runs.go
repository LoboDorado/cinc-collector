package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ConvergeRun is one chef-client run, built from a Chef Automate
// data-collector run_start/run_converge message pair.
type ConvergeRun struct {
	RunID                string
	Org                  string
	NodeName             string
	Status               string
	StartTime            time.Time
	EndTime              *time.Time
	TotalResourceCount   *int
	UpdatedResourceCount *int
	RunList              json.RawMessage
	Resources            json.RawMessage
	Error                json.RawMessage
	RawMessage           json.RawMessage
}

// RunStore manages the converge_runs table.
type RunStore struct {
	db *DB
}

// NewRunStore returns a RunStore backed by db.
func NewRunStore(db *DB) *RunStore { return &RunStore{db: db} }

// Upsert records a converge run. run_start and run_converge messages share
// the same run_id, so later messages for the same run overwrite earlier
// ones rather than producing duplicate rows.
func (s *RunStore) Upsert(ctx context.Context, r ConvergeRun) error {
	_, err := s.db.Pool.Exec(ctx, `
		INSERT INTO converge_runs (
			run_id, org, node_name, status, start_time, end_time,
			total_resource_count, updated_resource_count,
			run_list, resources, error, raw_message
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (run_id) DO UPDATE SET
			status = EXCLUDED.status,
			end_time = EXCLUDED.end_time,
			total_resource_count = EXCLUDED.total_resource_count,
			updated_resource_count = EXCLUDED.updated_resource_count,
			run_list = EXCLUDED.run_list,
			resources = EXCLUDED.resources,
			error = EXCLUDED.error,
			raw_message = EXCLUDED.raw_message
	`,
		r.RunID, r.Org, r.NodeName, r.Status, r.StartTime, r.EndTime,
		r.TotalResourceCount, r.UpdatedResourceCount,
		r.RunList, r.Resources, r.Error, r.RawMessage,
	)
	if err != nil {
		return fmt.Errorf("store: upsert converge run: %w", err)
	}
	return nil
}

// Get returns the run identified by runID, or ErrNotFound.
func (s *RunStore) Get(ctx context.Context, runID string) (*ConvergeRun, error) {
	var r ConvergeRun
	err := s.db.Pool.QueryRow(ctx, `
		SELECT run_id, org, node_name, status, start_time, end_time,
		       total_resource_count, updated_resource_count,
		       run_list, resources, error, raw_message
		  FROM converge_runs
		 WHERE run_id = $1
	`, runID).Scan(
		&r.RunID, &r.Org, &r.NodeName, &r.Status, &r.StartTime, &r.EndTime,
		&r.TotalResourceCount, &r.UpdatedResourceCount,
		&r.RunList, &r.Resources, &r.Error, &r.RawMessage,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get converge run: %w", err)
	}
	return &r, nil
}

// ListByNode returns runs for org/node, most recent first, up to limit.
func (s *RunStore) ListByNode(ctx context.Context, org, node string, limit int) ([]ConvergeRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Pool.Query(ctx, `
		SELECT run_id, org, node_name, status, start_time, end_time,
		       total_resource_count, updated_resource_count,
		       run_list, resources, error, raw_message
		  FROM converge_runs
		 WHERE org = $1 AND node_name = $2
		 ORDER BY start_time DESC
		 LIMIT $3
	`, org, node, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list converge runs: %w", err)
	}
	defer rows.Close()

	var out []ConvergeRun
	for rows.Next() {
		var r ConvergeRun
		if err := rows.Scan(
			&r.RunID, &r.Org, &r.NodeName, &r.Status, &r.StartTime, &r.EndTime,
			&r.TotalResourceCount, &r.UpdatedResourceCount,
			&r.RunList, &r.Resources, &r.Error, &r.RawMessage,
		); err != nil {
			return nil, fmt.Errorf("store: scan converge run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
