// Package local implements the storage.Backend interface on the local
// filesystem, used for development and small single-node deployments.
package local

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/cinc-project/cinc-collector/internal/storage"
)

// Backend stores blobs as files under Root. Keys may contain "/" and are
// treated as relative paths; Backend creates parent directories as needed
// and rejects keys that would escape Root.
type Backend struct {
	Root string
}

// New returns a local filesystem Backend rooted at root, creating it if
// necessary.
func New(root string) (*Backend, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Backend{Root: root}, nil
}

func (b *Backend) resolve(key string) (string, error) {
	clean := filepath.Clean("/" + key) // anchor to prevent ".." escaping Root
	return filepath.Join(b.Root, clean), nil
}

func (b *Backend) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	path, err := b.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func (b *Backend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := b.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

func (b *Backend) Delete(ctx context.Context, key string) error {
	path, err := b.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
