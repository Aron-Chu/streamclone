package objstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"streamclone/internal/metrics"
)

const (
	maxReadThroughPromotionBytes = 5 << 20
	maxConcurrentPromotions      = 4
	promotionTimeout             = 30 * time.Second
)

type ReadThroughOptions struct {
	PrimarySecondary bool
	DualWrite        bool
	ReadThrough      bool
}

type ReadThroughStore struct {
	local      Store
	secondary  Store
	primary    Store
	fallback   Store
	opts       ReadThroughOptions
	promotions chan struct{}
}

func NewReadThroughStore(local, secondary Store, opts ReadThroughOptions) (*ReadThroughStore, error) {
	if local == nil {
		return nil, errors.New("emote object store: local store is required")
	}
	store := &ReadThroughStore{
		local:      local,
		secondary:  secondary,
		primary:    local,
		opts:       opts,
		promotions: make(chan struct{}, maxConcurrentPromotions),
	}
	if opts.PrimarySecondary {
		if secondary == nil {
			return nil, errors.New("emote object store: secondary primary requested without secondary store")
		}
		store.primary = secondary
		if opts.ReadThrough {
			store.fallback = local
		}
	} else if secondary != nil && opts.ReadThrough {
		store.fallback = secondary
	}
	return store, nil
}

func (s *ReadThroughStore) replicaStore() Store {
	if s == nil || s.secondary == nil {
		return nil
	}
	if s.opts.PrimarySecondary {
		return s.local
	}
	return s.secondary
}

func (s *ReadThroughStore) Get(ctx context.Context, id, scale string) ([]byte, string, error) {
	data, contentType, err := s.primary.Get(ctx, id, scale)
	if err == nil || s.fallback == nil {
		return data, contentType, err
	}
	data, contentType, fallbackErr := s.fallback.Get(ctx, id, scale)
	if fallbackErr != nil {
		metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_fallback", "error").Inc()
		return nil, "", fmt.Errorf("emote object read primary: %v; fallback: %w", err, fallbackErr)
	}
	metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_fallback", "success").Inc()
	if s.opts.ReadThrough {
		if putErr := s.primary.Put(ctx, id, scale, data); putErr != nil {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_promotion", "error").Inc()
		} else {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_promotion", "success").Inc()
		}
	}
	return data, contentType, nil
}

func (s *ReadThroughStore) Open(ctx context.Context, id, scale string) (io.ReadCloser, ObjectInfo, error) {
	rc, info, err := s.primary.Open(ctx, id, scale)
	if err == nil || s.fallback == nil {
		return rc, info, err
	}
	rc, info, fallbackErr := s.fallback.Open(ctx, id, scale)
	if fallbackErr != nil {
		metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_stream_fallback", "error").Inc()
		return nil, ObjectInfo{}, fmt.Errorf("emote object open primary: %v; fallback: %w", err, fallbackErr)
	}
	metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_stream_fallback", "success").Inc()
	if !s.opts.ReadThrough {
		return rc, info, nil
	}
	return &promotionReadCloser{
		ReadCloser: rc,
		eligible:   info.Size <= 0 || info.Size <= maxReadThroughPromotionBytes,
		promote: func(data []byte) {
			s.promoteRender(ctx, id, scale, data)
		},
	}, info, nil
}

type promotionReadCloser struct {
	io.ReadCloser
	buffer   bytes.Buffer
	eligible bool
	finished bool
	promote  func([]byte)
}

func (r *promotionReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if r.eligible && n > 0 {
		if r.buffer.Len()+n > maxReadThroughPromotionBytes {
			r.eligible = false
			r.buffer.Reset()
		} else {
			_, _ = r.buffer.Write(p[:n])
		}
	}
	if err == io.EOF && !r.finished {
		r.finished = true
		if r.eligible && r.promote != nil {
			r.promote(append([]byte(nil), r.buffer.Bytes()...))
		}
		r.buffer.Reset()
	}
	return n, err
}

func (r *promotionReadCloser) Close() error {
	r.finished = true
	r.eligible = false
	r.buffer.Reset()
	return r.ReadCloser.Close()
}

func (s *ReadThroughStore) promoteRender(ctx context.Context, id, scale string, data []byte) {
	select {
	case s.promotions <- struct{}{}:
	default:
		metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_promotion", "skipped").Inc()
		return
	}
	go func() {
		defer func() { <-s.promotions }()
		promotionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), promotionTimeout)
		defer cancel()
		if err := s.primary.Put(promotionCtx, id, scale, data); err != nil {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_promotion", "error").Inc()
		} else {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_promotion", "success").Inc()
		}
	}()
}

