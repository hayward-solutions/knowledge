package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/hayward-solutions/knowledge/api/internal/config"
)

var ErrNotFound = errors.New("object not found")

const memoryBufferLimit = 8 << 20

type ObjectStore interface {
	Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
	Delete(ctx context.Context, key string) error
}

type S3Store struct {
	client *s3.Client
	bucket string
}

func FromEnv(ctx context.Context) (ObjectStore, error) {
	return New(ctx, config.LoadStorage())
}

func New(ctx context.Context, cfg config.StorageSettings) (ObjectStore, error) {
	if cfg.Bucket == "" {
		return nil, nil
	}
	st, err := NewS3(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func NewS3(ctx context.Context, cfg config.StorageSettings) (*S3Store, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if cfg.AccessKeyID != "" && cfg.SecretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretKey, cfg.SessionToken),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		}
		o.UsePathStyle = cfg.ForcePathStyle
	})
	return &S3Store{client: client, bucket: cfg.Bucket}, nil
}

func (s *S3Store) Bucket() string { return s.bucket }

func (s *S3Store) Put(ctx context.Context, key, contentType string, r io.Reader, size int64) error {
	body, n, cleanup, err := seekable(r, size)
	if err != nil {
		return err
	}
	defer cleanup()
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if n >= 0 {
		in.ContentLength = aws.Int64(n)
	}
	_, err = s.client.PutObject(ctx, in)
	return err
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, "", 0, ErrNotFound
		}
		return nil, "", 0, err
	}
	return out.Body, aws.ToString(out.ContentType), aws.ToInt64(out.ContentLength), nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func seekable(r io.Reader, size int64) (io.ReadSeeker, int64, func(), error) {
	if rs, ok := r.(io.ReadSeeker); ok {
		return rs, size, func() {}, nil
	}
	if size >= 0 && size <= memoryBufferLimit {
		data, err := io.ReadAll(io.LimitReader(r, size))
		if err != nil {
			return nil, 0, nil, err
		}
		if int64(len(data)) != size {
			return nil, 0, nil, io.ErrUnexpectedEOF
		}
		return bytes.NewReader(data), size, func() {}, nil
	}
	f, err := os.CreateTemp("", "knowledge-upload-*")
	if err != nil {
		return nil, 0, nil, err
	}
	cleanup := func() {
		f.Close()
		os.Remove(f.Name())
	}
	n, err := io.Copy(f, r)
	if err != nil {
		cleanup()
		return nil, 0, nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, 0, nil, err
	}
	return f, n, cleanup, nil
}
