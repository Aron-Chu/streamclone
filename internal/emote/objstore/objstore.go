package objstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

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
