package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Store is the minimal persistence surface Middleware needs. store.TokenStore
// implements it; tests can fake it without a real database.
type Store interface {
	// Authenticate reports whether tokenHash matches an unrevoked token,
	// and best-effort records its use. It must not fail the request on a
	// last-used-at update error. org is the token's home org (may be
	// empty), used to scope requests that carry no org of their own -- see
	// docs/specs for why this exists.
	Authenticate(ctx context.Context, tokenHash string) (ok bool, org string, err error)
}

type contextKey int

const orgContextKey contextKey = iota

// TokenOrg returns the authenticated token's home org, if any, from a
// request context populated by Middleware.Require.
func TokenOrg(ctx context.Context) string {
	org, _ := ctx.Value(orgContextKey).(string)
	return org
}

// Middleware enforces bearer-token authentication. There are no scopes: any
// valid token may call any route it's attached to.
type Middleware struct {
	store Store
}

// NewMiddleware returns a Middleware backed by store.
func NewMiddleware(store Store) *Middleware {
	return &Middleware{store: store}
}

// Require wraps next so it only runs for requests carrying a valid bearer
// token. On failure it writes the same {"error": [...]} JSON shape used
// everywhere else, duplicated here (rather than imported from internal/api)
// to avoid a package cycle, matching cinc-server-ng's own precedent of
// keeping auth-layer error responses self-contained.
func (m *Middleware) Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}

		valid, org, err := m.store.Authenticate(r.Context(), HashToken(token))
		if err != nil {
			writeAuthError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !valid {
			writeAuthError(w, http.StatusUnauthorized, "invalid or revoked token")
			return
		}
		ctx := context.WithValue(r.Context(), orgContextKey, org)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error []string `json:"error"`
	}{Error: []string{message}})
}
