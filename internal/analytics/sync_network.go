package analytics

import (
	"context"
	"net/http"
	"sort"
	"time"

	"streamclone/internal/analytics/netmeter"
	"streamclone/internal/metrics"
)

type syncNetworkCtxKey struct{}

type syncNetworkHandle struct {
	streamID string
	meter    *netmeter.Meter
	svc      *SyncService
}

type activeSyncState struct {
	channel string
	phase   SyncPhase
}

type syncActiveLabels struct {
	channel string
	phase   SyncPhase
}

func (s *SyncService) withSyncNetwork(ctx context.Context, streamID string) context.Context {
	meter := s.ensureSyncMeter(streamID)
	s.registerActiveSync(streamID)
	handle := &syncNetworkHandle{streamID: streamID, meter: meter, svc: s}
	return context.WithValue(ctx, syncNetworkCtxKey{}, handle)
}

func syncNetRecord(ctx context.Context, op string, n int64) {
	handle, _ := ctx.Value(syncNetworkCtxKey{}).(*syncNetworkHandle)
	if handle == nil || handle.meter == nil || n <= 0 {
		return
	}
	handle.meter.Record(op, n)
}

func (s *SyncService) ensureSyncMeter(streamID string) *netmeter.Meter {
	if v, ok := s.syncMeters.Load(streamID); ok {
		return v.(*netmeter.Meter)
	}
	meter := netmeter.NewMeter(func(op string, n int64) {
		s.recordSyncBytes(streamID, op, n)
	})
	actual, _ := s.syncMeters.LoadOrStore(streamID, meter)
	return actual.(*netmeter.Meter)
}

func (s *SyncService) registerActiveSync(streamID string) {
	s.activeSyncMu.Lock()
	if s.activeSyncRegistry == nil {
		s.activeSyncRegistry = make(map[string]*activeSyncState)
	}
	if _, ok := s.activeSyncRegistry[streamID]; !ok {
		s.activeSyncRegistry[streamID] = &activeSyncState{}
	}
	s.activeSyncMu.Unlock()
}

func (s *SyncService) unregisterActiveSync(streamID string) {
	s.clearActiveSyncGauge(streamID)
	s.activeSyncMu.Lock()
	delete(s.activeSyncRegistry, streamID)
	s.activeSyncMu.Unlock()
	s.syncMeters.Delete(streamID)
}

func (s *SyncService) setSyncChannel(streamID, channel string) {
	channel = normalizeLogin(channel)
	if channel == "" {
		return
	}
	s.activeSyncMu.Lock()
	if s.activeSyncRegistry == nil {
		s.activeSyncRegistry = make(map[string]*activeSyncState)
	}
	st, ok := s.activeSyncRegistry[streamID]
	if !ok {
		st = &activeSyncState{}
		s.activeSyncRegistry[streamID] = st
	}
	st.channel = channel
	s.activeSyncMu.Unlock()
}

func (s *SyncService) syncChannel(streamID string) string {
	s.activeSyncMu.RLock()
	defer s.activeSyncMu.RUnlock()
	if st, ok := s.activeSyncRegistry[streamID]; ok && st != nil {
		return st.channel
	}
	return ""
}

func (s *SyncService) setSyncActiveGauge(streamID, channel string, phase SyncPhase) {
	if channel == "" {
		channel = s.syncChannel(streamID)
	}
	if channel == "" {
		channel = "unknown"
	}
	if prev, ok := s.syncActivePhase.Load(streamID); ok {
		labels := prev.(syncActiveLabels)
		if labels.channel != "" && labels.phase != "" {
			metrics.AnalyticsSyncActive.WithLabelValues(labels.channel, string(labels.phase)).Set(0)
		}
	}
	metrics.AnalyticsSyncActive.WithLabelValues(channel, string(phase)).Set(1)
	s.syncActivePhase.Store(streamID, syncActiveLabels{channel: channel, phase: phase})

	s.activeSyncMu.Lock()
	if s.activeSyncRegistry == nil {
		s.activeSyncRegistry = make(map[string]*activeSyncState)
	}
	st, ok := s.activeSyncRegistry[streamID]
	if !ok {
		st = &activeSyncState{}
		s.activeSyncRegistry[streamID] = st
	}
	st.channel = channel
	st.phase = phase
	s.activeSyncMu.Unlock()
}

func (s *SyncService) clearActiveSyncGauge(streamID string) {
	if prev, ok := s.syncActivePhase.LoadAndDelete(streamID); ok {
		labels := prev.(syncActiveLabels)
		if labels.channel != "" && labels.phase != "" {
			metrics.AnalyticsSyncActive.WithLabelValues(labels.channel, string(labels.phase)).Set(0)
		}
	}
}

