// Package s3 implements storage.Backend against any S3-compatible object
// store: AWS S3 in production, or MinIO (via a custom Endpoint) in local
// dev/self-hosted deployments.
package s3

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/cinc-project/cinc-collector/internal/storage"
)

// Backend stores blobs as objects in Bucket.
type Backend struct {
	client *s3.Client
	bucket string
}

// Options configures the S3 backend.
type Options struct {
	Bucket string
	// Endpoint, if set, overrides the default AWS endpoint resolution --
	// used to point at MinIO or another S3-compatible service.
	Endpoint string
	Region   string
}

// New returns an S3-compatible Backend using the AWS SDK's default
// credential chain (environment, shared config, IAM role, etc.) -- no
// custom credential flags, matching how production S3 access is normally
// managed.
func New(ctx context.Context, opts Options) (*Backend, error) {
	cfgOpts := []func(*config.LoadOptions) error{}
	if opts.Region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(opts.Region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if opts.Endpoint != "" {
			o.BaseEndpoint = aws.String(opts.Endpoint)
			o.UsePathStyle = true // required by MinIO and most self-hosted S3-compatible services
		}
	})

	return &Backend{client: client, bucket: opts.Bucket}, nil
}

func (b *Backend) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(b.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
	})
	return err
}

func (b *Backend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey") {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return out.Body, nil
}

func (b *Backend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	return err
}
