package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	defaultR2Prefix = "archive"
)

// R2Config configures Cloudflare R2 via the S3-compatible API.
type R2Config struct {
	AccountID            string
	Bucket               string
	Prefix               string
	Endpoint             string
	AccessKeyID          string
	SecretAccessKey      string
	AccessKeyIDFile      string
	SecretAccessKeyFile  string
}

func (c R2Config) configured() bool {
	return strings.TrimSpace(c.Bucket) != "" &&
		(strings.TrimSpace(c.AccessKeyID) != "" || strings.TrimSpace(c.AccessKeyIDFile) != "") &&
		(strings.TrimSpace(c.SecretAccessKey) != "" || strings.TrimSpace(c.SecretAccessKeyFile) != "")
}

func loadSecretValue(value, file string) (string, error) {
	if v := strings.TrimSpace(value); v != "" {
		return v, nil
	}
	file = strings.TrimSpace(file)
	if file == "" {
		return "", errors.New("archive: secret value or secret file is required")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("archive: read secret file: %w", err)
	}
	v := strings.TrimSpace(string(raw))
	if v == "" {
		return "", errors.New("archive: secret file is empty")
	}
	return v, nil
}

func loadR2Credentials(cfg R2Config) (accessKey, secretKey string, err error) {
	accessKey, err = loadSecretValue(cfg.AccessKeyID, cfg.AccessKeyIDFile)
	if err != nil {
		return "", "", fmt.Errorf("archive: r2 access key: %w", err)
	}
	secretKey, err = loadSecretValue(cfg.SecretAccessKey, cfg.SecretAccessKeyFile)
	if err != nil {
		return "", "", fmt.Errorf("archive: r2 secret key: %w", err)
	}
	return accessKey, secretKey, nil
}

// R2DefaultEndpoint returns the default R2 S3 endpoint for an account ID.
func R2DefaultEndpoint(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
}

func parseR2Endpoint(raw string) (host string, secure bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, errors.New("archive: r2 endpoint is empty")
	}
	if !strings.Contains(raw, "://") {
		return raw, true, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("archive: r2 endpoint parse: %w", err)
	}
	if u.Host == "" {
		return "", false, errors.New("archive: r2 endpoint missing host")
	}
	return u.Host, u.Scheme != "http", nil
}

// R2BlobStore implements BlobStore against Cloudflare R2 (S3-compatible).
type R2BlobStore struct {
	client    *minio.Client
	bucket    string
	prefix    string
	accountID string
	endpoint  string
}

func NewR2BlobStore(cfg R2Config) (*R2BlobStore, error) {
	if !cfg.configured() {
		return nil, errors.New("archive: r2 bucket and credentials are required")
	}
	accessKey, secretKey, err := loadR2Credentials(cfg)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = R2DefaultEndpoint(cfg.AccountID)
	}
	host, secure, err := parseR2Endpoint(endpoint)
	if err != nil {
		return nil, err
	}
	client, err := minio.New(host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: "auto",
	})
	if err != nil {
		return nil, fmt.Errorf("archive: r2 client: %w", err)
	}
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix == "" {
		prefix = defaultR2Prefix
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	accountID := strings.TrimSpace(cfg.AccountID)
	return &R2BlobStore{
		client:    client,
		bucket:    bucket,
		prefix:    prefix,
		accountID: accountID,
		endpoint:  endpoint,
	}, nil
}

func (s *R2BlobStore) fullKey(key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if s.prefix == "" {
		return key
	}
	return path.Join(s.prefix, key)
}

func (s *R2BlobStore) logicalKey(fullKey string) string {
	fullKey = strings.TrimPrefix(strings.TrimSpace(fullKey), "/")
	if s.prefix == "" {
		return fullKey
	}
	prefix := s.prefix + "/"
	return strings.TrimPrefix(fullKey, prefix)
}

// R2BlobStoreFullKey exposes the object key used for S3 operations (tests).
func R2BlobStoreFullKey(s *R2BlobStore, key string) string {
	if s == nil {
		return key
	}
	return s.fullKey(key)
}

func (s *R2BlobStore) BlobURI(key string) string {
	objectKey := s.fullKey(key)
	base := strings.TrimSpace(s.endpoint)
	if base == "" {
		base = R2DefaultEndpoint(s.accountID)
	}
	base = strings.TrimRight(base, "/")
	return fmt.Sprintf("%s/%s/%s", base, s.bucket, objectKey)
}

func (s *R2BlobStore) Put(ctx context.Context, key string, data []byte, contentType string) (BlobPutResult, error) {
	if s == nil || s.client == nil {
		return BlobPutResult{}, errors.New("archive: r2 blob store is not configured")
	}
	objectKey := s.fullKey(key)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	info, err := s.client.PutObject(ctx, s.bucket, objectKey, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return BlobPutResult{}, err
	}
	return BlobPutResult{
		URI:      s.BlobURI(key),
		ETag:     strings.Trim(info.ETag, `"`),
		ByteSize: info.Size,
	}, nil
}

func (s *R2BlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("archive: r2 blob store is not configured")
	}
	objectKey := s.fullKey(key)
	obj, err := s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer obj.Close()
	if _, err := obj.Stat(); err != nil {
		if IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return io.ReadAll(io.LimitReader(obj, 256<<20))
}
