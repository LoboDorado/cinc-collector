package store

import (
	"context"
	"fmt"
	"time"

	"github.com/cinc-project/cinc-collector/internal/auth"
)

// TokenStore manages bearer tokens. It implements auth.Store.
type TokenStore struct {
	db *DB
}

// NewTokenStore returns a TokenStore backed by db.
func NewTokenStore(db *DB) *TokenStore { return &TokenStore{db: db} }

// TokenInfo describes a token without exposing its hash or plaintext value.
type TokenInfo struct {
	ID         string
	Name       string
	Org        string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// Create generates a new token named name with home org org (may be empty),
// stores its hash, and returns the plaintext token. The plaintext is never
// stored and this is the only time it is available.
func (s *TokenStore) Create(ctx context.Context, name, org string) (token string, err error) {
	token, hash, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO tokens (name, token_hash, org) VALUES ($1, $2, $3)`,
		name, hash, nullableString(org),
	)
	if err != nil {
		return "", fmt.Errorf("store: create token: %w", err)
	}
	return token, nil
}

// List returns all tokens, most recently created first.
func (s *TokenStore) List(ctx context.Context) ([]TokenInfo, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id, name, COALESCE(org, ''), created_at, last_used_at, revoked_at
		   FROM tokens
		  ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list tokens: %w", err)
	}
	defer rows.Close()

	var out []TokenInfo
	for rows.Next() {
		var t TokenInfo
		if err := rows.Scan(&t.ID, &t.Name, &t.Org, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("store: scan token: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Revoke marks the token named name as revoked. Revoking an already-revoked
// or missing token is not an error.
func (s *TokenStore) Revoke(ctx context.Context, name string) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE tokens SET revoked_at = now() WHERE name = $1 AND revoked_at IS NULL`,
		name,
	)
	if err != nil {
		return fmt.Errorf("store: revoke token: %w", err)
	}
	return nil
}

// Authenticate reports whether tokenHash matches an unrevoked token, and
// best-effort records its use. It implements auth.Store.
func (s *TokenStore) Authenticate(ctx context.Context, tokenHash string) (ok bool, org string, err error) {
	var id string
	err = s.db.Pool.QueryRow(ctx,
		`SELECT id, COALESCE(org, '') FROM tokens WHERE token_hash = $1 AND revoked_at IS NULL`,
		tokenHash,
	).Scan(&id, &org)
	if err != nil {
		if isNoRows(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("store: authenticate: %w", err)
	}

	// Best-effort: a failure here must not fail the request.
	_, _ = s.db.Pool.Exec(ctx, `UPDATE tokens SET last_used_at = now() WHERE id = $1`, id)

	return true, org, nil
}
