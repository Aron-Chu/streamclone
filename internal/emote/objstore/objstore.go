package objstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

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
	obj, err := c.mc.GetObject(ctx, c.bucket, c.srcKey(id), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
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
