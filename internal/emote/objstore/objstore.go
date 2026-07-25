package objstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store is the object-storage contract used by emote serving, rendering, and
// migration routing. It preserves streamed reads and metadata on the hot path.
type Store interface {
	Get(context.Context, string, string) ([]byte, string, error)
	Open(context.Context, string, string) (io.ReadCloser, ObjectInfo, error)
	Stat(context.Context, string, string) (ObjectInfo, error)
	Exists(context.Context, string, string) (bool, error)
	Put(context.Context, string, string, []byte) error
	PutSrc(context.Context, string, []byte, string) error
	GetSrc(context.Context, string) ([]byte, error)
	Delete(context.Context, string, string) error
	EnsureBucket(context.Context, bool) error
}

type Client struct {
	mc     *minio.Client
	bucket string
	prefix string
}

func New(endpoint, accessKey, secretKey, bucket, prefix string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	prefix = strings.Trim(prefix, "/")
	return &Client{mc: mc, bucket: bucket, prefix: prefix}, nil
}

func (c *Client) objectKey(parts ...string) string {
	key := strings.Join(parts, "/")
	if c.prefix == "" {
		return key
	}
	return c.prefix + "/" + key
}

func (c *Client) emoteKey(id, scale string) string {
	return c.objectKey(id, scale+".webp")
}

func (c *Client) srcKey(id string) string {
	return c.objectKey(id, "src")
}

func (c *Client) Get(ctx context.Context, id, scale string) ([]byte, string, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, c.emoteKey(id, scale), minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	defer obj.Close()
	info, err := obj.Stat()
	if err != nil {
		return nil, "", err
	}
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, "", err
	}
	contentType := info.ContentType
	if contentType == "" {
		contentType = "image/webp"
	}
	return data, contentType, nil
}

// ObjectInfo is metadata from a Stat without reading the body.
type ObjectInfo struct {
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// ManifestEntry is a read-only inventory record for a stored emote object.
// SHA256 is populated only when the inventory caller requests full-byte
// verification; ETag is retained as provider metadata, not treated as a hash.
type ManifestEntry struct {
	Key          string    `json:"key"`
	Kind         string    `json:"kind"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"contentType,omitempty"`
	ETag         string    `json:"etag,omitempty"`
	SHA256       string    `json:"sha256,omitempty"`
	LastModified time.Time `json:"lastModified,omitempty"`
}

// Inventory streams object metadata through emit without accumulating the
// bucket listing in memory. includeSHA256 downloads each object sequentially
// and is intended for an operator-controlled migration evidence run.
func (c *Client) Inventory(
	ctx context.Context,
	includeSHA256 bool,
	emit func(ManifestEntry) error,
) error {
	if c == nil || c.mc == nil {
		return fmt.Errorf("emote object inventory: client unavailable")
	}
	if emit == nil {
		return fmt.Errorf("emote object inventory: emit callback is required")
	}
	listPrefix := c.prefix
	if listPrefix != "" {
		listPrefix += "/"
	}
	for object := range c.mc.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    listPrefix,
		Recursive: true,
	}) {
		if object.Err != nil {
			return fmt.Errorf("emote object inventory list: %w", object.Err)
		}
		stat, err := c.mc.StatObject(ctx, c.bucket, object.Key, minio.StatObjectOptions{})
		if err != nil {
			return fmt.Errorf("emote object inventory stat %s: %w", object.Key, err)
		}
		entry := ManifestEntry{
			Key:          object.Key,
			Kind:         manifestObjectKind(object.Key),
			Size:         stat.Size,
			ContentType:  stat.ContentType,
			ETag:         strings.Trim(stat.ETag, `"`),
			LastModified: stat.LastModified,
		}
		if includeSHA256 {
			obj, err := c.mc.GetObject(ctx, c.bucket, object.Key, minio.GetObjectOptions{})
			if err != nil {
				return fmt.Errorf("emote object inventory open %s: %w", object.Key, err)
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, obj)
			closeErr := obj.Close()
			if copyErr != nil {
				return fmt.Errorf("emote object inventory hash %s: %w", object.Key, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("emote object inventory close %s: %w", object.Key, closeErr)
			}
			entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
		}
		if err := emit(entry); err != nil {
			return fmt.Errorf("emote object inventory emit %s: %w", object.Key, err)
		}
	}
	return nil
}

func manifestObjectKind(key string) string {
	switch {
	case path.Base(key) == "src":
		return "source"
	case strings.HasSuffix(strings.ToLower(key), ".webp"):
		return "render"
	default:
		return "other"
	}
}

// Exists reports whether an emote scale object is present (Stat only).
func (c *Client) Exists(ctx context.Context, id, scale string) (bool, error) {
	_, err := c.Stat(ctx, id, scale)
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" || errResp.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Stat returns object metadata without downloading the body.
func (c *Client) Stat(ctx context.Context, id, scale string) (ObjectInfo, error) {
	info, err := c.mc.StatObject(ctx, c.bucket, c.emoteKey(id, scale), minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, err
	}
	ct := info.ContentType
	if ct == "" {
		ct = "image/webp"
	}
	return ObjectInfo{
		Size:         info.Size,
		ContentType:  ct,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

// Open returns a streaming reader for an emote asset. Caller must Close.
func (c *Client) Open(ctx context.Context, id, scale string) (io.ReadCloser, ObjectInfo, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, c.emoteKey(id, scale), minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	info, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, ObjectInfo{}, err
	}
	ct := info.ContentType
	if ct == "" {
		ct = "image/webp"
	}
	return obj, ObjectInfo{
		Size:         info.Size,
		ContentType:  ct,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

func (c *Client) Put(ctx context.Context, id, scale string, data []byte) error {
	k := c.emoteKey(id, scale)
	_, err := c.mc.PutObject(ctx, c.bucket, k, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "image/webp",
	})
	return err
}

func (c *Client) PutSrc(ctx context.Context, id string, data []byte, contentType string) error {
	k := c.srcKey(id)
	_, err := c.mc.PutObject(ctx, c.bucket, k, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (c *Client) GetSrc(ctx context.Context, id string) ([]byte, error) {
	data, _, err := c.GetSrcWithContentType(ctx, id)
	return data, err
}

// GetSrcWithContentType preserves source-object metadata for read-through
// promotion without widening the core Store contract used by existing fakes.
func (c *Client) GetSrcWithContentType(ctx context.Context, id string) ([]byte, string, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, c.srcKey(id), minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	defer obj.Close()
	info, err := obj.Stat()
	if err != nil {
		return nil, "", err
	}
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, "", err
	}
	contentType := strings.TrimSpace(info.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return data, contentType, nil
}

func (c *Client) Delete(ctx context.Context, id, scale string) error {
	return c.mc.RemoveObject(ctx, c.bucket, c.emoteKey(id, scale), minio.RemoveObjectOptions{})
}

func (c *Client) EnsureBucket(ctx context.Context, publicRead bool) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if !exists {
		if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
			return err
		}
	}
	if !publicRead {
		return nil
	}
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, c.bucket)
	return c.mc.SetBucketPolicy(ctx, c.bucket, policy)
}
