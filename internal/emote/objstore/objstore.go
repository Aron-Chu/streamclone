package objstore

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc     *minio.Client
	bucket string
}

func New(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	return &Client{mc: mc, bucket: bucket}, nil
}

func emoteKey(id, scale string) string {
	return fmt.Sprintf("%s/%s.webp", id, scale)
}

func srcKey(id string) string {
	return fmt.Sprintf("%s/src", id)
}

func (c *Client) Put(ctx context.Context, id, scale string, data []byte) error {
	k := emoteKey(id, scale)
	_, err := c.mc.PutObject(ctx, c.bucket, k, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: "image/webp",
	})
	return err
}

func (c *Client) PutSrc(ctx context.Context, id string, data []byte, contentType string) error {
	k := srcKey(id)
	_, err := c.mc.PutObject(ctx, c.bucket, k, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (c *Client) GetSrc(ctx context.Context, id string) ([]byte, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, srcKey(id), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

func (c *Client) Delete(ctx context.Context, id, scale string) error {
	return c.mc.RemoveObject(ctx, c.bucket, emoteKey(id, scale), minio.RemoveObjectOptions{})
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if !exists {
		if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
			return err
		}
	}
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, c.bucket)
	return c.mc.SetBucketPolicy(ctx, c.bucket, policy)
}
