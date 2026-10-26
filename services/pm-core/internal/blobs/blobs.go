// Package blobs is pm-core's door to the S3-compatible object store (FSD §05:
// RustFS in dev, any S3 endpoint in production): signed envelopes, document
// binaries, exports. Keys are namespaced by kind; every write returns the
// SHA-256 so the audit log can carry it.
package blobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config is the S3 connection.
type Config struct {
	Endpoint  string // http://rustfs:9000 or https://s3.example
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
}

// Store is one bucket.
type Store struct {
	c      *minio.Client
	bucket string
}

// ErrNotFound is returned for a missing key.
var ErrNotFound = errors.New("blobs: not found")

// Open connects and makes sure the bucket exists (dev convenience; production buckets pre-exist).
func Open(ctx context.Context, cfg Config) (*Store, error) {
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("blobs: invalid endpoint %q", cfg.Endpoint)
	}
	if cfg.Bucket == "" {
		return nil, errors.New("blobs: bucket required")
	}
	c, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Secure: u.Scheme == "https", Region: cfg.Region})
	if err != nil {
		return nil, fmt.Errorf("blobs: client: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	exists, err := c.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("blobs: bucket check: %w", err)
	}
	if !exists {
		if err := c.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("blobs: make bucket: %w", err)
		}
	}
	return &Store{c: c, bucket: cfg.Bucket}, nil
}

// Ping is the readiness probe.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.c.BucketExists(ctx, s.bucket)
	return err
}

// Put writes an object and returns its SHA-256.
func (s *Store) Put(ctx context.Context, key string, data []byte, mediaType string) (sha string, err error) {
	sum := sha256.Sum256(data)
	_, err = s.c.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: mediaType, UserMetadata: map[string]string{"sha256": hex.EncodeToString(sum[:])}})
	if err != nil {
		return "", fmt.Errorf("blobs: put %s: %w", key, err)
	}
	return hex.EncodeToString(sum[:]), nil
}

// Get reads an object.
func (s *Store) Get(ctx context.Context, key string) (data []byte, mediaType string, err error) {
	obj, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("blobs: get %s: %w", key, err)
	}
	defer obj.Close()
	data, err = io.ReadAll(obj)
	if err != nil {
		var resp minio.ErrorResponse
		if errors.As(err, &resp) && resp.Code == "NoSuchKey" {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("blobs: read %s: %w", key, err)
	}
	info, err := obj.Stat()
	if err == nil {
		mediaType = info.ContentType
	}
	return data, mediaType, nil
}

// Key builds a namespaced object key.
func Key(kind string, parts ...string) string {
	return strings.Trim(kind, "/") + "/" + strings.Join(parts, "/")
}
