package analytics

import (
	"context"
	"strings"

	"streamclone/internal/archive"
)

// StoredArtifactSlot is a hosted-safe artifact readiness slot (no blob URLs or raw chat).
type StoredArtifactSlot struct {
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Provider    string `json:"provider,omitempty"`
	ByteSize    int64  `json:"byteSize,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
	UpdatedAt   int64  `json:"updatedAt,omitempty"`
	CanRestore  bool   `json:"canRestore,omitempty"`
	CanBackfill bool   `json:"canBackfill,omitempty"`
}

// StoredArtifactsSummary exposes sanitized export readiness for portal/extension clients.
// Field names avoid forbidden portal substrings such as "rollups" and "archive".
type StoredArtifactsSummary struct {
	MinuteSeries StoredArtifactSlot `json:"minuteSeries"`
	ChatExport   StoredArtifactSlot `json:"chatExport"`
	Session      StoredArtifactSlot `json:"session,omitempty"`
}

const (
	storedArtifactMissing = "missing"
	storedArtifactPending = "pending"
	storedArtifactReady   = "ready"
	storedArtifactPartial = "partial"
	storedArtifactFailed  = "failed"
)

func emptyStoredArtifacts() StoredArtifactsSummary {
	return StoredArtifactsSummary{
		MinuteSeries: StoredArtifactSlot{Kind: "minute_series", State: storedArtifactMissing},
		ChatExport:   StoredArtifactSlot{Kind: "chat_export", State: storedArtifactMissing},
		Session:      StoredArtifactSlot{Kind: "session", State: storedArtifactMissing},
	}
}

func ptrStoredArtifacts(summary StoredArtifactsSummary) *StoredArtifactsSummary {
	if summary.MinuteSeries.State == storedArtifactMissing &&
		summary.ChatExport.State == storedArtifactMissing &&
		summary.Session.State == storedArtifactMissing {
		return nil
	}
	return &summary
}

func (h *Handler) storedArtifactsForStream(ctx context.Context, streamID string) StoredArtifactsSummary {
	if h == nil || h.store == nil {
		return emptyStoredArtifacts()
	}
	pool := h.store.Pool()
	if pool == nil {
		return emptyStoredArtifacts()
	}
	manifest := archive.NewManifestStore(pool)
	rows, err := manifest.StreamExportRows(ctx, streamID)
	if err != nil || len(rows) == 0 {
		return emptyStoredArtifacts()
	}
	return buildStoredArtifactsSummary(rows)
}

func buildStoredArtifactsSummary(rows []archive.StreamExportRow) StoredArtifactsSummary {
	out := emptyStoredArtifacts()
	for _, row := range rows {
		slot := mapExportRowToSlot(row)
		switch row.ArtifactType {
		case archive.ArtifactAnalyticsRollups:
			out.MinuteSeries = mergeStoredSlot(out.MinuteSeries, slot)
		case archive.ArtifactVODChatMessage:
			out.ChatExport = mergeStoredSlot(out.ChatExport, slot)
		case archive.ArtifactAnalyticsStream:
			out.Session = mergeStoredSlot(out.Session, slot)
		}
	}
	out.MinuteSeries.Kind = "minute_series"
	out.ChatExport.Kind = "chat_export"
	out.Session.Kind = "session"
	return out
}

func mergeStoredSlot(current, incoming StoredArtifactSlot) StoredArtifactSlot {
	if slotRank(incoming.State) > slotRank(current.State) {
		return incoming
	}
	if slotRank(incoming.State) == slotRank(current.State) && incoming.UpdatedAt > current.UpdatedAt {
		return incoming
	}
	return current
}

func slotRank(state string) int {
	switch state {
	case storedArtifactReady:
		return 4
	case storedArtifactPartial:
		return 3
	case storedArtifactPending:
		return 2
	case storedArtifactFailed:
		return 1
	default:
		return 0
	}
}

func mapExportRowToSlot(row archive.StreamExportRow) StoredArtifactSlot {
	state := mapExportStatus(row.ExportStatus)
	slot := StoredArtifactSlot{
		State:     state,
		Provider:  sanitizeStorageProvider(row.Provider),
		ByteSize:  row.ByteSize,
		Checksum:  sanitizeChecksum(row.ContentSHA256),
		UpdatedAt: exportTimestamp(row),
	}
	switch row.ArtifactType {
	case archive.ArtifactAnalyticsRollups:
		slot.Kind = "minute_series"
		slot.CanRestore = state == storedArtifactReady || state == storedArtifactPartial
	case archive.ArtifactVODChatMessage:
		slot.Kind = "chat_export"
		slot.CanBackfill = state == storedArtifactReady || state == storedArtifactPartial
	case archive.ArtifactAnalyticsStream:
		slot.Kind = "session"
		slot.CanRestore = state == storedArtifactReady || state == storedArtifactPartial
	}
	return slot
}

func mapExportStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case archive.StatusConfirmed, archive.StatusComplete:
		return storedArtifactReady
	case archive.StatusPartial:
		return storedArtifactPartial
	case archive.StatusPending:
		return storedArtifactPending
	case archive.StatusFailed:
		return storedArtifactFailed
	default:
		if status == "" {
			return storedArtifactMissing
		}
		return storedArtifactPending
	}
}

func sanitizeStorageProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch {
	case p == "":
		return ""
	case strings.Contains(p, "r2"):
		return "r2"
	case strings.Contains(p, "azure"), strings.Contains(p, "blob"):
		return "azure"
	default:
		return p
	}
}

func sanitizeChecksum(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 16 {
		return raw
	}
	return raw[:16]
}

func exportTimestamp(row archive.StreamExportRow) int64 {
	if row.UpdatedAt.IsZero() {
		if row.ExportedAt != nil && !row.ExportedAt.IsZero() {
			return row.ExportedAt.UTC().UnixMilli()
		}
		return 0
	}
	return row.UpdatedAt.UTC().UnixMilli()
}

func enrichCoverageWithStoredArtifacts(c ExtensionCoverage, stored StoredArtifactsSummary, vodID string, isLive bool) ExtensionCoverage {
	chat := stored.ChatExport
	minute := stored.MinuteSeries

	switch chat.State {
	case storedArtifactReady, storedArtifactPartial:
		if strings.TrimSpace(vodID) != "" && (c.HasGaps || c.CoverageStartOffsetSeconds > coverageStartToleranceSec) {
			if !c.CanBackfill && c.State != CoverageStateBackfillRunning {
				c.CanBackfill = true
				c.BackfillReason = "stored_export_ready"
			}
		}
		switch {
		case c.State == CoverageStateWaitingForVOD && isLive:
			c.VODStatus = "export_ready"
			c.Message = "Stored VOD chat export ready — Twitch VOD link still pending"
		case c.State == CoverageStateWaitingForVOD && !isLive:
			c.VODStatus = "export_ready"
			if strings.TrimSpace(vodID) != "" {
				c.CanBackfill = true
				c.BackfillReason = "stored_export_ready"
				c.Message = "VOD chat export ready — load missed moments when ready"
			} else {
				c.Message = "VOD chat export ready — waiting for Twitch VOD link"
				c.ManualRetryAllowed = true
			}
		case c.State == CoverageStateVODUnavailable && strings.TrimSpace(vodID) != "":
			c.CanBackfill = true
			c.BackfillReason = "stored_export_ready"
			c.ManualRetryAllowed = true
			c.Message = "VOD chat export ready — retry missed moments"
		case c.VODStatus == "waiting" || c.VODStatus == "":
			c.VODStatus = "export_ready"
		}
	case storedArtifactPending:
		if c.State == CoverageStateWaitingForVOD {
			c.VODStatus = "export_pending"
			c.Message = "VOD chat export in progress — try again soon"
		}
	case storedArtifactFailed:
		if c.State == CoverageStateWaitingForVOD || c.State == CoverageStateVODUnavailable {
			c.VODStatus = "export_failed"
			if c.Message == "" || strings.Contains(strings.ToLower(c.Message), "archive publishes") {
				c.Message = "VOD chat export failed — retry after the stream ends"
			}
		}
	case storedArtifactMissing:
		if c.State == CoverageStateWaitingForVOD && !isLive {
			c.VODStatus = "export_missing"
			c.Message = "Waiting for VOD chat export after the stream ends"
		}
	}

	if minute.State == storedArtifactReady || minute.State == storedArtifactPartial {
		if c.CopyKey == "" {
			c.CopyKey = c.State
		}
	}

	c = decoratePulseCoverage(c, vodID)

	switch chat.State {
	case storedArtifactReady, storedArtifactPartial:
		if strings.TrimSpace(vodID) == "" {
			c.VODStatus = "export_ready"
		}
	case storedArtifactPending:
		if c.State == CoverageStateWaitingForVOD {
			c.VODStatus = "export_pending"
		}
	case storedArtifactFailed:
		if c.State == CoverageStateWaitingForVOD || c.State == CoverageStateVODUnavailable {
			c.VODStatus = "export_failed"
		}
	case storedArtifactMissing:
		if c.State == CoverageStateWaitingForVOD && !isLive {
			c.VODStatus = "export_missing"
		}
	}

	return c
}

func portalBadgesFromStored(stored StoredArtifactsSummary, sources []SourceStatus) []PortalDataSourceBadge {
	badges := portalBadgesFromSources(sources)
	seen := make(map[string]struct{}, len(badges)+3)
	for _, badge := range badges {
		seen[strings.ToLower(strings.TrimSpace(badge.Source))] = struct{}{}
	}
	appendBadge := func(source, state, label string) {
		if state == storedArtifactMissing || state == "" {
			return
		}
		key := strings.ToLower(strings.TrimSpace(source))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		badges = append(badges, PortalDataSourceBadge{Source: source, State: state, Label: label})
	}
	appendBadge("stored_minute_series", stored.MinuteSeries.State, "Stored minute series")
	appendBadge("stored_chat_export", stored.ChatExport.State, "Stored VOD chat")
	if stored.Session.State != storedArtifactMissing {
		appendBadge("stored_session", stored.Session.State, "Stored session")
	}
	return badges
}

func mergeStoredSources(sources []SourceStatus, stored StoredArtifactsSummary) []SourceStatus {
	out := append([]SourceStatus(nil), sources...)
	if stored.MinuteSeries.State == storedArtifactReady || stored.MinuteSeries.State == storedArtifactPartial {
		out = append(out, SourceStatus{Source: "stored_minute_series", State: stored.MinuteSeries.State})
	}
	if stored.ChatExport.State == storedArtifactReady || stored.ChatExport.State == storedArtifactPartial {
		out = append(out, SourceStatus{Source: "stored_chat_export", State: stored.ChatExport.State})
	}
	return out
}
