package local

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/cinc-project/cinc-collector/internal/storage"
)

func TestBackendRoundTrip(t *testing.T) {
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	content := []byte("profile tarball bytes")
	if err := b.Put(ctx, "acme/myprofile/1.0.0.tar.gz", bytes.NewReader(content), int64(len(content))); err != nil {
		t.Fatalf("Put: %v", err)
	}

	r, err := b.Get(ctx, "acme/myprofile/1.0.0.tar.gz")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer func() { _ = r.Close() }()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("round-tripped content = %q, want %q", got, content)
	}

	if err := b.Delete(ctx, "acme/myprofile/1.0.0.tar.gz"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := b.Get(ctx, "acme/myprofile/1.0.0.tar.gz"); err != storage.ErrNotFound {
		t.Errorf("Get after Delete: err = %v, want storage.ErrNotFound", err)
	}
}

func TestBackendGetMissingKey(t *testing.T) {
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := b.Get(context.Background(), "nope"); err != storage.ErrNotFound {
		t.Errorf("Get: err = %v, want storage.ErrNotFound", err)
	}
}

func TestBackendDeleteMissingKeyIsNotAnError(t *testing.T) {
	b, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := b.Delete(context.Background(), "nope"); err != nil {
		t.Errorf("Delete of missing key: err = %v, want nil", err)
	}
}
