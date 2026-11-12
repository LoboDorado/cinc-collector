package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Node is a Chef node as tracked by Cinc Collector: identity plus the
// latest-known state reported by converge runs.
type Node struct {
	Org             string
	Name            string
	FQDN            string
	Platform        string
	PlatformFamily  string
	PlatformVersion string
	Environment     string
	ChefVersion     string
	PolicyName      string
	PolicyGroup     string
	Attributes      json.RawMessage
	Tags            json.RawMessage
	LastCCRStatus   string
	LatestRunID     string
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
}

// NodeUpsert carries the fields updated on every check-in. Every field is
// overwritten on conflict (there is no partial-update semantics), matching
// how a Chef Automate data-collector run_converge message is self-contained:
// callers should always pass the full picture from the latest message.
type NodeUpsert struct {
	Org             string
	Name            string
	FQDN            string
	Platform        string
	PlatformFamily  string
	PlatformVersion string
	Environment     string
	ChefVersion     string
	PolicyName      string
	PolicyGroup     string
	Attributes      json.RawMessage
	Tags            json.RawMessage
	LastCCRStatus   string
	LatestRunID     string
	SeenAt          time.Time
}

// NodeStore manages the nodes table.
type NodeStore struct {
	db *DB
}

// NewNodeStore returns a NodeStore backed by db.
func NewNodeStore(db *DB) *NodeStore { return &NodeStore{db: db} }

// Upsert creates or updates a node's latest-known state. Concurrent
// check-ins for the same node race harmlessly: whichever write commits last
// wins, which matches how a fleet's chef-client runs are already ordered by
// wall-clock time, not something this needs to referee more carefully.
func (s *NodeStore) Upsert(ctx context.Context, n NodeUpsert) error {
	if n.Attributes == nil {
		n.Attributes = json.RawMessage(`{}`)
	}
	if n.Tags == nil {
		n.Tags = json.RawMessage(`[]`)
	}
	_, err := s.db.Pool.Exec(ctx, `
		INSERT INTO nodes (
			org, name, fqdn, platform, platform_family, platform_version,
			environment, chef_version, policy_name, policy_group,
			attributes, tags, last_ccr_status, latest_run_id,
			first_seen_at, last_seen_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)
		ON CONFLICT (org, name) DO UPDATE SET
			fqdn = EXCLUDED.fqdn,
			platform = EXCLUDED.platform,
			platform_family = EXCLUDED.platform_family,
			platform_version = EXCLUDED.platform_version,
			environment = EXCLUDED.environment,
			chef_version = EXCLUDED.chef_version,
			policy_name = EXCLUDED.policy_name,
			policy_group = EXCLUDED.policy_group,
			attributes = EXCLUDED.attributes,
			tags = EXCLUDED.tags,
			last_ccr_status = EXCLUDED.last_ccr_status,
			latest_run_id = EXCLUDED.latest_run_id,
			last_seen_at = EXCLUDED.last_seen_at
	`,
		n.Org, n.Name, n.FQDN, n.Platform, n.PlatformFamily, n.PlatformVersion,
		n.Environment, n.ChefVersion, n.PolicyName, n.PolicyGroup,
		n.Attributes, n.Tags, n.LastCCRStatus, nullableUUID(n.LatestRunID), n.SeenAt,
	)
	if err != nil {
		return fmt.Errorf("store: upsert node: %w", err)
	}
	return nil
}

// Get returns the node identified by org/name, or ErrNotFound.
func (s *NodeStore) Get(ctx context.Context, org, name string) (*Node, error) {
	var n Node
	err := s.db.Pool.QueryRow(ctx, `
		SELECT org, name, fqdn, platform, platform_family, platform_version,
		       environment, chef_version, policy_name, policy_group,
		       attributes, tags, last_ccr_status, COALESCE(latest_run_id::text, ''),
		       first_seen_at, last_seen_at
		  FROM nodes
		 WHERE org = $1 AND name = $2
	`, org, name).Scan(
		&n.Org, &n.Name, &n.FQDN, &n.Platform, &n.PlatformFamily, &n.PlatformVersion,
		&n.Environment, &n.ChefVersion, &n.PolicyName, &n.PolicyGroup,
		&n.Attributes, &n.Tags, &n.LastCCRStatus, &n.LatestRunID,
		&n.FirstSeenAt, &n.LastSeenAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get node: %w", err)
	}
	return &n, nil
}

// List returns nodes in org, most recently seen first, up to limit.
func (s *NodeStore) List(ctx context.Context, org string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Pool.Query(ctx, `
		SELECT org, name, fqdn, platform, platform_family, platform_version,
		       environment, chef_version, policy_name, policy_group,
		       attributes, tags, last_ccr_status, COALESCE(latest_run_id::text, ''),
		       first_seen_at, last_seen_at
		  FROM nodes
		 WHERE org = $1
		 ORDER BY last_seen_at DESC
		 LIMIT $2
	`, org, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list nodes: %w", err)
	}
	defer rows.Close()

	var out []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.Org, &n.Name, &n.FQDN, &n.Platform, &n.PlatformFamily, &n.PlatformVersion,
			&n.Environment, &n.ChefVersion, &n.PolicyName, &n.PolicyGroup,
			&n.Attributes, &n.Tags, &n.LastCCRStatus, &n.LatestRunID,
			&n.FirstSeenAt, &n.LastSeenAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan node: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// nullableUUID returns nil for an empty string so a missing UUID is bound as
// SQL NULL rather than failing UUID parsing.
func nullableUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
