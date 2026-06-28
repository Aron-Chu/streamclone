package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultGoldIVRShadowArtifactDir = "runtime/ivr-shadow"

// GoldIVRShadowArtifact is a read-only JSON record for shadow IVR runs (no DB writes).
type GoldIVRShadowArtifact struct {
	StreamID                       string    `json:"stream_id"`
	VodID                          string    `json:"vod_id,omitempty"`
	ChannelID                      string    `json:"channel_id"`
	Login                          string    `json:"login"`
	WindowStart                    time.Time `json:"window_start"`
	WindowEnd                      time.Time `json:"window_end"`
	IVRMessageCount                int       `json:"ivr_message_count"`
	ExistingRollupMinutes          int       `json:"existing_rollup_minutes"`
	MatchedIDCount                 int       `json:"matched_id_count,omitempty"`
	MedianDeltaSeconds             float64   `json:"median_delta_seconds,omitempty"`
	RawSuitabilityPct              float64   `json:"raw_suitability_pct"`
	DedupedSuitabilityPct          float64   `json:"deduped_suitability_pct"`
	AdjustedSuitabilityPct         float64   `json:"adjusted_suitability_pct"`
	PeakOverlapTop3Pct             float64   `json:"peak_overlap_top3_pct"`
	ShapeSimilarityPct             float64   `json:"shape_similarity_pct"`
	Recommendation                 string    `json:"recommendation"`
	GQLPriorityRecommendation      string    `json:"gql_priority_recommendation"`
	WroteRollups                   bool      `json:"wrote_rollups"`
	UpdatedStreamMetadata          bool      `json:"updated_stream_metadata"`
	ShadowOnly                     bool      `json:"shadow_only"`
	Success                        bool      `json:"success"`
	FailureReason                  string    `json:"failure_reason,omitempty"`
	PeakMinuteTimestamps           []string  `json:"peak_minute_timestamps,omitempty"`
	RawFetchSpeedup                float64   `json:"raw_fetch_speedup,omitempty"`
	NormalizedMessagesPerSecondIVR float64   `json:"normalized_messages_per_second_ivr,omitempty"`
	NormalizedMessagesPerSecondGQL float64   `json:"normalized_messages_per_second_gql,omitempty"`
	NormalizedWindowSpeedup        float64   `json:"normalized_window_speedup,omitempty"`
	CreatedAt                      time.Time `json:"created_at"`
	ArtifactPath                   string    `json:"artifact_path,omitempty"`
}

// GoldIVRReconciliationArtifact links a shadow prediction to post-GQL canonical rollups.
type GoldIVRReconciliationArtifact struct {
	StreamID                     string    `json:"stream_id"`
	VodID                        string    `json:"vod_id,omitempty"`
	Login                        string    `json:"login"`
	ShadowArtifactPath           string    `json:"shadow_artifact_path"`
	ShadowRecommendation         string    `json:"shadow_recommendation"`
	ShadowScorePct               float64   `json:"shadow_score_pct"`
	GQLCanonicalMinutes          int       `json:"gql_canonical_minutes"`
	GQLPeakOverlapTop3Pct        float64   `json:"gql_peak_overlap_top3_pct"`
	GQLPeakOverlapTop5Pct        float64   `json:"gql_peak_overlap_top5_pct"`
	ReconciliationScorePct       float64   `json:"reconciliation_score_pct"`
	ReconciliationRecommendation string    `json:"reconciliation_recommendation"`
	GQLPriorityRecommendation    string    `json:"gql_priority_recommendation"`
	GQLCanonicalPresent          bool      `json:"gql_canonical_present"`
	PeakOverlapPass              bool      `json:"peak_overlap_pass"`
	WroteRollups                 bool      `json:"wrote_rollups"`
	UpdatedStreamMetadata        bool      `json:"updated_stream_metadata"`
	CreatedAt                    time.Time `json:"created_at"`
	ArtifactPath                 string    `json:"artifact_path,omitempty"`
}

func resolveGoldIVRShadowArtifactDir(cfg GoldIVRConfig) string {
	dir := strings.TrimSpace(cfg.ShadowArtifactDir)
	if dir == "" {
		dir = defaultGoldIVRShadowArtifactDir
	}
	return dir
}

func writeGoldIVRShadowArtifact(dir string, artifact GoldIVRShadowArtifact) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s.json", sanitizeArtifactToken(artifact.StreamID), artifact.CreatedAt.UTC().Format("20060102T150405Z"))
	path := filepath.Join(dir, name)
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func writeGoldIVRReconciliationArtifact(dir string, artifact GoldIVRReconciliationArtifact) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-reconcile-%s.json", sanitizeArtifactToken(artifact.StreamID), artifact.CreatedAt.UTC().Format("20060102T150405Z"))
	path := filepath.Join(dir, name)
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