func (s *SyncService) recordSyncBytes(streamID, op string, n int64) {
	if n <= 0 {
		return
	}
	channel := s.syncChannel(streamID)
	if channel == "" {
		channel = "unknown"
	}
	metrics.AnalyticsSyncBytesTotal.WithLabelValues(channel, op).Add(float64(n))
}

func (s *SyncService) updateNetworkUsage(status *SyncStatus) {
	if status == nil {
		return
	}
	v, ok := s.syncMeters.Load(status.StreamID)
	if !ok {
		return
	}
	meter, _ := v.(*netmeter.Meter)
	if meter == nil {
		return
	}
	snap := meter.Snapshot()
	status.Network = &SyncNetworkUsage{
		TrackerScrapeBytes: snap.TrackerScrapeBytes,
		GQLFetchBytes:      snap.GQLFetchBytes,
		EmotePreloadBytes:  snap.EmotePreloadBytes,
		HelixBytes:         snap.HelixBytes,
		TotalBytes:         snap.TotalBytes,
		LastRateBps:        snap.LastRateBps,
	}
	if status.Channel == "" {
		status.Channel = s.syncChannel(status.StreamID)
	}
}

type ActiveSyncItem struct {
	StreamID string      `json:"streamId"`
	Channel  string      `json:"channel,omitempty"`
	Phase    SyncPhase   `json:"phase,omitempty"`
	Status   *SyncStatus `json:"status,omitempty"`
}

func (s *SyncService) attachSyncNetwork(ctx context.Context, streamID, channel string) context.Context {
	ctx = s.withSyncNetwork(ctx, streamID)
	s.setSyncChannel(streamID, channel)
	return ctx
}

func (s *SyncService) setActiveSyncPhase(streamID string, phase SyncPhase) {
	s.setSyncActiveGauge(streamID, "", phase)
}

func (s *SyncService) detachSyncNetwork(streamID string) {
	s.unregisterActiveSync(streamID)
}

func (s *SyncService) syncCountingClient(ctx context.Context, op string, base *http.Client) *http.Client {
	handle, _ := ctx.Value(syncNetworkCtxKey{}).(*syncNetworkHandle)
	fallback := s.client
	if base != nil {
		fallback = base
	}
	if handle == nil || handle.meter == nil {
		return fallback
	}
	var rt http.RoundTripper
	if base != nil && base.Transport != nil {
		rt = base.Transport
	}
	return &http.Client{
		Timeout:   fallback.Timeout,
		Transport: netmeter.NewCountingTransport(rt, handle.meter, op),
	}
}

func (s *SyncService) gqlHTTPClient(ctx context.Context) *http.Client {
	return s.syncCountingClient(ctx, netmeter.OpGQL, s.gqlHTTPClientLegacy())
}

func (s *SyncService) gqlHTTPClientLegacy() *http.Client {
	if s.gqlClient != nil {
		return s.gqlClient
	}
	return s.client
}

func (s *SyncService) syncTrackerClient(ctx context.Context) *http.Client {
	return s.syncCountingClient(ctx, netmeter.OpTracker, s.client)
}

func (s *SyncService) syncEmoteClient(ctx context.Context) *http.Client {
	return s.syncCountingClient(ctx, netmeter.OpEmote, &http.Client{Timeout: 30 * time.Second})
}

func (s *SyncService) ListActiveSyncs(ctx context.Context) []ActiveSyncItem {
	s.activeSyncMu.RLock()
	ids := make([]string, 0, len(s.activeSyncRegistry))
	states := make(map[string]*activeSyncState, len(s.activeSyncRegistry))
	for streamID, st := range s.activeSyncRegistry {
		ids = append(ids, streamID)
		states[streamID] = st
	}
	s.activeSyncMu.RUnlock()

	sort.Strings(ids)
	out := make([]ActiveSyncItem, 0, len(ids))
	for _, streamID := range ids {
		item := ActiveSyncItem{StreamID: streamID}
		if st := states[streamID]; st != nil {
			item.Channel = st.channel
			item.Phase = st.phase
		}
		if status, err := s.GetSyncStatus(ctx, streamID); err == nil && status != nil {
			s.updateNetworkUsage(status)
			item.Status = status
			if item.Channel == "" {
				item.Channel = status.Channel
			}
			if item.Phase == "" {
				item.Phase = status.Phase
			}
		}
		out = append(out, item)
	}
	return out
}
