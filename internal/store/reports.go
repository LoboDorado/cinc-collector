package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ComplianceReport is one InSpec compliance scan report.
type ComplianceReport struct {
	ReportID        string
	Org             string
	NodeName        string
	Environment     string
	Status          string
	ControlsPassed  *int
	ControlsFailed  *int
	ControlsSkipped *int
	Profiles        json.RawMessage
	RawReport       json.RawMessage
	StartTime       time.Time
	EndTime         *time.Time
}

// ReportStore manages the compliance_reports table and its search index.
type ReportStore struct {
	db *DB
}

// NewReportStore returns a ReportStore backed by db.
func NewReportStore(db *DB) *ReportStore { return &ReportStore{db: db} }

// Create inserts a compliance report. searchText is the flattened
// profile/control titles and error text, built by the caller from Profiles
// and RawReport, since the inverted index is declared over a real text
// column rather than a JSON path.
func (s *ReportStore) Create(ctx context.Context, r ComplianceReport, searchText string) error {
	_, err := s.db.Pool.Exec(ctx, `
		INSERT INTO compliance_reports (
			report_id, org, node_name, environment, status,
			controls_passed, controls_failed, controls_skipped,
			profiles, raw_report, search_text, start_time, end_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`,
		r.ReportID, r.Org, r.NodeName, r.Environment, r.Status,
		r.ControlsPassed, r.ControlsFailed, r.ControlsSkipped,
		r.Profiles, r.RawReport, searchText, r.StartTime, r.EndTime,
	)
	if err != nil {
		return fmt.Errorf("store: create compliance report: %w", err)
	}
	return nil
}

// Get returns the report identified by reportID, or ErrNotFound.
func (s *ReportStore) Get(ctx context.Context, reportID string) (*ComplianceReport, error) {
	var r ComplianceReport
	err := s.db.Pool.QueryRow(ctx, `
		SELECT report_id, org, node_name, environment, status,
		       controls_passed, controls_failed, controls_skipped,
		       profiles, raw_report, start_time, end_time
		  FROM compliance_reports
		 WHERE report_id = $1
	`, reportID).Scan(
		&r.ReportID, &r.Org, &r.NodeName, &r.Environment, &r.Status,
		&r.ControlsPassed, &r.ControlsFailed, &r.ControlsSkipped,
		&r.Profiles, &r.RawReport, &r.StartTime, &r.EndTime,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get compliance report: %w", err)
	}
	return &r, nil
}

// ListByNode returns reports for org/node, most recent first, up to limit.
func (s *ReportStore) ListByNode(ctx context.Context, org, node string, limit int) ([]ComplianceReport, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Pool.Query(ctx, `
		SELECT report_id, org, node_name, environment, status,
		       controls_passed, controls_failed, controls_skipped,
		       profiles, raw_report, start_time, end_time
		  FROM compliance_reports
		 WHERE org = $1 AND node_name = $2
		 ORDER BY start_time DESC
		 LIMIT $3
	`, org, node, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list compliance reports: %w", err)
	}
	defer rows.Close()

	var out []ComplianceReport
	for rows.Next() {
		var r ComplianceReport
		if err := rows.Scan(
			&r.ReportID, &r.Org, &r.NodeName, &r.Environment, &r.Status,
			&r.ControlsPassed, &r.ControlsFailed, &r.ControlsSkipped,
			&r.Profiles, &r.RawReport, &r.StartTime, &r.EndTime,
		); err != nil {
			return nil, fmt.Errorf("store: scan compliance report: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchResult is one match from Search, with its BM25 relevance score.
type SearchResult struct {
	ComplianceReport
	Relevance float64
}

// Search performs full-text search over compliance reports using
// SereneDB's native inverted index. Queries select FROM the index name
// (compliance_reports_search), not the base table: see docs/specs for why.
// org is always required (every search is scoped to one org); status, if
// non-empty, additionally filters by exact match.
func (s *ReportStore) Search(ctx context.Context, org, query, status string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	sql := `
		SELECT report_id, org, node_name, environment, status,
		       controls_passed, controls_failed, controls_skipped,
		       profiles, raw_report, start_time, end_time,
		       BM25(compliance_reports_search.tableoid) AS relevance
		  FROM compliance_reports_search
		 WHERE org = $1
		   AND (node_name @@ $2 OR search_text @@ $2)`
	args := []any{org, query}
	if status != "" {
		sql += " AND status = $3"
		args = append(args, status)
	}
	sql += " ORDER BY relevance DESC, start_time DESC LIMIT " + limitPlaceholder(len(args)+1)
	args = append(args, limit)

	rows, err := s.db.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("store: search compliance reports: %w", err)
	}
	defer rows.Close()

	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(
			&r.ReportID, &r.Org, &r.NodeName, &r.Environment, &r.Status,
			&r.ControlsPassed, &r.ControlsFailed, &r.ControlsSkipped,
			&r.Profiles, &r.RawReport, &r.StartTime, &r.EndTime,
			&r.Relevance,
		); err != nil {
			return nil, fmt.Errorf("store: scan search result: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func limitPlaceholder(n int) string {
	return fmt.Sprintf("$%d", n)
}
