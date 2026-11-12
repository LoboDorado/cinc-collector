package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeStore struct {
	validHash string
	org       string
}

func (f fakeStore) Authenticate(ctx context.Context, tokenHash string) (bool, string, error) {
	if tokenHash == f.validHash {
		return true, f.org, nil
	}
	return false, "", nil
}

func TestMiddlewareRequire(t *testing.T) {
	token, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	m := NewMiddleware(fakeStore{validHash: hash, org: "acme"})

	var sawOrg string
	handler := m.Require(func(w http.ResponseWriter, r *http.Request) {
		sawOrg = TokenOrg(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if sawOrg != "acme" {
			t.Errorf("TokenOrg in handler = %q, want acme", sawOrg)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer cinc_wrongvalue")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
}
