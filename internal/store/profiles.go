package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ProfileMetadata describes an InSpec profile tarball. The tarball bytes
// themselves live in blob storage (internal/storage), keyed by StorageKey;
// only metadata lives here. See docs/specs for why the tarball itself isn't
// stored in SereneDB.
type ProfileMetadata struct {
	ID         string
	Org        string
	Name       string
	Version    string
	Title      string
	Summary    string
	Owner      string
	SHA256     string
	StorageKey string
	SizeBytes  int64
	Metadata   json.RawMessage
	UploadedBy string
	UploadedAt time.Time
}

// ProfileStore manages the profiles table.
type ProfileStore struct {
	db *DB
}

// NewProfileStore returns a ProfileStore backed by db.
func NewProfileStore(db *DB) *ProfileStore { return &ProfileStore{db: db} }

// Create inserts profile metadata for a newly uploaded tarball.
func (s *ProfileStore) Create(ctx context.Context, p ProfileMetadata) error {
	if p.Metadata == nil {
		p.Metadata = json.RawMessage(`{}`)
	}
	_, err := s.db.Pool.Exec(ctx, `
		INSERT INTO profiles (
			org, name, version, title, summary, owner,
			sha256, storage_key, size_bytes, metadata, uploaded_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		p.Org, p.Name, p.Version, p.Title, p.Summary, p.Owner,
		p.SHA256, p.StorageKey, p.SizeBytes, p.Metadata, nullableUUID(p.UploadedBy),
	)
	if err != nil {
		return fmt.Errorf("store: create profile: %w", err)
	}
	return nil
}

// Get returns one profile version, or ErrNotFound.
func (s *ProfileStore) Get(ctx context.Context, org, name, version string) (*ProfileMetadata, error) {
	var p ProfileMetadata
	err := s.db.Pool.QueryRow(ctx, `
		SELECT id, org, name, version, title, summary, owner,
		       sha256, storage_key, size_bytes, metadata, uploaded_at
		  FROM profiles
		 WHERE org = $1 AND name = $2 AND version = $3
	`, org, name, version).Scan(
		&p.ID, &p.Org, &p.Name, &p.Version, &p.Title, &p.Summary, &p.Owner,
		&p.SHA256, &p.StorageKey, &p.SizeBytes, &p.Metadata, &p.UploadedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get profile: %w", err)
	}
	return &p, nil
}

// List returns all profile versions in org, newest first.
func (s *ProfileStore) List(ctx context.Context, org string) ([]ProfileMetadata, error) {
	rows, err := s.db.Pool.Query(ctx, `
		SELECT id, org, name, version, title, summary, owner,
		       sha256, storage_key, size_bytes, metadata, uploaded_at
		  FROM profiles
		 WHERE org = $1
		 ORDER BY uploaded_at DESC
	`, org)
	if err != nil {
		return nil, fmt.Errorf("store: list profiles: %w", err)
	}
	defer rows.Close()

	var out []ProfileMetadata
	for rows.Next() {
		var p ProfileMetadata
		if err := rows.Scan(
			&p.ID, &p.Org, &p.Name, &p.Version, &p.Title, &p.Summary, &p.Owner,
			&p.SHA256, &p.StorageKey, &p.SizeBytes, &p.Metadata, &p.UploadedAt,
		); err != nil {
			return nil, fmt.Errorf("store: scan profile: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete removes one profile version's metadata row. It does not delete the
// underlying blob; callers are expected to delete that separately via
// internal/storage once this succeeds.
func (s *ProfileStore) Delete(ctx context.Context, org, name, version string) error {
	_, err := s.db.Pool.Exec(ctx,
		`DELETE FROM profiles WHERE org = $1 AND name = $2 AND version = $3`,
		org, name, version,
	)
	if err != nil {
		return fmt.Errorf("store: delete profile: %w", err)
	}
	return nil
}
