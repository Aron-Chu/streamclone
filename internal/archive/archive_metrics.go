package archive

import "streamclone/internal/metrics"

func recordArchiveExportConfirmed(artifactType string) {
	metrics.RecordArchiveExportConfirmed(artifactType)
}

func recordArchiveExportFailed(artifactType string) {
	metrics.RecordArchiveExportFailed(artifactType)
}
