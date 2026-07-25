package integration_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/objstore"
	"streamclone/internal/emote/store"
	"streamclone/internal/emote/worker"
)

func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run integration tests")
	}
}

func pgURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://app:test@localhost:15432/emotes?sslmode=disable"
}

func redisAddr() string {
	if a := os.Getenv("TEST_REDIS_ADDR"); a != "" {
		return a
	}
	return "localhost:16379"
}

func minioEndpoint() string {
	if e := os.Getenv("TEST_MINIO_ENDPOINT"); e != "" {
		return e
	}
	return "localhost:19000"
}

func setupPG(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(ctx, pgURL())
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pg ping: %v", err)
	}
	return pool
}

func setupRedis(t *testing.T, ctx context.Context) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr()})
	t.Cleanup(func() { rdb.Close() })
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}
	return rdb
}

func setupMinio(t *testing.T, ctx context.Context) *objstore.Client {
	t.Helper()
	endpoint := minioEndpoint()
	client, err := objstore.New(endpoint, "minioadmin", "minioadmin", "test-emotes", "", false)
	if err != nil {
		t.Fatalf("objstore.New: %v", err)
	}
	if err := client.EnsureBucket(ctx, true); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	return client
}

func skipIfNoVips(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("vips"); err != nil {
		t.Skip("libvips CLI (vips) not installed; skipping upload→active pipeline test")
	}
}

func applyMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	files := []string{
		"000002_emotes.up.sql",
		"000003_provider_emotes.up.sql",
		"000004_channel_emote_providers.up.sql",
	}
	dir := migrationsDir(t)
	for _, name := range files {
		path := filepath.Join(dir, name)
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
}

func cleanDB(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tables := []string{"processing_jobs", "emote_set_items", "channel_emote_providers", "channels", "emote_sets", "emotes"}
	for _, tbl := range tables {
		if _, err := pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", tbl)); err != nil {
			t.Logf("clean %s: %v", tbl, err)
		}
	}
}

