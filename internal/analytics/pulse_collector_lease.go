package analytics

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Collector lease states. A lease moves claimed -> joining -> collecting while
// owned, and may be set to releasing during graceful shutdown before deletion.
const (
	CollectorLeaseStateClaimed    = "claimed"
	CollectorLeaseStateJoining    = "joining"
	CollectorLeaseStateCollecting = "collecting"
	CollectorLeaseStateReleasing  = "releasing"
)

// CollectorLease is one row in pulse_collector_leases — exclusive ownership of a
// login's IRC collection by a single collector instance until expiry.
type CollectorLease struct {
	Login               string    `json:"login"`
	StreamID            string    `json:"streamId"`
	CollectorInstanceID string    `json:"collectorInstanceId"`
	Priority            int       `json:"priority"`
	ShardIndex          int       `json:"shardIndex"`
	ShardCount          int       `json:"shardCount"`
	State               string    `json:"state"`
	ClaimedAt           time.Time `json:"claimedAt"`
	HeartbeatAt         time.Time `json:"heartbeatAt"`
	ExpiresAt           time.Time `json:"expiresAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// CollectorLeaseClaim is the input to ClaimCollectorLease.
type CollectorLeaseClaim struct {
	Login      string
	StreamID   string
	InstanceID string
	Priority   int
	ShardIndex int
	ShardCount int
	State      string
	Now        time.Time
	TTL        time.Duration
}

func (c CollectorLeaseClaim) normalized() CollectorLeaseClaim {
	c.Login = normalizeLogin(c.Login)
	if c.State == "" {
		c.State = CollectorLeaseStateClaimed
	}
	if c.ShardCount < 1 {
		c.ShardCount = 1
	}
	if c.ShardIndex < 0 {
		c.ShardIndex = 0
	}
	if c.Now.IsZero() {
		c.Now = time.Now().UTC()
	}
	if c.TTL <= 0 {
		c.TTL = 90 * time.Second
	}
	return c
}

// CollectorLeaseStore is the ownership surface used by the lease manager. Both
// *Store (Postgres) and the in-memory test fake satisfy it.
type CollectorLeaseStore interface {
	ClaimCollectorLease(ctx context.Context, req CollectorLeaseClaim) (CollectorLease, bool, error)
	RenewCollectorLease(ctx context.Context, login, instanceID string, now time.Time, ttl time.Duration) (bool, error)
	ReleaseCollectorLease(ctx context.Context, login, instanceID string) error
	ListCollectorLeasesByInstance(ctx context.Context, instanceID string, now time.Time) ([]CollectorLease, error)
}

// ClaimCollectorLease atomically claims (or re-claims) the lease for a login.
// The lease is granted when the row is free/expired, already owned by this
// instance, or the incoming priority strictly exceeds the current holder's
// (preemption). Returns the resulting lease and whether this instance owns it.
//
// All timestamps and expiry comparisons use the Postgres server clock (now()),
// not the caller's Now. Collector instances (especially laptopworker over
// Tailscale) can have meaningful clock skew; deriving claimed_at/heartbeat_at/
// expires_at and the "is it expired?" guard from a single authoritative DB clock
// prevents premature expiry or delayed reclaim from a fast/slow worker clock.
// req.Now is retained only for the in-memory test fake.
func (s *Store) ClaimCollectorLease(ctx context.Context, req CollectorLeaseClaim) (CollectorLease, bool, error) {
	req = req.normalized()
	if req.Login == "" || req.InstanceID == "" {
		return CollectorLease{}, false, errors.New("collector lease requires login and instance id")
	}
	ttlSeconds := req.TTL.Seconds()
	var lease CollectorLease
	err := s.db.QueryRow(ctx, `
		INSERT INTO pulse_collector_leases AS l (
			login, stream_id, collector_instance_id, priority, shard_index, shard_count,
			state, claimed_at, heartbeat_at, expires_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7, now(), now(), now() + make_interval(secs => $8), now())
		ON CONFLICT (login) DO UPDATE SET
			stream_id = EXCLUDED.stream_id,
			collector_instance_id = EXCLUDED.collector_instance_id,
			priority = EXCLUDED.priority,
			shard_index = EXCLUDED.shard_index,
			shard_count = EXCLUDED.shard_count,
			state = EXCLUDED.state,
			claimed_at = CASE
				WHEN l.collector_instance_id = EXCLUDED.collector_instance_id THEN l.claimed_at
				ELSE now()
			END,
			heartbeat_at = now(),
			expires_at = now() + make_interval(secs => $8),
			updated_at = now()
		WHERE l.expires_at <= now()
			OR l.collector_instance_id = EXCLUDED.collector_instance_id
			OR EXCLUDED.priority > l.priority
		RETURNING login, stream_id, collector_instance_id, priority, shard_index, shard_count,
			state, claimed_at, heartbeat_at, expires_at, updated_at`,
		req.Login, req.StreamID, req.InstanceID, req.Priority, req.ShardIndex, req.ShardCount,
		req.State, ttlSeconds,
	).Scan(
		&lease.Login, &lease.StreamID, &lease.CollectorInstanceID, &lease.Priority,
		&lease.ShardIndex, &lease.ShardCount, &lease.State, &lease.ClaimedAt,
		&lease.HeartbeatAt, &lease.ExpiresAt, &lease.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Conflict and the WHERE guard rejected the takeover: another active,
		// equal/higher-priority instance still owns the lease.
		current, ok, lookupErr := s.activeCollectorLease(ctx, req.Login)
		if lookupErr != nil {
			return CollectorLease{}, false, lookupErr
		}
		if ok {
			return current, false, nil
		}
		return CollectorLease{}, false, nil
	}
	if err != nil {
		return CollectorLease{}, false, err
	}
	return lease, lease.CollectorInstanceID == req.InstanceID, nil
}

func (s *Store) activeCollectorLease(ctx context.Context, login string) (CollectorLease, bool, error) {
	var lease CollectorLease
	err := s.db.QueryRow(ctx, `
		SELECT login, stream_id, collector_instance_id, priority, shard_index, shard_count,
			state, claimed_at, heartbeat_at, expires_at, updated_at
		FROM pulse_collector_leases
		WHERE login = $1 AND expires_at > now()`, normalizeLogin(login),
	).Scan(
		&lease.Login, &lease.StreamID, &lease.CollectorInstanceID, &lease.Priority,
		&lease.ShardIndex, &lease.ShardCount, &lease.State, &lease.ClaimedAt,
		&lease.HeartbeatAt, &lease.ExpiresAt, &lease.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectorLease{}, false, nil
	}
	if err != nil {
		return CollectorLease{}, false, err
	}
	return lease, true, nil
}

// RenewCollectorLease extends the lease (heartbeat) only while this instance is
// still the owner. Returns false if the lease was lost (reclaimed/preempted).
//
// Like ClaimCollectorLease, the heartbeat/expiry stamps use the Postgres server
// clock so a skewed worker clock cannot shorten or extend the real TTL. The now
// parameter is retained for the in-memory test fake only.
func (s *Store) RenewCollectorLease(ctx context.Context, login, instanceID string, now time.Time, ttl time.Duration) (bool, error) {
	login = normalizeLogin(login)
	if login == "" || instanceID == "" {
		return false, errors.New("collector lease renew requires login and instance id")
	}
	if ttl <= 0 {
		ttl = 90 * time.Second
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE pulse_collector_leases
		SET heartbeat_at = now(), expires_at = now() + make_interval(secs => $3), updated_at = now()
		WHERE login = $1 AND collector_instance_id = $2`,
		login, instanceID, ttl.Seconds(),
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ReleaseCollectorLease relinquishes ownership during graceful shutdown. It only
// deletes rows still owned by this instance so a reclaimed lease is left intact.
func (s *Store) ReleaseCollectorLease(ctx context.Context, login, instanceID string) error {
	login = normalizeLogin(login)
	if login == "" || instanceID == "" {
		return errors.New("collector lease release requires login and instance id")
	}
	_, err := s.db.Exec(ctx, `
		DELETE FROM pulse_collector_leases
		WHERE login = $1 AND collector_instance_id = $2`, login, instanceID)
	return err
}

func scanCollectorLeaseRows(rows pgx.Rows) ([]CollectorLease, error) {
	defer rows.Close()
	var out []CollectorLease
	for rows.Next() {
		var lease CollectorLease
		if err := rows.Scan(
			&lease.Login, &lease.StreamID, &lease.CollectorInstanceID, &lease.Priority,
			&lease.ShardIndex, &lease.ShardCount, &lease.State, &lease.ClaimedAt,
			&lease.HeartbeatAt, &lease.ExpiresAt, &lease.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, lease)
	}
	return out, rows.Err()
}

// ListCollectorLeasesByInstance returns the active (unexpired) leases owned by an
// instance. Expiry is evaluated against the DB clock; the now parameter is kept
// for the in-memory test fake only.
func (s *Store) ListCollectorLeasesByInstance(ctx context.Context, instanceID string, now time.Time) ([]CollectorLease, error) {
	if instanceID == "" {
		return nil, errors.New("collector lease list requires instance id")
	}
	rows, err := s.db.Query(ctx, `
		SELECT login, stream_id, collector_instance_id, priority, shard_index, shard_count,
			state, claimed_at, heartbeat_at, expires_at, updated_at
		FROM pulse_collector_leases
		WHERE collector_instance_id = $1 AND expires_at > now()
		ORDER BY priority DESC, login ASC`, instanceID)
	if err != nil {
		return nil, err
	}
	return scanCollectorLeaseRows(rows)
}

// ListActiveCollectorLeases returns every unexpired lease across all instances.
// Used by readiness/hub to report lease-backed external collector ownership even
// when no in-process collector tracks the login. now is kept for the test fake.
func (s *Store) ListActiveCollectorLeases(ctx context.Context, now time.Time) ([]CollectorLease, error) {
	rows, err := s.db.Query(ctx, `
		SELECT login, stream_id, collector_instance_id, priority, shard_index, shard_count,
			state, claimed_at, heartbeat_at, expires_at, updated_at
		FROM pulse_collector_leases
		WHERE expires_at > now()
		ORDER BY priority DESC, login ASC`)
	if err != nil {
		return nil, err
	}
	return scanCollectorLeaseRows(rows)
}

// ActiveCollectorLeaseByLogin returns the active lease for a login, if any. now
// is kept for the test fake; the SQL uses the DB clock.
func (s *Store) ActiveCollectorLeaseByLogin(ctx context.Context, login string, now time.Time) (CollectorLease, bool, error) {
	return s.activeCollectorLease(ctx, login)
}

// CountActiveCollectorLeases returns the total number of unexpired leases across
// all instances. now is kept for the test fake; the SQL uses the DB clock.
func (s *Store) CountActiveCollectorLeases(ctx context.Context, now time.Time) (int, error) {
	var n int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM pulse_collector_leases WHERE expires_at > now()`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ExpireStaleCollectorLeases removes expired rows. Safe housekeeping — expired
// leases are already ignorable, this just keeps the table small. now is kept for
// the test fake; the SQL uses the DB clock.
func (s *Store) ExpireStaleCollectorLeases(ctx context.Context, now time.Time) (int, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM pulse_collector_leases WHERE expires_at <= now()`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
