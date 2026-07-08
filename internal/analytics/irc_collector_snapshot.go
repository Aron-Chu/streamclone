package analytics

// ircCollectorSnapshot returns active IRC collector count and capacity.
// When ingest-core owns admission, use ingest-core manager state; otherwise legacy collector.
func (h *Handler) ircCollectorSnapshot() (active, max int) {
	if h == nil {
		return 0, 0
	}
	if h.ingestEngine != nil && h.ingestEngine.OwnsIRCAdmission() {
		snap := h.ingestEngine.ManagerSnapshot()
		cfg := h.ingestEngine.Config()
		max = cfg.MaxActiveIRC
		if max <= 0 {
			max = snap.DesiredCollectors
		}
		return snap.ActiveCollectors, max
	}
	if h.collector != nil {
		snap := h.collector.TrackingSnapshot()
		return snap.Active, snap.Max
	}
	return 0, 0
}

// isIRCActiveLogin reports whether login has an active IRC collector for readiness/hub coverage.
func (h *Handler) isIRCActiveLogin(login string) bool {
	if h == nil {
		return false
	}
	if h.ingestEngine != nil && h.ingestEngine.OwnsIRCAdmission() {
		return h.ingestEngine.IsActiveLogin(login)
	}
	if h.collector != nil {
		return h.collector.IsTracking(login)
	}
	return false
}

// ingestCoreOperational reports healthy ingest-core IRC when core writer owns admission.
func (h *Handler) ingestCoreOperational() bool {
	if h == nil || h.ingestEngine == nil || !h.ingestEngine.OwnsIRCAdmission() {
		return false
	}
	snap := h.ingestEngine.ManagerSnapshot()
	return snap.ActiveCollectors > 0
}

// corpusCriticalFromStaleLegacyCollector is true when corpus pipeline is critical only
// because legacy readiness still sees zero collector tracking while ingest-core is live.
func corpusCriticalFromStaleLegacyCollector(h *Handler, pipeline HubCorpusPipeline) bool {
	if pipeline.State != CorpusStatusCritical {
		return false
	}
	if pipeline.CollectorActive > 0 || pipeline.Roster.CollectorTracking > 0 {
		return false
	}
	return h != nil && h.ingestCoreOperational()
}
