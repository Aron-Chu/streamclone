package analytics

import (
	"context"
	"testing"
	"time"
)

// fakeCollectorLeaseStore is an in-memory CollectorLeaseStore mirroring the
// Postgres claim/renew/release semantics so lease and manager logic can be
// tested without a database.
type fakeCollectorLeaseStore struct {
	leases map[string]CollectorLease
}

func newFakeCollectorLeaseStore() *fakeCollectorLeaseStore {
	return &fakeCollectorLeaseStore{leases: map[string]CollectorLease{}}
}

func (f *fakeCollectorLeaseStore) ClaimCollectorLease(_ context.Context, req CollectorLeaseClaim) (CollectorLease, bool, error) {
	req = req.normalized()
	expiresAt := req.Now.Add(req.TTL)
	next := CollectorLease{
		Login:               req.Login,
		StreamID:            req.StreamID,
		CollectorInstanceID: req.InstanceID,
		Priority:            req.Priority,
		ShardIndex:          req.ShardIndex,
		ShardCount:          req.ShardCount,
		State:               req.State,
		ClaimedAt:           req.Now,
		HeartbeatAt:         req.Now,
		ExpiresAt:           expiresAt,
		UpdatedAt:           req.Now,
	}
	existing, ok := f.leases[req.Login]
	if !ok {
		f.leases[req.Login] = next
		return next, true, nil
	}
	expired := !existing.ExpiresAt.After(req.Now)
	sameOwner := existing.CollectorInstanceID == req.InstanceID
	preempt := req.Priority > existing.Priority
	if expired || sameOwner || preempt {
		if sameOwner {
			next.ClaimedAt = existing.ClaimedAt
		}
		f.leases[req.Login] = next
		return next, true, nil
	}
	return existing, false, nil
}

func (f *fakeCollectorLeaseStore) RenewCollectorLease(_ context.Context, login, instanceID string, now time.Time, ttl time.Duration) (bool, error) {
	login = normalizeLogin(login)
	existing, ok := f.leases[login]
	if !ok || existing.CollectorInstanceID != instanceID {
		return false, nil
	}
	existing.HeartbeatAt = now
	existing.ExpiresAt = now.Add(ttl)
	existing.UpdatedAt = now
	f.leases[login] = existing
	return true, nil
}

func (f *fakeCollectorLeaseStore) ReleaseCollectorLease(_ context.Context, login, instanceID string) error {
	login = normalizeLogin(login)
	if existing, ok := f.leases[login]; ok && existing.CollectorInstanceID == instanceID {
		delete(f.leases, login)
	}
	return nil
}

// leaseFor returns the current lease for a login (test inspection helper).
func (f *fakeCollectorLeaseStore) leaseFor(login string) (CollectorLease, bool, error) {
	lease, ok := f.leases[normalizeLogin(login)]
	return lease, ok, nil
}

func (f *fakeCollectorLeaseStore) ListCollectorLeasesByInstance(_ context.Context, instanceID string, now time.Time) ([]CollectorLease, error) {
	var out []CollectorLease
	for _, lease := range f.leases {
		if lease.CollectorInstanceID == instanceID && lease.ExpiresAt.After(now) {
			out = append(out, lease)
		}
	}
	return out, nil
}

func mustClaim(t *testing.T, store CollectorLeaseStore, login, instance string, priority int, now time.Time, ttl time.Duration) (CollectorLease, bool) {
	t.Helper()
	lease, owned, err := store.ClaimCollectorLease(context.Background(), CollectorLeaseClaim{
		Login:      login,
		StreamID:   "s-" + login,
		InstanceID: instance,
		Priority:   priority,
		Now:        now,
		TTL:        ttl,
	})
	if err != nil {
		t.Fatalf("claim %s/%s: %v", login, instance, err)
	}
	return lease, owned
}

func TestCollectorLeaseClaimOnceAndDuplicatePrevented(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	now := time.Now().UTC()
	ttl := 90 * time.Second

	if _, owned := mustClaim(t, store, "xqc", "collector-0", 100, now, ttl); !owned {
		t.Fatal("first claim should succeed")
	}
	// A different instance must not be able to take an active, equal-priority lease.
	if _, owned := mustClaim(t, store, "xqc", "collector-1", 100, now, ttl); owned {
		t.Fatal("duplicate active lease must be prevented")
	}
}

