package storage

import (
	"context"
	"io"
	"time"
)

type ObjectStorage interface {
	PutObject(ctx context.Context, key string, body io.Reader, contentLength int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, key string) error
	PresignGetObject(ctx context.Context, key string, expiry time.Duration) (string, error)
	PresignPutObject(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error)
}