type shadowArtifactFile struct {
	path    string
	modTime time.Time
}

func pruneGoldIVRShadowArtifacts(dir string, retentionDays, maxFiles int) error {
	if retentionDays <= 0 && maxFiles <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	kept := make([]shadowArtifactFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if retentionDays > 0 && info.ModTime().UTC().Before(cutoff) {
			_ = os.Remove(path)
			continue
		}
		kept = append(kept, shadowArtifactFile{path: path, modTime: info.ModTime()})
	}
	if maxFiles > 0 && len(kept) > maxFiles {
		for i := 0; i < len(kept); i++ {
			for j := i + 1; j < len(kept); j++ {
				if kept[j].modTime.After(kept[i].modTime) {
					kept[i], kept[j] = kept[j], kept[i]
				}
			}
		}
		for _, f := range kept[maxFiles:] {
			_ = os.Remove(f.path)
		}
	}
	return nil
}

func writeGoldIVRShadowFailureArtifact(dir string, artifact GoldIVRShadowArtifact) (string, error) {
	artifact.Success = false
	artifact.ShadowOnly = true
	artifact.WroteRollups = false
	artifact.UpdatedStreamMetadata = false
	return writeGoldIVRShadowArtifact(dir, artifact)
}

func sanitizeArtifactToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	return out
}

// gqlPriorityFromIVRRollups scores how urgently canonical GQL should follow IVR shadow signal.
func gqlPriorityFromIVRRollups(rollups []MinuteRollup, messageCount int) string {
	if len(rollups) == 0 || messageCount <= 0 {
		return "normal"
	}
	maxChat := 0
	total := 0
	for _, r := range rollups {
		if r.ChatCount > maxChat {
			maxChat = r.ChatCount
		}
		total += r.ChatCount
	}
	avgPerMin := float64(total) / float64(len(rollups))
	switch {
	case maxChat >= 200 || (avgPerMin >= 80 && maxChat >= 120):
		return "urgent"
	case maxChat >= 80 || avgPerMin >= 40:
		return "high"
	default:
		return "normal"
	}
}

func peakOverlapTopN(shadow, canonical []MinuteRollup, n int) float64 {
	shadowCounts := rollupMinuteCounts(shadow)
	canonCounts := rollupMinuteCounts(canonical)
	return peakOverlapFromCounts(shadowCounts, canonCounts, n)
}

func rollupMinuteCounts(rollups []MinuteRollup) map[int64]int {
	out := map[int64]int{}
	for _, r := range rollups {
		if r.ChatCount <= 0 {
			continue
		}
		key := r.MinuteTS.UTC().Truncate(time.Minute).Unix()
		out[key] += r.ChatCount
	}
	return out
}

func peakOverlapFromCounts(left, right map[int64]int, n int) float64 {
	leftPeaks := topPeakMinuteKeys(left, n)
	rightPeaks := topPeakMinuteKeys(right, n)
	if len(leftPeaks) == 0 && len(rightPeaks) == 0 {
		return 100.0
	}
	if len(leftPeaks) == 0 || len(rightPeaks) == 0 {
		return 0.0
	}
	union := map[int64]struct{}{}
	intersect := 0
	for k := range leftPeaks {
		union[k] = struct{}{}
		if _, ok := rightPeaks[k]; ok {
			intersect++
		}
	}
	for k := range rightPeaks {
		union[k] = struct{}{}
	}
	if len(union) == 0 {
		return 0.0
	}
	return float64(intersect) / float64(len(union)) * 100.0
}

func topPeakMinuteKeys(counts map[int64]int, n int) map[int64]struct{} {
	type pair struct {
		key   int64
		count int
	}
	ranked := make([]pair, 0, len(counts))
	for k, c := range counts {
		if c > 0 {
			ranked = append(ranked, pair{k, c})
		}
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].count > ranked[i].count || (ranked[j].count == ranked[i].count && ranked[j].key < ranked[i].key) {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	out := map[int64]struct{}{}
	limit := n
	if limit > len(ranked) {
		limit = len(ranked)
	}
	for i := 0; i < limit; i++ {
		out[ranked[i].key] = struct{}{}
	}
	return out
}

func reconciliationScorePct(shadow, canonical []MinuteRollup) float64 {
	score, _ := shadowCompareRollups(canonical, shadow)
	return score
}

func reconciliationRecommendation(scorePct, peakTop3 float64) string {
	switch {
	case scorePct >= 95 && peakTop3 >= 66:
		return "shadow_reconciled_peaks_only"
	case scorePct >= 85 || peakTop3 >= 66:
		return "shadow_reconciled_hold"
	default:
		return "shadow_reconciled_reject"
	}
}