func TestUploadToActive(t *testing.T) {
	skipIfShort(t)
	skipIfNoVips(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := setupPG(t, ctx)
	rdb := setupRedis(t, ctx)
	obj := setupMinio(t, ctx)
	applyMigrations(t, ctx, pool)
	cleanDB(t, ctx, pool)

	st := store.New(pool)
	d := dict.New(rdb, "/emotes")
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	w := worker.NewWithDictionaryDebounce(st, obj, d, log, 50*time.Millisecond)

	if err := st.UpsertChannel(ctx, "12345", "testchannel", "TestChannel"); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	setID, err := st.CreateEmoteSet(ctx, "test-set", "12345")
	if err != nil {
		t.Fatalf("CreateEmoteSet: %v", err)
	}
	if err := st.SetActiveEmoteSet(ctx, "12345", setID); err != nil {
		t.Fatalf("SetActiveEmoteSet: %v", err)
	}

	emoteID, err := st.UpsertEmote(ctx, store.Emote{
		Name:       "TestEmote",
		OwnerID:    "12345",
		MimeType:   "image/webp",
		SourceHash: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Status:     0,
	})
	if err != nil {
		t.Fatalf("UpsertEmote: %v", err)
	}

	if err := st.AddEmoteToSet(ctx, setID, emoteID, nil); err != nil {
		t.Fatalf("AddEmoteToSet: %v", err)
	}

	srcData := testWebPImage()
	if err := obj.PutSrc(ctx, emoteID, srcData, "image/webp"); err != nil {
		t.Fatalf("PutSrc: %v", err)
	}

	_, err = st.InsertJob(ctx, emoteID, emoteID+"/src")
	if err != nil {
		t.Fatalf("InsertJob: %v", err)
	}

	workerCtx, workerCancel := context.WithCancel(ctx)
	w.RunPool(workerCtx, 1)

	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			workerCancel()
			t.Fatal("timed out waiting for emote to become active")
		default:
		}
		emote, err := st.GetEmote(ctx, emoteID)
		if err != nil {
			t.Fatalf("GetEmote: %v", err)
		}
		if emote.Status == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	workerCancel()

	time.Sleep(200 * time.Millisecond)

	emotes, err := st.GetChannelEmotes(ctx, "testchannel")
	if err != nil {
		t.Fatalf("GetChannelEmotes: %v", err)
	}
	if len(emotes) == 0 {
		t.Fatal("expected channel emotes after activation, got 0")
	}
	found := false
	for _, e := range emotes {
		if e.EmoteID == emoteID && e.Name == "TestEmote" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected TestEmote in channel emotes, got: %+v", emotes)
	}

	mc, err := minio.New(minioEndpoint(), &minio.Options{
		Creds:  credentials.NewStaticV4("minioadmin", "minioadmin", ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("minio.New: %v", err)
	}
	for _, scale := range []string{"1x", "2x", "3x", "4x"} {
		key := fmt.Sprintf("%s/%s.webp", emoteID, scale)
		_, err := mc.StatObject(ctx, "test-emotes", key, minio.StatObjectOptions{})
		if err != nil {
			t.Errorf("missing rendered object %s: %v", key, err)
		}
	}
}

func TestSetChangeToDelta(t *testing.T) {
	skipIfShort(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := setupPG(t, ctx)
	rdb := setupRedis(t, ctx)
	applyMigrations(t, ctx, pool)
	cleanDB(t, ctx, pool)

	st := store.New(pool)
	d := dict.New(rdb, "/emotes")

	if err := rdb.HSet(ctx, "channel:emotes:legacyttl", "LegacyEmote", "{}").Err(); err != nil {
		t.Fatalf("seed legacy dictionary: %v", err)
	}
	if updated, err := d.BackfillLegacyTTLs(ctx, 100); err != nil {
		t.Fatalf("BackfillLegacyTTLs: %v", err)
	} else if updated < 1 {
		t.Fatalf("BackfillLegacyTTLs updated=%d, want at least legacyttl", updated)
	}
	if ttl, err := rdb.TTL(ctx, "channel:emotes:legacyttl").Result(); err != nil || ttl <= 0 {
		t.Fatalf("legacy dictionary ttl = %s, err=%v", ttl, err)
	}

	if err := st.UpsertChannel(ctx, "99999", "deltachannel", "DeltaChannel"); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	setID, err := st.CreateEmoteSet(ctx, "delta-set", "99999")
	if err != nil {
		t.Fatalf("CreateEmoteSet: %v", err)
	}
	if err := st.SetActiveEmoteSet(ctx, "99999", setID); err != nil {
		t.Fatalf("SetActiveEmoteSet: %v", err)
	}

	emoteID, err := st.UpsertEmote(ctx, store.Emote{
		Name:       "DeltaEmote",
		OwnerID:    "99999",
		MimeType:   "image/webp",
		SourceHash: "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd",
		Status:     1,
	})
	if err != nil {
		t.Fatalf("UpsertEmote: %v", err)
	}

	sub := rdb.Subscribe(ctx, "emotes:delta:deltachannel")
	defer sub.Close()
	ch := sub.Channel()

	time.Sleep(100 * time.Millisecond)

	if err := d.AddEmote(ctx, "deltachannel", "DeltaEmote", emoteID, false); err != nil {
		t.Fatalf("AddEmote: %v", err)
	}

	select {
	case msg := <-ch:
		if msg == nil {
			t.Fatal("nil message from delta subscription")
		}
		if msg.Payload == "" {
			t.Fatal("empty delta payload")
		}
		t.Logf("received delta: %s", msg.Payload)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for emote delta")
	}

	val, err := rdb.HGet(ctx, "channel:emotes:deltachannel", "DeltaEmote").Result()
	if err != nil {
		t.Fatalf("HGet: %v", err)
	}
	if val == "" {
		t.Fatal("expected non-empty hash entry for DeltaEmote")
	}
	if ttl, err := rdb.TTL(ctx, "channel:emotes:deltachannel").Result(); err != nil || ttl <= 0 {
		t.Fatalf("channel dictionary ttl after add = %s, err=%v", ttl, err)
	}

	if err := d.RemoveEmote(ctx, "deltachannel", "DeltaEmote"); err != nil {
		t.Fatalf("RemoveEmote: %v", err)
	}

	select {
	case msg := <-ch:
		if msg == nil {
			t.Fatal("nil remove message")
		}
		t.Logf("received remove delta: %s", msg.Payload)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for remove delta")
	}

	exists, err := rdb.HExists(ctx, "channel:emotes:deltachannel", "DeltaEmote").Result()
	if err != nil {
		t.Fatalf("HExists: %v", err)
	}
	if exists {
		t.Error("expected DeltaEmote to be removed from hash")
	}
}

func testWebPImage() []byte {
	return []byte{
		0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00,
		0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x4C,
		0x17, 0x00, 0x00, 0x00, 0x2F, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
}
