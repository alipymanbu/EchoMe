package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/justin/echome-be/config"
	domainstorage "github.com/justin/echome-be/internal/domain/storage"
)

const defaultPresignExpiry = 15 * time.Minute

var (
	ErrInvalidObjectKey = errors.New("invalid object key")
	ErrInvalidS3Config  = errors.New("invalid s3 configuration")
)

type client interface {
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	DeleteObject(context.Context, *awss3.DeleteObjectInput, ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
}

type presigner interface {
	PresignGetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
	PresignPutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

// Service is an S3-compatible object storage implementation.
type Service struct {
	client        client
	presigner     presigner
	bucket        string
	presignExpiry time.Duration
}

var _ domainstorage.ObjectStorage = (*Service)(nil)

// NewService creates an S3 client. When access keys are omitted, the AWS SDK's
// default credential chain is used, which supports environment variables,
// workload identity, and instance roles.
func NewService(cfg config.S3Config) (*Service, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load s3 SDK config: %w", err)
	}

	clientOptions := func(options *awss3.Options) {
		options.UsePathStyle = cfg.ForcePathStyle
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(strings.TrimRight(cfg.Endpoint, "/"))
		}
	}
	s3Client := awss3.NewFromConfig(awsCfg, clientOptions)

	expiry := defaultPresignExpiry
	if cfg.PresignExpiryMins > 0 {
		expiry = time.Duration(cfg.PresignExpiryMins) * time.Minute
	}

	return &Service{
		client:        s3Client,
		presigner:     awss3.NewPresignClient(s3Client),
		bucket:        cfg.Bucket,
		presignExpiry: expiry,
	}, nil
}

// newService is kept small and injectable for unit tests.
func newService(client client, presigner presigner, bucket string, expiry time.Duration) (*Service, error) {
	if strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("%w: bucket is required", ErrInvalidS3Config)
	}
	if expiry <= 0 {
		expiry = defaultPresignExpiry
	}
	return &Service{client: client, presigner: presigner, bucket: bucket, presignExpiry: expiry}, nil
}

func (s *Service) PutObject(ctx context.Context, key string, body io.Reader, contentLength int64, contentType string) error {
	if err := validateObjectKey(key); err != nil {
		return err
	}
	if body == nil {
		return errors.New("object body is required")
	}

	input := &awss3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentLength >= 0 {
		input.ContentLength = aws.Int64(contentLength)
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if _, err := s.client.PutObject(ctx, input); err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

func (s *Service) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}
	output, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	if output == nil || output.Body == nil {
		return nil, fmt.Errorf("get object %q: empty response body", key)
	}
	return output.Body, nil
}

func (s *Service) DeleteObject(ctx context.Context, key string) error {
	if err := validateObjectKey(key); err != nil {
		return err
	}
	if _, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete object %q: %w", key, err)
	}
	return nil
}

func (s *Service) PresignGetObject(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	request, err := s.presigner.PresignGetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *awss3.PresignOptions) {
		options.Expires = s.resolveExpiry(expiry)
	})
	if err != nil {
		return "", fmt.Errorf("presign get object %q: %w", key, err)
	}
	return request.URL, nil
}

func (s *Service) PresignPutObject(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}
	input := &awss3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	request, err := s.presigner.PresignPutObject(ctx, input, func(options *awss3.PresignOptions) {
		options.Expires = s.resolveExpiry(expiry)
	})
	if err != nil {
		return "", fmt.Errorf("presign put object %q: %w", key, err)
	}
	return request.URL, nil
}

func (s *Service) resolveExpiry(expiry time.Duration) time.Duration {
	if expiry <= 0 {
		return s.presignExpiry
	}
	return expiry
}

func validateConfig(cfg config.S3Config) error {
	if strings.TrimSpace(cfg.Region) == "" {
		return fmt.Errorf("%w: region is required", ErrInvalidS3Config)
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return fmt.Errorf("%w: bucket is required", ErrInvalidS3Config)
	}
	if (cfg.AccessKeyID == "") != (cfg.SecretAccessKey == "") {
		return fmt.Errorf("%w: access_key_id and secret_access_key must be provided together", ErrInvalidS3Config)
	}
	if cfg.Endpoint != "" {
		parsed, err := url.Parse(cfg.Endpoint)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%w: endpoint must be an http(s) URL", ErrInvalidS3Config)
		}
	}
	if cfg.SessionToken != "" && cfg.AccessKeyID == "" {
		return fmt.Errorf("%w: session_token requires static credentials", ErrInvalidS3Config)
	}
	return nil
}

func validateObjectKey(key string) error {
	if key == "" || len(key) > 1024 || !utf8.ValidString(key) || strings.ContainsRune(key, '\x00') {
		return ErrInvalidObjectKey
	}
	return nil
}
