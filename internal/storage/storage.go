// Package storage abstracts blob storage for InSpec profile tarballs behind
// a single interface, with a local-filesystem implementation for dev and an
// S3-compatible (AWS S3 or MinIO) implementation for production. Profile
// metadata (name, version, checksum, size) lives in SereneDB; only the raw
// tarball bytes live here, keyed by an opaque storage key.
package storage

import (
	"context"
	"io"
)

// Backend stores and retrieves opaque blobs by key.
type Backend interface {
	// Put stores size bytes read from r under key, overwriting any existing
	// value.
	Put(ctx context.Context, key string, r io.Reader, size int64) error

	// Get returns a reader for the blob stored under key. The caller must
	// close it. Get returns ErrNotFound if key does not exist.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the blob stored under key. Deleting a missing key is
	// not an error.
	Delete(ctx context.Context, key string) error
}

// ErrNotFound is returned by Get when key does not exist.
var ErrNotFound = errNotFound{}

type errNotFound struct{}

func (errNotFound) Error() string { return "storage: not found" }
