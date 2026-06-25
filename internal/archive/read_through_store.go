package archive

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ReadThroughOptions configures optional R2 read-through and dual-write behavior.
type ReadThroughOptions struct {
	ReadThrough bool
	DualWrite   bool
	PrimaryR2   bool
}

// ReadThroughStore reads from R2 when enabled and falls back to Azure; writes stay on Azure unless dual-write or primary R2 is set.
type ReadThroughStore struct {
	azure  BlobStore
	r2     BlobStore
	opts   ReadThroughOptions
}

func NewReadThroughStore(azure, r2 BlobStore, opts ReadThroughOptions) (*ReadThroughStore, error) {
	if azure == nil {
		return nil, errors.New("archive: azure blob store is required")
	}
	if (opts.ReadThrough || opts.DualWrite || opts.PrimaryR2) && r2 == nil {
		return nil, errors.New("archive: r2 blob store is required when read-through, dual-write, or primary=r2 is enabled")
	}
	return &ReadThroughStore{azure: azure, r2: r2, opts: opts}, nil
}

func (s *ReadThroughStore) writeStore() BlobStore {
	if s.opts.PrimaryR2 && s.r2 != nil {
		return s.r2
	}
	return s.azure
}

func (s *ReadThroughStore) Get(ctx context.Context, key string) ([]byte, error) {
	if s == nil || s.azure == nil {
		return nil, errors.New("archive: read-through store is not configured")
	}
	if s.opts.ReadThrough && s.r2 != nil {
		data, err := s.r2.Get(ctx, key)
		if err == nil {
			return data, nil
		}
		if !IsNotFound(err) {
			return nil, fmt.Errorf("archive: r2 get: %w", err)
		}
	}
	if s.opts.PrimaryR2 && s.r2 != nil && !s.opts.ReadThrough {
		return s.r2.Get(ctx, key)
	}
	return s.azure.Get(ctx, key)
}

func (s *ReadThroughStore) Put(ctx context.Context, key string, data []byte, contentType string) (BlobPutResult, error) {
	if s == nil || s.azure == nil {
		return BlobPutResult{}, errors.New("archive: read-through store is not configured")
	}
	writer := s.writeStore()
	res, err := writer.Put(ctx, key, data, contentType)
	if err != nil {
		return BlobPutResult{}, err
	}
	if s.opts.DualWrite && s.r2 != nil && writer == s.azure {
		if _, err := s.r2.Put(ctx, key, data, contentType); err != nil {
			return BlobPutResult{}, fmt.Errorf("archive: r2 dual-write: %w", err)
		}
	}
	return res, nil
}

func (s *ReadThroughStore) BlobURI(key string) string {
	if s == nil {
		return ""
	}
	return s.writeStore().BlobURI(key)
}

// NormalizePrimaryProvider returns azure or r2.
func NormalizePrimaryProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "r2":
		return "r2"
	default:
		return "azure"
	}
}
