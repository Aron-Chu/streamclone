package analytics

// IngestCoreAdmission delegates IRC admission to ingest-core when cutover is active.
type IngestCoreAdmission interface {
	OwnsIRCAdmission() bool
	RegisterProtectedGoLive(login, streamID string, trackPriority int)
	TouchAdmission(login string)
}