func TestCollectorLeaseExpiredReclaim(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	now := time.Now().UTC()
	ttl := 90 * time.Second

	mustClaim(t, store, "xqc", "collector-0", 100, now, ttl)
	// After expiry a different instance can reclaim.
	later := now.Add(ttl + time.Second)
	if _, owned := mustClaim(t, store, "xqc", "collector-1", 50, later, ttl); !owned {
		t.Fatal("expired lease should be reclaimable")
	}
}

func TestCollectorLeaseHeartbeatExtends(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	now := time.Now().UTC()
	ttl := 90 * time.Second
	lease, _ := mustClaim(t, store, "xqc", "collector-0", 100, now, ttl)

	renewAt := now.Add(30 * time.Second)
	ok, err := store.RenewCollectorLease(context.Background(), "xqc", "collector-0", renewAt, ttl)
	if err != nil || !ok {
		t.Fatalf("heartbeat renew failed: ok=%v err=%v", ok, err)
	}
	leases, _ := store.ListCollectorLeasesByInstance(context.Background(), "collector-0", renewAt)
	if len(leases) != 1 {
		t.Fatalf("expected 1 active lease, got %d", len(leases))
	}
	if !leases[0].ExpiresAt.After(lease.ExpiresAt) {
		t.Fatalf("heartbeat should extend expiry: was %v now %v", lease.ExpiresAt, leases[0].ExpiresAt)
	}
	// Renewal by a non-owner must fail.
	if ok, _ := store.RenewCollectorLease(context.Background(), "xqc", "collector-1", renewAt, ttl); ok {
		t.Fatal("non-owner heartbeat must not extend lease")
	}
}

func TestCollectorLeaseGracefulRelease(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	now := time.Now().UTC()
	ttl := 90 * time.Second
	mustClaim(t, store, "xqc", "collector-0", 100, now, ttl)
	if err := store.ReleaseCollectorLease(context.Background(), "xqc", "collector-0"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, owned := mustClaim(t, store, "xqc", "collector-1", 10, now, ttl); !owned {
		t.Fatal("released lease should be immediately claimable")
	}
}

func TestCollectorLeasePriorityPreempt(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	now := time.Now().UTC()
	ttl := 90 * time.Second
	mustClaim(t, store, "xqc", "collector-0", 100, now, ttl)
	// Lower priority cannot preempt an active lease.
	if _, owned := mustClaim(t, store, "xqc", "collector-1", 50, now, ttl); owned {
		t.Fatal("lower priority must not preempt")
	}
	// Higher priority preempts.
	lease, owned := mustClaim(t, store, "xqc", "collector-2", 200, now, ttl)
	if !owned || lease.CollectorInstanceID != "collector-2" {
		t.Fatalf("higher priority should preempt, got owned=%v owner=%s", owned, lease.CollectorInstanceID)
	}
}

func TestCollectorLeaseManagerRespectsMaxChannels(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	mgr := NewCollectorLeaseManager(store, CollectorLeaseManagerConfig{
		InstanceID:  "collector-0",
		MaxChannels: 3,
		TTL:         90 * time.Second,
	})
	desired := []DesiredChannel{
		{Login: "a", Priority: 10}, {Login: "b", Priority: 9}, {Login: "c", Priority: 8},
		{Login: "d", Priority: 7}, {Login: "e", Priority: 6},
	}
	res, err := mgr.Reconcile(context.Background(), desired, time.Now().UTC())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.Owned) != 3 {
		t.Fatalf("expected 3 owned (capped), got %d", len(res.Owned))
	}
	if len(res.Skipped) != 2 {
		t.Fatalf("expected 2 skipped over cap, got %d", len(res.Skipped))
	}
	// The three highest priorities (a, b, c) must be the ones claimed.
	for _, login := range []string{"a", "b", "c"} {
		if _, ok := store.leases[login]; !ok {
			t.Fatalf("expected highest-priority %q to be claimed", login)
		}
	}
}

func TestCollectorLeaseManagerReleasesUndesired(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	mgr := NewCollectorLeaseManager(store, CollectorLeaseManagerConfig{
		InstanceID:  "collector-0",
		MaxChannels: 10,
		TTL:         90 * time.Second,
	})
	now := time.Now().UTC()
	if _, err := mgr.Reconcile(context.Background(), []DesiredChannel{{Login: "a", Priority: 5}, {Login: "b", Priority: 5}}, now); err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	// "b" drops out of the desired roster.
	res, err := mgr.Reconcile(context.Background(), []DesiredChannel{{Login: "a", Priority: 5}}, now.Add(time.Second))
	if err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	if len(res.Released) != 1 || res.Released[0] != "b" {
		t.Fatalf("expected b released, got %v", res.Released)
	}
	if _, ok := store.leases["b"]; ok {
		t.Fatal("b lease should be deleted after release")
	}
}

