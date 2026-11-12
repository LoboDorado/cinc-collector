package server

import (
	"context"

	"github.com/cinc-project/cinc-collector/internal/storage"
	"github.com/cinc-project/cinc-collector/internal/storage/s3"
)

func newS3Backend(ctx context.Context, opts StorageOptions) (storage.Backend, error) {
	return s3.New(ctx, s3.Options{
		Bucket:   opts.S3Bucket,
		Endpoint: opts.S3Endpoint,
		Region:   opts.S3Region,
	})
}
