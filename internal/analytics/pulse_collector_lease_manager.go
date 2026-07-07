package analytics

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// DesiredChannel is one entry of the roster a collector wants to own. Priority
// orders claims under the channel cap and drives preemption.
type DesiredChannel struct {
	Login    string
	StreamID string
	Priority int
}

// CollectorLeaseReconcileResult summarizes one reconcile pass so the caller can
// drive IRC join/part and emit metrics.
type CollectorLeaseReconcileResult struct {
	Owned    []CollectorLease // leases this instance currently holds -> ensure IRC joined
	Claimed  []string         // newly acquired this pass
	Renewed  []string         // still-held desired logins refreshed this pass
	Released []string         // previously held but no longer desired -> IRC part
	Skipped  []string         // desired but owned by another instance or over cap
}

// CollectorLeaseManager turns a desired roster into exclusive, lease-backed
// ownership for a single collector instance. It is safe for concurrent use by
// the heartbeat and rebalance loops. All persistence goes through the injected
// CollectorLeaseStore so the logic is unit-testable without Postgres.
//
// Runtime note (2026-07): not wired in cmd/analytics/main.go. Hosted production
// admission uses Top500PriorityWatchPoller plus the in-process Collector cap.
type CollectorLeaseManager struct {
	store       CollectorLeaseStore
	instanceID  string
	shardIndex  int
	shardCount  int
	maxChannels int
	ttl         time.Duration
	log         *slog.Logger

	mu   sync.Mutex
	held map[string]CollectorLease
}

// CollectorLeaseManagerConfig configures a manager.
type CollectorLeaseManagerConfig struct {
	InstanceID  string
	ShardIndex  int
	ShardCount  int
	MaxChannels int
	TTL         time.Duration
	Logger      *slog.Logger
}

// NewCollectorLeaseManager builds a manager. Defaults are filled for unset fields.
func NewCollectorLeaseManager(store CollectorLeaseStore, cfg CollectorLeaseManagerConfig) *CollectorLeaseManager {
	if cfg.ShardCount < 1 {
		cfg.ShardCount = 1
	}
	if cfg.ShardIndex < 0 || cfg.ShardIndex >= cfg.ShardCount {
		cfg.ShardIndex = 0
	}
	if cfg.MaxChannels <= 0 {
		cfg.MaxChannels = 50
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 90 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &CollectorLeaseManager{
		store:       store,
		instanceID:  cfg.InstanceID,
		shardIndex:  cfg.ShardIndex,
		shardCount:  cfg.ShardCount,
		maxChannels: cfg.MaxChannels,
		ttl:         cfg.TTL,
		log:         cfg.Logger,
		held:        map[string]CollectorLease{},
	}
}

// Cap returns the maximum number of leases this manager will hold concurrently.
func (m *CollectorLeaseManager) Cap() int {
	if m == nil {
		return 0
	}
	return m.maxChannels
}

// OwnsShard reports whether a login belongs to this collector's shard. With a
// single shard every login is owned.
func (m *CollectorLeaseManager) OwnsShard(login string) bool {
	if m.shardCount <= 1 {
		return true
	}
	login = normalizeLogin(login)
	h := fnv.New32a()
	_, _ = h.Write([]byte(login))
	return int(h.Sum32()%uint32(m.shardCount)) == m.shardIndex
}

// Held returns a snapshot of the logins this instance currently believes it owns.
func (m *CollectorLeaseManager) Held() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.held))
	for login := range m.held {
		out = append(out, login)
	}
	sort.Strings(out)
	return out
}