func TestCollectorLeaseManagerShardOwnership(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	shard0 := NewCollectorLeaseManager(store, CollectorLeaseManagerConfig{InstanceID: "c0", ShardIndex: 0, ShardCount: 2, MaxChannels: 100, TTL: 90 * time.Second})
	shard1 := NewCollectorLeaseManager(store, CollectorLeaseManagerConfig{InstanceID: "c1", ShardIndex: 1, ShardCount: 2, MaxChannels: 100, TTL: 90 * time.Second})

	logins := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"}
	for _, l := range logins {
		if shard0.OwnsShard(l) == shard1.OwnsShard(l) {
			t.Fatalf("login %q must belong to exactly one shard", l)
		}
	}
	desired := make([]DesiredChannel, 0, len(logins))
	for i, l := range logins {
		desired = append(desired, DesiredChannel{Login: l, Priority: i})
	}
	now := time.Now().UTC()
	r0, _ := shard0.Reconcile(context.Background(), desired, now)
	r1, _ := shard1.Reconcile(context.Background(), desired, now)
	if len(r0.Owned)+len(r1.Owned) != len(logins) {
		t.Fatalf("shards should partition all logins: %d + %d != %d", len(r0.Owned), len(r1.Owned), len(logins))
	}
	for _, l0 := range r0.Owned {
		for _, l1 := range r1.Owned {
			if l0.Login == l1.Login {
				t.Fatalf("login %q owned by both shards", l0.Login)
			}
		}
	}
}

// TestCollectorLeaseNoDuplicateOwnershipAcrossInstances models BearHost and
// laptopworker reconciling the same roster against the shared lease table; no
// login may end up owned by both.
func TestCollectorLeaseNoDuplicateOwnershipAcrossInstances(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	bearhost := NewCollectorLeaseManager(store, CollectorLeaseManagerConfig{InstanceID: "bearhost-0", MaxChannels: 10, TTL: 90 * time.Second})
	laptop := NewCollectorLeaseManager(store, CollectorLeaseManagerConfig{InstanceID: "laptopworker-0", MaxChannels: 10, TTL: 90 * time.Second})

	desired := []DesiredChannel{{Login: "xqc", Priority: 100}, {Login: "jynxzi", Priority: 90}, {Login: "caseoh", Priority: 80}}
	now := time.Now().UTC()
	rBear, _ := bearhost.Reconcile(context.Background(), desired, now)
	rLap, _ := laptop.Reconcile(context.Background(), desired, now)

	owners := map[string]string{}
	for _, l := range rBear.Owned {
		owners[l.Login] = "bearhost-0"
	}
	for _, l := range rLap.Owned {
		if prev, ok := owners[l.Login]; ok {
			t.Fatalf("login %q owned by both %s and laptopworker-0", l.Login, prev)
		}
		owners[l.Login] = "laptopworker-0"
	}
	// Whoever reconciled first owns all three; the other gets none.
	if len(rBear.Owned) != 3 || len(rLap.Owned) != 0 {
		t.Fatalf("expected first instance to own all, got bear=%d lap=%d", len(rBear.Owned), len(rLap.Owned))
	}
}

func TestCollectorForceUntrackPartsAndStops(t *testing.T) {
	joiner := &fakeJoiner{}
	c := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 5, time.Hour, time.Hour, 200)
	if !c.Watch(context.Background(), "xqc").Tracking {
		t.Fatal("expected initial Watch to track")
	}
	if !c.IsTracking("xqc") {
		t.Fatal("expected collector to track xqc before untrack")
	}
	c.ForceUntrack("XQC") // case-insensitive normalization
	if c.IsTracking("xqc") {
		t.Fatal("expected collector to stop tracking after ForceUntrack")
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "xqc" {
		t.Fatalf("expected IRC part for xqc, got %v", joiner.parted)
	}
	// Untracking an unknown login is a no-op and must not part again or panic.
	c.ForceUntrack("nobody")
	if len(joiner.parted) != 1 {
		t.Fatalf("unknown untrack should not part, got %v", joiner.parted)
	}
}

