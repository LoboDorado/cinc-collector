package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cinc-project/cinc-collector/internal/auth"
)

// testDB opens a connection to a real SereneDB instance for integration
// tests, skipping if CINC_COLLECTOR_TEST_DB_DSN isn't set -- so `go test
// ./...` needs no external services by default, matching cinc-server-ng's
// own convention. Point it at the dev/docker-compose.yml instance locally;
// CI provides it via a SereneDB service container.
func testDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("CINC_COLLECTOR_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("CINC_COLLECTOR_TEST_DB_DSN not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := Open(ctx, dsn, 5)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := Migrate(dsn); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func TestTokenStoreLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tokens := NewTokenStore(db)

	name := uniqueName(t, "token")
	plaintext, err := tokens.Create(ctx, name, "acme")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	hash := auth.HashToken(plaintext)
	ok, org, err := tokens.Authenticate(ctx, hash)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !ok {
		t.Fatal("Authenticate: expected ok=true for freshly created token")
	}
	if org != "acme" {
		t.Errorf("Authenticate org = %q, want acme", org)
	}

	if err := tokens.Revoke(ctx, name); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	ok, _, err = tokens.Authenticate(ctx, hash)
	if err != nil {
		t.Fatalf("Authenticate after revoke: %v", err)
	}
	if ok {
		t.Fatal("Authenticate: expected ok=false after revoke")
	}
}

func TestNodeStoreUpsertIsIdempotentOnConflict(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	nodes := NewNodeStore(db)
	org := uniqueName(t, "org")

	first := NodeUpsert{Org: org, Name: "node1", Platform: "ubuntu", SeenAt: time.Now()}
	if err := nodes.Upsert(ctx, first); err != nil {
		t.Fatalf("Upsert (create): %v", err)
	}

	second := first
	second.Platform = "rhel"
	second.SeenAt = first.SeenAt.Add(time.Minute)
	if err := nodes.Upsert(ctx, second); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	got, err := nodes.Get(ctx, org, "node1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Platform != "rhel" {
		t.Errorf("Platform = %q, want rhel (the second upsert should win)", got.Platform)
	}
}

func TestReportStoreSearch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	reports := NewReportStore(db)
	org := uniqueName(t, "org")

	failed, passed := 0, 1
	err := reports.Create(ctx, ComplianceReport{
		ReportID:       uuid.New().String(),
		Org:            org,
		NodeName:       "node1",
		Status:         "passed",
		ControlsPassed: &passed,
		ControlsFailed: &failed,
		Profiles:       json.RawMessage(`[]`),
		RawReport:      json.RawMessage(`{}`),
		StartTime:      time.Now(),
	}, "ssh hardening baseline disable root login")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// SereneDB's inverted index is near-real-time, not immediately
	// consistent (it has a refresh_interval, ~1s by default): a row just
	// written may not be searchable yet. Poll briefly rather than assuming
	// either instant visibility or a fixed delay. See docs/specs.
	var results []SearchResult
	deadline := time.Now().Add(5 * time.Second)
	for {
		results, err = reports.Search(ctx, org, "hardening", "", 10)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(results) != 1 {
		t.Fatalf("Search returned %d results, want 1", len(results))
	}
	if results[0].NodeName != "node1" {
		t.Errorf("NodeName = %q, want node1", results[0].NodeName)
	}
}

func uniqueName(t *testing.T, prefix string) string {
	t.Helper()
	return prefix + "-" + t.Name() + "-" + time.Now().Format("150405.000000000")
}
