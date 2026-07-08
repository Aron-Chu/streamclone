package ingestcore

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"streamclone/internal/metrics"
)

// IRCConn abstracts websocket IRC join/part.
type IRCConn interface {
	Join(ctx context.Context, channel string)
	Part(ctx context.Context, channel string)
}

// ActiveChannel tracks one admitted IRC collector.
type ActiveChannel struct {
	Login         string
	StreamID      string
	Tier          IngestTier
	TrackPriority int
	AdmittedAt    time.Time
	LastSeenAt    time.Time
}

// ReconcileResult summarizes one admission reconcile pass.
type ReconcileResult struct {
	Joined   []string
	Parted   []string
	Active   int
	Desired  int
	AdmitLag time.Duration
}

// CollectorManager owns IRC admission up to maxActive slots.
type CollectorManager struct {
	cfg    Config
	irc    IRCConn
	log    *slog.Logger
	runCtx context.Context

	mu       sync.Mutex
	active   map[string]*ActiveChannel
	desired  map[string]DesiredChannel
	joins1m  int
	parts1m  int
	windowAt time.Time
}

// NewCollectorManager builds a manager. maxActive comes from cfg.MaxActiveIRC.
func NewCollectorManager(cfg Config, irc IRCConn, log *slog.Logger) *CollectorManager {
	if log == nil {
		log = slog.Default()
	}
	return &CollectorManager{
		cfg:     cfg,
		irc:     irc,
		log:     log,
		active:  make(map[string]*ActiveChannel),
		desired: make(map[string]DesiredChannel),
	}
}

// SetRunContext stores the process context for join/part calls.
func (m *CollectorManager) SetRunContext(ctx context.Context) {
	m.mu.Lock()
	m.runCtx = ctx
	m.mu.Unlock()
}

// TouchAdmissionObservation refreshes idle clock for steady-state roster rows (anti-churn).
func (m *CollectorManager) TouchAdmissionObservation(login string) {
	login = normalizeLogin(login)
	if login == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.active[login]; ok {
		ch.LastSeenAt = time.Now().UTC()
	}
}

// Reconcile admits desired IRC channels and parts stale ones.
func (m *CollectorManager) Reconcile(candidates []DesiredChannel) ReconcileResult {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rotateRateWindow(now)

	wantIRC := make(map[string]DesiredChannel)
	for _, c := range candidates {
		login := normalizeLogin(c.Login)
		if login == "" || !c.Tier.WantsFullIRC() {
			continue
		}
		c.Login = login
		wantIRC[login] = c
	}
	m.desired = wantIRC

	// Part channels no longer desired.
	for login := range m.active {
		if _, ok := wantIRC[login]; !ok {
			m.partLocked(login)
		}
	}

	// Sort candidates by tier desc, then helix rank asc.
	type item struct {
		login string
		d     DesiredChannel
	}
	order := make([]item, 0, len(wantIRC))
	for login, d := range wantIRC {
		order = append(order, item{login: login, d: d})
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].d.Tier != order[j].d.Tier {
			return order[i].d.Tier < order[j].d.Tier
		}
		if order[i].d.TrackPriority != order[j].d.TrackPriority {
			return order[i].d.TrackPriority > order[j].d.TrackPriority
		}
		if order[i].d.HelixRank != order[j].d.HelixRank {
			if order[i].d.HelixRank <= 0 {
				return false
			}
			if order[j].d.HelixRank <= 0 {
				return true
			}
			return order[i].d.HelixRank < order[j].d.HelixRank
		}
		return order[i].login < order[j].login
	})

	for _, it := range order {
		if _, ok := m.active[it.login]; ok {
			ch := m.active[it.login]
			ch.LastSeenAt = now
			ch.StreamID = it.d.StreamID
			ch.Tier = it.d.Tier
			ch.TrackPriority = it.d.TrackPriority
			continue
		}
		if len(m.active) >= m.cfg.MaxActiveIRC {
			// Evict lowest tier / longest idle non-P0.
			if victim := m.pickEvictionVictimLocked(it.d); victim != "" {
				m.partLocked(victim)
			} else {
				continue
			}
		}
		m.joinLocked(it.login, it.d, now)
	}

	result := ReconcileResult{
		Active:  len(m.active),
		Desired: len(wantIRC),
	}
	if result.Desired > result.Active {
		result.AdmitLag = time.Duration(result.Desired-result.Active) * time.Second
	}
	m.publishMetricsLocked(result, now)
	return result
}