func TestForceUntrackNilCollectorAndNilIRCSafe(t *testing.T) {
	var nilC *Collector
	nilC.ForceUntrack("x") // must not panic
	// Collector with a nil joiner interface must not panic on part.
	c := NewCollector(&fakeStore{}, fakeProvider{}, nil, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	c.tracked["solo"] = &trackedChannel{login: "solo", refCounts: map[string]int{}}
	c.ForceUntrack("solo")
	if c.IsTracking("solo") {
		t.Fatal("expected solo untracked even with nil IRC")
	}
}

func TestSyncCollectorOwnershipTracksOwnedAndPartsReleased(t *testing.T) {
	joiner := &fakeJoiner{}
	c := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 10, time.Hour, time.Hour, 200)
	// Pre-track "dropme" so a Released entry has something to part.
	c.Watch(context.Background(), "dropme")
	joiner.joined = nil

	SyncCollectorOwnership(context.Background(), c, CollectorLeaseReconcileResult{
		Owned:    []CollectorLease{{Login: "keepme", Priority: TrackPriorityTopRoster}},
		Released: []string{"dropme"},
	})
	if !c.IsTracking("keepme") {
		t.Fatal("expected owned lease login to be tracked")
	}
	if c.IsTracking("dropme") {
		t.Fatal("expected released login to be untracked")
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "dropme" {
		t.Fatalf("expected part for dropme, got %v", joiner.parted)
	}
	// SyncCollectorOwnership must be nil-safe.
	SyncCollectorOwnership(context.Background(), nil, CollectorLeaseReconcileResult{Released: []string{"x"}})
}

// TestCollectorLeaseLostStopsCollection wires the lease manager and a live
// collector exactly as cmd/pulse-collector does: own a channel, then have
// another instance preempt it, and assert the heartbeat-lost lease causes the
// collector to part/untrack the login so two processes never write it.
func TestCollectorLeaseLostStopsCollection(t *testing.T) {
	leaseStore := newFakeCollectorLeaseStore()
	joiner := &fakeJoiner{}
	collector := NewCollector(&fakeStore{}, fakeProvider{}, joiner, nil, nilLogger(), 10, time.Hour, time.Hour, 200)
	mgr := NewCollectorLeaseManager(leaseStore, CollectorLeaseManagerConfig{InstanceID: "c0", MaxChannels: 10, TTL: 90 * time.Second})
	now := time.Now().UTC()

	res, err := mgr.Reconcile(context.Background(), []DesiredChannel{{Login: "a", StreamID: "s-a", Priority: 10}}, now)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	SyncCollectorOwnership(context.Background(), collector, res)
	if !collector.IsTracking("a") {
		t.Fatal("expected collector to track owned lease")
	}

	// Another instance preempts "a" with higher priority.
	mustClaim(t, leaseStore, "a", "c1", 999, now, 90*time.Second)
	_, lost, err := mgr.Heartbeat(context.Background(), now.Add(time.Second))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if len(lost) != 1 || lost[0] != "a" {
		t.Fatalf("expected a lost, got %v", lost)
	}
	// cmd/pulse-collector parts every lost login on heartbeat.
	for _, login := range lost {
		collector.ForceUntrack(login)
	}
	if collector.IsTracking("a") {
		t.Fatal("expected collector to stop tracking lost lease")
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "a" {
		t.Fatalf("expected IRC part for lost lease, got %v", joiner.parted)
	}
}

func TestCollectorLeaseManagerHeartbeatDropsLostLease(t *testing.T) {
	store := newFakeCollectorLeaseStore()
	mgr := NewCollectorLeaseManager(store, CollectorLeaseManagerConfig{InstanceID: "c0", MaxChannels: 5, TTL: 90 * time.Second})
	now := time.Now().UTC()
	if _, err := mgr.Reconcile(context.Background(), []DesiredChannel{{Login: "a", Priority: 10}}, now); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Simulate another instance preempting "a" with higher priority.
	mustClaim(t, store, "a", "c1", 999, now, 90*time.Second)
	renewed, lost, err := mgr.Heartbeat(context.Background(), now.Add(time.Second))
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if len(renewed) != 0 || len(lost) != 1 || lost[0] != "a" {
		t.Fatalf("expected a to be lost, renewed=%v lost=%v", renewed, lost)
	}
	if got := mgr.Held(); len(got) != 0 {
		t.Fatalf("expected no held leases after loss, got %v", got)
	}
}
