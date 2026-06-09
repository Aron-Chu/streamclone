package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrNotFound = errors.New("not found")

type Session struct {
	ID              string   `json:"id"`
	UserID          string   `json:"user_id"`
	Login           string   `json:"login"`
	DisplayName     string   `json:"display_name"`
	ProfileImageURL string   `json:"profile_image_url"`
	AccessToken     string   `json:"access_token"`
	RefreshToken    string   `json:"refresh_token"`
	Scopes          []string `json:"scopes"`
	ExpiresAt       int64    `json:"expires_at"`
}

type DeviceAuth struct {
	DeviceCode          string `json:"device_code"`
	UserCode            string `json:"user_code"`
	VerificationURI     string `json:"verification_uri"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	ExpiresAt           int64  `json:"expires_at"`
}

type Store interface {
	SetDevClaim(ctx context.Context, claimID string, sessionID string, ttl time.Duration) error
	TakeDevClaim(ctx context.Context, claimID string) (string, bool, error)
	TakeLatestDevClaim(ctx context.Context) (string, bool, error)
	SaveDeviceAuth(ctx context.Context, requestID string, deviceAuth DeviceAuth, ttl time.Duration) error
	GetDeviceAuth(ctx context.Context, requestID string) (DeviceAuth, error)
	DeleteDeviceAuth(ctx context.Context, requestID string) error
	SaveSession(ctx context.Context, session Session, ttl time.Duration) error
	GetSession(ctx context.Context, id string) (Session, error)
	DeleteSession(ctx context.Context, id string) error
}

type RedisStore struct {
	rdb *redis.Client
}

func NewRedisStore(rdb *redis.Client) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func (s *RedisStore) SetDevClaim(ctx context.Context, claimID string, sessionID string, ttl time.Duration) error {
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, "auth:dev-claim:"+claimID, sessionID, ttl)
	pipe.Set(ctx, "auth:dev-claim:latest", claimID, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *RedisStore) TakeDevClaim(ctx context.Context, claimID string) (string, bool, error) {
	key := "auth:dev-claim:" + claimID
	pipe := s.rdb.TxPipeline()
	get := pipe.Get(ctx, key)
	pipe.Del(ctx, key)
	latest := pipe.Get(ctx, "auth:dev-claim:latest")
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return "", false, err
	}
	if err := get.Err(); errors.Is(err, redis.Nil) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	if latest.Err() == nil && latest.Val() == claimID {
		_ = s.rdb.Del(ctx, "auth:dev-claim:latest").Err()
	}
	return get.Val(), true, nil
}

func (s *RedisStore) TakeLatestDevClaim(ctx context.Context) (string, bool, error) {
	claimID, err := s.rdb.GetDel(ctx, "auth:dev-claim:latest").Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return s.TakeDevClaim(ctx, claimID)
}

func (s *RedisStore) SaveDeviceAuth(ctx context.Context, requestID string, deviceAuth DeviceAuth, ttl time.Duration) error {
	data, err := json.Marshal(deviceAuth)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, "auth:device-auth:"+requestID, data, ttl).Err()
}

func (s *RedisStore) GetDeviceAuth(ctx context.Context, requestID string) (DeviceAuth, error) {
	data, err := s.rdb.Get(ctx, "auth:device-auth:"+requestID).Bytes()
	if errors.Is(err, redis.Nil) {
		return DeviceAuth{}, ErrNotFound
	}
	if err != nil {
		return DeviceAuth{}, err
	}
	var deviceAuth DeviceAuth
	if err := json.Unmarshal(data, &deviceAuth); err != nil {
		return DeviceAuth{}, err
	}
	return deviceAuth, nil
}

func (s *RedisStore) DeleteDeviceAuth(ctx context.Context, requestID string) error {
	return s.rdb.Del(ctx, "auth:device-auth:"+requestID).Err()
}

func (s *RedisStore) SaveSession(ctx context.Context, session Session, ttl time.Duration) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, "auth:session:"+session.ID, data, ttl).Err()
}

func (s *RedisStore) GetSession(ctx context.Context, id string) (Session, error) {
	data, err := s.rdb.Get(ctx, "auth:session:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *RedisStore) DeleteSession(ctx context.Context, id string) error {
	return s.rdb.Del(ctx, "auth:session:"+id).Err()
}
