package config

import (
	"os"
	"testing"
	"time"
)

func TestTop500MetadataConfigDefaultsDisabled(t *testing.T) {
	clearTop500Env(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Top500MetadataEnabled {
		t.Fatal("Top500MetadataEnabled default = true, want false")
	}
	if !cfg.Top500MetadataDryRun {
		t.Fatal("Top500MetadataDryRun default = false, want true")
	}
	if cfg.Top500MetadataWriteEnabled {
		t.Fatal("Top500MetadataWriteEnabled default = true, want false")
	}
	if cfg.Top500MetadataTopN != 100 {
		t.Fatalf("Top500MetadataTopN default = %d, want 100", cfg.Top500MetadataTopN)
	}
	if cfg.Top500MetadataLiveInterval != time.Minute {
		t.Fatalf("Top500MetadataLiveInterval = %s, want 1m", cfg.Top500MetadataLiveInterval)
	}
	if cfg.Top500MetadataOfflineInterval != 10*time.Minute {
		t.Fatalf("Top500MetadataOfflineInterval = %s, want 10m", cfg.Top500MetadataOfflineInterval)
	}
	if cfg.Top500MetadataBatchSize != 100 {
		t.Fatalf("Top500MetadataBatchSize default = %d, want 100", cfg.Top500MetadataBatchSize)
	}
	if cfg.Top500MetadataFixtureProvider {
		t.Fatal("Top500MetadataFixtureProvider default = true, want false")
	}
}

func TestTop500MetadataConfigOverridesAndCaps(t *testing.T) {
	clearTop500Env(t)
	t.Setenv("TOP500_METADATA_ENABLED", "true")
	t.Setenv("TOP500_METADATA_DRY_RUN", "false")
	t.Setenv("TOP500_METADATA_WRITE_ENABLED", "true")
	t.Setenv("TOP500_METADATA_TOP_N", "500")
	t.Setenv("TOP500_METADATA_BATCH_SIZE", "250")
	t.Setenv("TOP500_METADATA_LIVE_INTERVAL", "45s")
	t.Setenv("TOP500_METADATA_OFFLINE_INTERVAL", "5m")
	t.Setenv("TOP500_METADATA_DB_P95_HOLD_MS", "60")
	t.Setenv("TOP500_METADATA_ROLLBACK_DB_P95_MS", "250")
	t.Setenv("TOP500_METADATA_ROLLBACK_DISK_FREE_PERCENT", "20")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Top500MetadataEnabled || cfg.Top500MetadataDryRun || !cfg.Top500MetadataWriteEnabled {
		t.Fatalf("unexpected top500 booleans: enabled=%v dryRun=%v write=%v", cfg.Top500MetadataEnabled, cfg.Top500MetadataDryRun, cfg.Top500MetadataWriteEnabled)
	}
	if cfg.Top500MetadataTopN != 100 {
		t.Fatalf("Top500MetadataTopN = %d, want cap 100", cfg.Top500MetadataTopN)
	}
	if cfg.Top500MetadataBatchSize != 100 {
		t.Fatalf("Top500MetadataBatchSize = %d, want cap 100", cfg.Top500MetadataBatchSize)
	}
	if cfg.Top500MetadataLiveInterval != 45*time.Second {
		t.Fatalf("Top500MetadataLiveInterval = %s, want 45s", cfg.Top500MetadataLiveInterval)
	}
	if cfg.Top500MetadataOfflineInterval != 5*time.Minute {
		t.Fatalf("Top500MetadataOfflineInterval = %s, want 5m", cfg.Top500MetadataOfflineInterval)
	}
	if cfg.Top500MetadataDBP95HoldMS != 60 || cfg.Top500MetadataRollbackDBP95MS != 250 || cfg.Top500MetadataRollbackDiskFreePercent != 20 {
		t.Fatalf("unexpected thresholds: hold=%d rollbackDB=%d disk=%d", cfg.Top500MetadataDBP95HoldMS, cfg.Top500MetadataRollbackDBP95MS, cfg.Top500MetadataRollbackDiskFreePercent)
	}
}

func clearTop500Env(t *testing.T) {
	t.Helper()
	keys := []string{
		"TOP500_METADATA_ENABLED",
		"TOP500_METADATA_DRY_RUN",
		"TOP500_METADATA_TOP_N",
		"TOP500_METADATA_WRITE_ENABLED",
		"TOP500_METADATA_LIVE_INTERVAL",
		"TOP500_METADATA_OFFLINE_INTERVAL",
		"TOP500_METADATA_BATCH_SIZE",
		"TOP500_METADATA_FIXTURE_PROVIDER",
		"TOP500_METADATA_DB_P95_HOLD_MS",
		"TOP500_METADATA_ROLLBACK_DB_P95_MS",
		"TOP500_METADATA_ROLLBACK_DISK_FREE_PERCENT",
	}
	previous := make(map[string]string, len(keys))
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if ok {
			previous[key] = value
			present[key] = true
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, previous[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}

func TestTop500SilverGateConfigDefaultsDisabled(t *testing.T) {
	clearTop500SilverGateEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Top500SilverGateEnabled {
		t.Fatal("Top500SilverGateEnabled default = true, want false")
	}
	if !cfg.Top500SilverGateDryRun {
		t.Fatal("Top500SilverGateDryRun default = false, want true")
	}
	if cfg.Top500SilverGateWriteEnabled {
		t.Fatal("Top500SilverGateWriteEnabled default = true, want false")
	}
	if cfg.Top500SilverGateMaxCandidates != 5 {
		t.Fatalf("Top500SilverGateMaxCandidates = %d, want 5", cfg.Top500SilverGateMaxCandidates)
	}
	if cfg.Top500SilverGateMaxEnqueuePerRun != 1 {
		t.Fatalf("Top500SilverGateMaxEnqueuePerRun = %d, want 1", cfg.Top500SilverGateMaxEnqueuePerRun)
	}
	if cfg.Top500SilverGateInterval != 10*time.Minute {
		t.Fatalf("Top500SilverGateInterval = %s, want 10m", cfg.Top500SilverGateInterval)
	}
}

func TestTop500SilverGateConfigValidationCapsUnsafeValues(t *testing.T) {
	clearTop500SilverGateEnv(t)
	t.Setenv("TOP500_SILVER_GATE_MAX_CANDIDATES", "500")
	t.Setenv("TOP500_SILVER_GATE_MAX_ENQUEUE_PER_RUN", "99")
	t.Setenv("TOP500_SILVER_GATE_INTERVAL", "0")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Top500SilverGateMaxCandidates != 5 {
		t.Fatalf("MaxCandidates = %d, want cap 5", cfg.Top500SilverGateMaxCandidates)
	}
	if cfg.Top500SilverGateMaxEnqueuePerRun != 1 {
		t.Fatalf("MaxEnqueuePerRun = %d, want cap 1", cfg.Top500SilverGateMaxEnqueuePerRun)
	}
	if cfg.Top500SilverGateInterval != 10*time.Minute {
		t.Fatalf("Interval = %s, want default 10m", cfg.Top500SilverGateInterval)
	}
}

func TestTop500SilverGateConfigAllowsMaxBoundaries(t *testing.T) {
	clearTop500SilverGateEnv(t)
	t.Setenv("TOP500_SILVER_GATE_MAX_CANDIDATES", "100")
	t.Setenv("TOP500_SILVER_GATE_MAX_ENQUEUE_PER_RUN", "10")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Top500SilverGateMaxCandidates != 100 {
		t.Fatalf("MaxCandidates = %d, want 100", cfg.Top500SilverGateMaxCandidates)
	}
	if cfg.Top500SilverGateMaxEnqueuePerRun != 10 {
		t.Fatalf("MaxEnqueuePerRun = %d, want 10", cfg.Top500SilverGateMaxEnqueuePerRun)
	}
}

func TestTop500SilverGateConfigRejectsAboveMaxBoundaries(t *testing.T) {
	clearTop500SilverGateEnv(t)
	t.Setenv("TOP500_SILVER_GATE_MAX_CANDIDATES", "101")
	t.Setenv("TOP500_SILVER_GATE_MAX_ENQUEUE_PER_RUN", "11")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Top500SilverGateMaxCandidates != 5 {
		t.Fatalf("MaxCandidates = %d, want reset to 5", cfg.Top500SilverGateMaxCandidates)
	}
	if cfg.Top500SilverGateMaxEnqueuePerRun != 1 {
		t.Fatalf("MaxEnqueuePerRun = %d, want reset to 1", cfg.Top500SilverGateMaxEnqueuePerRun)
	}
}

func clearTop500SilverGateEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"TOP500_SILVER_GATE_ENABLED",
		"TOP500_SILVER_GATE_DRY_RUN",
		"TOP500_SILVER_GATE_WRITE_ENABLED",
		"TOP500_SILVER_GATE_MAX_CANDIDATES",
		"TOP500_SILVER_GATE_MAX_ENQUEUE_PER_RUN",
		"TOP500_SILVER_GATE_INTERVAL",
	}
	previous := make(map[string]string, len(keys))
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if ok {
			previous[key] = value
			present[key] = true
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("unset %s: %v", key, err)
			}
		}
	}
	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, previous[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}
