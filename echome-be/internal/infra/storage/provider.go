package storage

import (
	"github.com/justin/echome-be/config"
	"github.com/justin/echome-be/internal/domain/storage"
	s3storage "github.com/justin/echome-be/internal/infra/storage/s3"
)

// ProvideObjectStorage wires the S3-compatible storage implementation into the application.
func ProvideObjectStorage(cfg *config.Config) (storage.ObjectStorage, error) {
	// Keep storage optional for existing deployments. Once any S3 setting is
	// supplied, NewService validates the complete configuration.
	if cfg.S3.Endpoint == "" && cfg.S3.Region == "" && cfg.S3.Bucket == "" &&
		cfg.S3.AccessKeyID == "" && cfg.S3.SecretAccessKey == "" && cfg.S3.SessionToken == "" {
		return nil, nil
	}
	return s3storage.NewService(cfg.S3)
}
