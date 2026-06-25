package archive

import (
	"encoding/json"
	"time"
)

// SilverManifestSidecar accompanies silver rollup exports (TASK-013).
type SilverManifestSidecar struct {
	SchemaVersion  string    `json:"schemaVersion"`
	StreamID       string    `json:"streamId"`
	Login          string    `json:"login,omitempty"`
	CoverageRatio  float64   `json:"coverageRatio"`
	ExpectedMinutes int      `json:"expectedMinutes"`
	RollupMinutes  int       `json:"rollupMinutes"`
	Status         string    `json:"status"`
	ExportedAt     time.Time `json:"exportedAt"`
}

func ComputeSilverCoverage(rollups []RollupExportLine, durationMinutes int, minCoverage float64) (ratio float64, status string) {
	if minCoverage <= 0 {
		minCoverage = 0.5
	}
	withSamples := 0
	for _, line := range rollups {
		if line.ViewerSamples > 0 {
			withSamples++
		}
	}
	expected := durationMinutes
	if expected <= 0 && len(rollups) > 0 {
		expected = len(rollups)
	}
	if expected <= 0 {
		return 0, StatusFailed
	}
	ratio = float64(withSamples) / float64(expected)
	switch {
	case ratio >= minCoverage:
		return ratio, StatusConfirmed
	case ratio > 0:
		return ratio, StatusPartial
	default:
		return 0, StatusFailed
	}
}

func BuildSilverSidecar(streamID, login string, rollups []RollupExportLine, durationMinutes int, minCoverage float64) SilverManifestSidecar {
	ratio, status := ComputeSilverCoverage(rollups, durationMinutes, minCoverage)
	withSamples := 0
	for _, line := range rollups {
		if line.ViewerSamples > 0 {
			withSamples++
		}
	}
	return SilverManifestSidecar{
		SchemaVersion:   "silver_manifest/v1",
		StreamID:        streamID,
		Login:           login,
		CoverageRatio:   ratio,
		ExpectedMinutes: durationMinutes,
		RollupMinutes:   withSamples,
		Status:          status,
		ExportedAt:      time.Now().UTC(),
	}
}

func SilverSidecarBlobKey(streamID string) string {
	return "manifests/silver/stream_id=" + streamID + ".json"
}

func HiveViewerRollupBlobKey(streamID string) string {
	return "viewer_rollup/v1/stream_id=" + streamID + "/part-000.jsonl.gz"
}

func (s SilverManifestSidecar) JSON() ([]byte, error) {
	return json.Marshal(s)
}
