package main

import (
	"testing"

	"streamclone/internal/config"
)

func TestGoldVODSegmentOptsFromConfig(t *testing.T) {
	cfg := config.Config{
		GoldVODSegmentsEnabled: true,
		GoldMaxSegmentsPerVOD:  8,
		GoldRetryMax:           5,
		GoldLeaseTTLSeconds:    240,
	}
	enabled, maxSegments, retryMax, leaseTTL, owner := goldVODSegmentOptsFromConfig(cfg)
	if !enabled || maxSegments != 8 || retryMax != 5 || leaseTTL != 240 || owner != "" {
		t.Fatalf("opts = (%v,%d,%d,%d,%q)", enabled, maxSegments, retryMax, leaseTTL, owner)
	}
}