func (m *CollectorManager) joinLocked(login string, d DesiredChannel, now time.Time) {
	ctx := m.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if m.irc != nil {
		m.irc.Join(ctx, login)
	}
	m.active[login] = &ActiveChannel{
		Login:         login,
		StreamID:      d.StreamID,
		Tier:          d.Tier,
		TrackPriority: d.TrackPriority,
		AdmittedAt:    now,
		LastSeenAt:    now,
	}
	m.joins1m++
}

func (m *CollectorManager) partLocked(login string) {
	ctx := m.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if m.irc != nil {
		m.irc.Part(ctx, login)
	}
	delete(m.active, login)
	m.parts1m++
}

func (m *CollectorManager) pickEvictionVictimLocked(incoming DesiredChannel) string {
	if incoming.Tier == TierP0Always {
		// P0 never preempted by eviction picker for lower tiers only.
	}
	var victim string
	var victimTier IngestTier = TierP0Always
	var victimIdle time.Duration
	for login, ch := range m.active {
		if ch.Tier == TierP0Always {
			continue
		}
		if incoming.Tier == TierP0Always || int(incoming.Tier) < int(ch.Tier) ||
			(incoming.Tier == ch.Tier && incoming.TrackPriority > ch.TrackPriority) {
			idle := time.Since(ch.LastSeenAt)
			if victim == "" || ch.Tier > victimTier || (ch.Tier == victimTier && idle > victimIdle) {
				victim = login
				victimTier = ch.Tier
				victimIdle = idle
			}
		}
	}
	return victim
}

func (m *CollectorManager) rotateRateWindow(now time.Time) {
	if m.windowAt.IsZero() || now.Sub(m.windowAt) >= time.Minute {
		if !m.windowAt.IsZero() {
			metrics.IngestIRCJoinRate.Set(float64(m.joins1m))
			metrics.IngestIRCPartRate.Set(float64(m.parts1m))
		}
		m.joins1m = 0
		m.parts1m = 0
		m.windowAt = now
	}
}

func (m *CollectorManager) publishMetricsLocked(result ReconcileResult, now time.Time) {
	metrics.IngestActiveCollectors.Set(float64(result.Active))
	metrics.IngestDesiredCollectors.Set(float64(result.Desired))
	metrics.IngestAdmitLagSeconds.Set(result.AdmitLag.Seconds())
}

// Snapshot returns current manager state for hub/API.
type ManagerSnapshot struct {
	ActiveCollectors  int
	DesiredCollectors int
	AdmitLagSeconds   float64
	JoinRate1m        float64
	PartRate1m        float64
}

func (m *CollectorManager) Snapshot() ManagerSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := ManagerSnapshot{
		ActiveCollectors:  len(m.active),
		DesiredCollectors: len(m.desired),
		JoinRate1m:        float64(m.joins1m),
		PartRate1m:        float64(m.parts1m),
	}
	if len(m.desired) > len(m.active) {
		snap.AdmitLagSeconds = float64(len(m.desired)-len(m.active)) * 1.0
	}
	return snap
}

// IsActiveLogin reports whether login has an active IRC collector.
func (m *CollectorManager) IsActiveLogin(login string) bool {
	login = normalizeLogin(login)
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.active[login]
	return ok
}

// ActiveLogins returns a copy of active IRC logins.
func (m *CollectorManager) ActiveLogins() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.active))
	for login := range m.active {
		out = append(out, login)
	}
	sort.Strings(out)
	return out
}
