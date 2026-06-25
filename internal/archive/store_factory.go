package archive

import (
	"errors"
	"fmt"
)

// StoreConfig selects Azure-only or optional R2 read-through/dual-write wiring.
type StoreConfig struct {
	Azure           AzureConfig
	R2              R2Config
	PrimaryProvider string
	ReadThrough     bool
	DualWrite       bool
}

func (c StoreConfig) primaryR2() bool {
	return NormalizePrimaryProvider(c.PrimaryProvider) == "r2"
}

func (c StoreConfig) needsR2() bool {
	return c.ReadThrough || c.DualWrite || c.primaryR2()
}

// NewBlobStore returns Azure-only by default, or a read-through wrapper when flags require R2.
func NewBlobStore(cfg StoreConfig) (BlobStore, error) {
	azure, err := NewAzureBlobStore(cfg.Azure)
	if err != nil {
		return nil, err
	}
	if !cfg.needsR2() {
		return azure, nil
	}
	if !cfg.R2.configured() {
		return nil, errors.New("archive: r2 credentials required when ARCHIVE_READ_THROUGH, ARCHIVE_DUAL_WRITE, or ARCHIVE_PRIMARY_PROVIDER=r2 is set")
	}
	r2, err := NewR2BlobStore(cfg.R2)
	if err != nil {
		return nil, fmt.Errorf("archive: r2 init: %w", err)
	}
	return NewReadThroughStore(azure, r2, ReadThroughOptions{
		ReadThrough: cfg.ReadThrough,
		DualWrite:   cfg.DualWrite,
		PrimaryR2:   cfg.primaryR2(),
	})
}