// Reconcile claims desired channels (shard-filtered, priority-ordered, capped),
// renews already-held desired channels, and releases held channels that are no
// longer desired. Returns the resulting ownership view.
func (m *CollectorLeaseManager) Reconcile(ctx context.Context, desired []DesiredChannel, now time.Time) (CollectorLeaseReconcileResult, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var result CollectorLeaseReconcileResult

	wanted := make(map[string]DesiredChannel)
	ordered := make([]DesiredChannel, 0, len(desired))
	for _, d := range desired {
		login := normalizeLogin(d.Login)
		if login == "" || !m.OwnsShard(login) {
			continue
		}
		if _, dup := wanted[login]; dup {
			continue
		}
		d.Login = login
		wanted[login] = d
		ordered = append(ordered, d)
	}
	// Highest priority first so the cap admits the most important channels.
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Priority > ordered[j].Priority
	})

	m.mu.Lock()
	defer m.mu.Unlock()

	// Release leases we hold that are no longer desired.
	for login := range m.held {
		if _, ok := wanted[login]; ok {
			continue
		}
		if err := m.store.ReleaseCollectorLease(ctx, login, m.instanceID); err != nil {
			m.log.Warn("collector lease release failed", "login", login, "err", err)
			continue
		}
		delete(m.held, login)
		result.Released = append(result.Released, login)
	}

	for _, d := range ordered {
		if existing, ok := m.held[d.Login]; ok {
			renewed, err := m.store.RenewCollectorLease(ctx, d.Login, m.instanceID, now, m.ttl)
			if err != nil {
				m.log.Warn("collector lease renew failed", "login", d.Login, "err", err)
				continue
			}
			if !renewed {
				// Lost the lease (reclaimed/preempted elsewhere); drop it.
				delete(m.held, d.Login)
				result.Skipped = append(result.Skipped, d.Login)
				continue
			}
			existing.ExpiresAt = now.Add(m.ttl)
			existing.HeartbeatAt = now
			m.held[d.Login] = existing
			result.Renewed = append(result.Renewed, d.Login)
			continue
		}
		if len(m.held) >= m.maxChannels {
			result.Skipped = append(result.Skipped, d.Login)
			continue
		}
		lease, owned, err := m.store.ClaimCollectorLease(ctx, CollectorLeaseClaim{
			Login:      d.Login,
			StreamID:   d.StreamID,
			InstanceID: m.instanceID,
			Priority:   d.Priority,
			ShardIndex: m.shardIndex,
			ShardCount: m.shardCount,
			State:      CollectorLeaseStateClaimed,
			Now:        now,
			TTL:        m.ttl,
		})
		if err != nil {
			m.log.Warn("collector lease claim failed", "login", d.Login, "err", err)
			continue
		}
		if !owned {
			result.Skipped = append(result.Skipped, d.Login)
			continue
		}
		m.held[d.Login] = lease
		result.Claimed = append(result.Claimed, d.Login)
	}

	result.Owned = m.heldSnapshotLocked()
	return result, nil
}

// Heartbeat renews every lease this instance currently holds. Leases that can no
// longer be renewed (reclaimed/preempted) are dropped from the held set and
// returned so the caller can stop collecting them.
func (m *CollectorLeaseManager) Heartbeat(ctx context.Context, now time.Time) (renewed []string, lost []string, err error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	logins := make([]string, 0, len(m.held))
	for login := range m.held {
		logins = append(logins, login)
	}
	sort.Strings(logins)
	for _, login := range logins {
		ok, renewErr := m.store.RenewCollectorLease(ctx, login, m.instanceID, now, m.ttl)
		if renewErr != nil {
			m.log.Warn("collector lease heartbeat failed", "login", login, "err", renewErr)
			continue
		}
		if !ok {
			delete(m.held, login)
			lost = append(lost, login)
			continue
		}
		lease := m.held[login]
		lease.HeartbeatAt = now
		lease.ExpiresAt = now.Add(m.ttl)
		m.held[login] = lease
		renewed = append(renewed, login)
	}
	return renewed, lost, nil
}

// ReleaseAll relinquishes every held lease — call on graceful shutdown.
func (m *CollectorLeaseManager) ReleaseAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for login := range m.held {
		if err := m.store.ReleaseCollectorLease(ctx, login, m.instanceID); err != nil {
			m.log.Warn("collector lease release-all failed", "login", login, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		delete(m.held, login)
	}
	return firstErr
}

// SyncCollectorOwnership applies a reconcile result to a live collector: it
// ensures IRC is joined/tracked for every owned lease and force-untracks every
// released login. This guarantees a relinquished stream stops being collected at
// once instead of lingering until the normal offline/idle path catches up — the
// key invariant that keeps two collector instances from writing the same stream.
func SyncCollectorOwnership(ctx context.Context, c *Collector, result CollectorLeaseReconcileResult) {
	if c == nil {
		return
	}
	for _, lease := range result.Owned {
		priority := lease.Priority
		if priority <= 0 {
			priority = TrackPriorityTopRoster
		}
		c.WatchWithPriority(ctx, lease.Login, "", priority)
	}
	for _, login := range result.Released {
		c.ForceUntrack(login)
	}
}

func (m *CollectorLeaseManager) heldSnapshotLocked() []CollectorLease {
	out := make([]CollectorLease, 0, len(m.held))
	for _, lease := range m.held {
		out = append(out, lease)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Login < out[j].Login
	})
	return out
}
