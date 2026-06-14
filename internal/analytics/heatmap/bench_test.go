package heatmap

import (
	"math/rand"
	"testing"
	"time"
)

// Performance targets (Requirements 13.1, 13.3):
//   - 360 rollups cold (no cache): p95 ≤ 500ms
//   - With cache (pure compute path): p50 ≤ 100ms, p95 ≤ 200ms
//
// These benchmarks exercise the pure ComputeHeatmap scoring function only
// (no Redis, no HTTP). Cache-hit performance is dominated by Redis GET latency,
// so the compute path must stay well under the cold budget to leave headroom.

// generateRollups produces deterministic rollup data using a fixed seed.
// The data simulates realistic stream patterns: baseline chat/viewers with
// occasional spikes, emote bursts, and viewer momentum changes.
func generateRollups(n int) []MinuteRollup {
	rng := rand.New(rand.NewSource(42))
	base := time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC)

	rollups := make([]MinuteRollup, n)
	for i := range rollups {
		viewers := 200 + rng.Intn(300)
		chat := 5 + rng.Intn(30)
		emotes := rng.Intn(20)
		seventv := 0
		if emotes > 5 {
			seventv = rng.Intn(emotes)
		}

		// Inject spikes every ~20 minutes to produce realistic peak patterns.
		if i%20 == 10 {
			chat = 200 + rng.Intn(500)
			emotes = 50 + rng.Intn(200)
			seventv = emotes / 2
			viewers = 800 + rng.Intn(500)
		}

		emoteMap := make(map[string]int)
		if emotes > 0 {
			numDistinct := 1 + rng.Intn(min(emotes, 8))
			remaining := emotes
			for j := 0; j < numDistinct && remaining > 0; j++ {
				provider := "seventv"
				if j%3 == 1 {
					provider = "twitch"
				} else if j%3 == 2 {
					provider = "ffz"
				}
				count := 1
				if j < numDistinct-1 && remaining > 1 {
					count = 1 + rng.Intn(remaining)
				} else {
					count = remaining
				}
				remaining -= count
				emoteMap[provider+":id"+string(rune('A'+j))+":emote"+string(rune('A'+j))] = count
			}
		}

		rollups[i] = MinuteRollup{
			MinuteTS:          base.Add(time.Duration(i) * time.Minute),
			ViewerAvg:         viewers,
			ViewerMax:         viewers + 50,
			ViewerLatest:      viewers,
			ViewerSamples:     4,
			ChatCount:         chat,
			TotalEmoteCount:   emotes,
			SevenTVEmoteCount: seventv,
			Emotes:            emoteMap,
		}
	}
	return rollups
}

// BenchmarkComputeHeatmap_60Rollups benchmarks 60 rollups (1 hour stream, 1-min windows).
func BenchmarkComputeHeatmap_60Rollups(b *testing.B) {
	rollups := generateRollups(60)
	config := DefaultScoringConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeHeatmap(rollups, config)
	}
}

// BenchmarkComputeHeatmap_180Rollups benchmarks 180 rollups (3 hour stream).
func BenchmarkComputeHeatmap_180Rollups(b *testing.B) {
	rollups := generateRollups(180)
	config := DefaultScoringConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeHeatmap(rollups, config)
	}
}

// BenchmarkComputeHeatmap_360Rollups benchmarks 360 rollups (6 hour stream).
// Target: p95 ≤ 500ms cold, p50 ≤ 100ms cached compute path.
func BenchmarkComputeHeatmap_360Rollups(b *testing.B) {
	rollups := generateRollups(360)
	config := DefaultScoringConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeHeatmap(rollups, config)
	}
}

// BenchmarkComputeHeatmapDetail_360Rollups benchmarks 360 rollups with the
// detail response (per-signal component breakdown).
func BenchmarkComputeHeatmapDetail_360Rollups(b *testing.B) {
	rollups := generateRollups(360)
	config := DefaultScoringConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeHeatmapDetail(rollups, config)
	}
}