func (s *ReadThroughStore) Stat(ctx context.Context, id, scale string) (ObjectInfo, error) {
	info, err := s.primary.Stat(ctx, id, scale)
	if err == nil || s.fallback == nil {
		return info, err
	}
	info, fallbackErr := s.fallback.Stat(ctx, id, scale)
	if fallbackErr != nil {
		return ObjectInfo{}, fmt.Errorf("emote object stat primary: %v; fallback: %w", err, fallbackErr)
	}
	return info, nil
}

func (s *ReadThroughStore) Exists(ctx context.Context, id, scale string) (bool, error) {
	exists, err := s.primary.Exists(ctx, id, scale)
	if err == nil && exists {
		return true, nil
	}
	if s.fallback == nil {
		return exists, err
	}
	fallbackExists, fallbackErr := s.fallback.Exists(ctx, id, scale)
	if fallbackErr == nil {
		return fallbackExists, nil
	}
	if err != nil {
		return false, fmt.Errorf("emote object exists primary: %v; fallback: %w", err, fallbackErr)
	}
	return false, fmt.Errorf("emote object fallback exists: %w", fallbackErr)
}

func (s *ReadThroughStore) Put(ctx context.Context, id, scale string, data []byte) error {
	if err := s.primary.Put(ctx, id, scale, data); err != nil {
		return err
	}
	if replica := s.replicaStore(); s.opts.DualWrite && replica != nil {
		if err := replica.Put(ctx, id, scale, data); err != nil {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_dual_write", "error").Inc()
			return fmt.Errorf("emote object dual-write render: %w", err)
		}
		metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("render_dual_write", "success").Inc()
	}
	return nil
}

func (s *ReadThroughStore) PutSrc(ctx context.Context, id string, data []byte, contentType string) error {
	if err := s.primary.PutSrc(ctx, id, data, contentType); err != nil {
		return err
	}
	if replica := s.replicaStore(); s.opts.DualWrite && replica != nil {
		if err := replica.PutSrc(ctx, id, data, contentType); err != nil {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("source_dual_write", "error").Inc()
			return fmt.Errorf("emote object dual-write source: %w", err)
		}
		metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("source_dual_write", "success").Inc()
	}
	return nil
}

func (s *ReadThroughStore) GetSrc(ctx context.Context, id string) ([]byte, error) {
	data, _, err := getSourceWithContentType(ctx, s.primary, id)
	if err == nil || s.fallback == nil {
		return data, err
	}
	data, contentType, fallbackErr := getSourceWithContentType(ctx, s.fallback, id)
	if fallbackErr != nil {
		metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("source_fallback", "error").Inc()
		return nil, fmt.Errorf("emote source read primary: %v; fallback: %w", err, fallbackErr)
	}
	metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("source_fallback", "success").Inc()
	if s.opts.ReadThrough {
		if putErr := s.primary.PutSrc(ctx, id, data, contentType); putErr != nil {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("source_promotion", "error").Inc()
		} else {
			metrics.EmoteObjectStoreMigrationOperations.WithLabelValues("source_promotion", "success").Inc()
		}
	}
	return data, nil
}

type sourceGetterWithContentType interface {
	GetSrcWithContentType(context.Context, string) ([]byte, string, error)
}

func getSourceWithContentType(ctx context.Context, store Store, id string) ([]byte, string, error) {
	if metadataStore, ok := store.(sourceGetterWithContentType); ok {
		return metadataStore.GetSrcWithContentType(ctx, id)
	}
	data, err := store.GetSrc(ctx, id)
	return data, "application/octet-stream", err
}

func (s *ReadThroughStore) Delete(ctx context.Context, id, scale string) error {
	primaryErr := s.primary.Delete(ctx, id, scale)
	var replicaErr error
	if replica := s.replicaStore(); replica != nil {
		replicaErr = replica.Delete(ctx, id, scale)
	}
	if primaryErr != nil && replicaErr != nil {
		return fmt.Errorf("emote object delete primary: %v; replica: %w", primaryErr, replicaErr)
	}
	if primaryErr != nil {
		return fmt.Errorf("emote object primary delete: %w", primaryErr)
	}
	if replicaErr != nil {
		return fmt.Errorf("emote object replica delete: %w", replicaErr)
	}
	return nil
}

func (s *ReadThroughStore) EnsureBucket(ctx context.Context, publicRead bool) error {
	if err := s.local.EnsureBucket(ctx, publicRead); err != nil {
		return err
	}
	if s.secondary != nil {
		if err := s.secondary.EnsureBucket(ctx, false); err != nil {
			return fmt.Errorf("emote object secondary bucket: %w", err)
		}
	}
	return nil
}
