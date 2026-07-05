package analytics

import "testing"

func TestCorpusRuntimeHubCacheFingerprintStable(t *testing.T) {
	cfg := CorpusRuntimeConfig{
		TargetTopN:           1000,
		LiveAdmissionEnabled: true,
		LiveAdmissionTopN:    1000,
		MaxActiveIRCChannels: 300,
	}
	got := corpusRuntimeHubCacheFingerprint(cfg, 300)
	want := "n1000:a1:t1000:irc300:col300"
	if got != want {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
}

func TestPublicHubRuntimeFingerprintMatchesLegacyPipeline(t *testing.T) {
	expected := corpusRuntimeHubCacheFingerprint(CorpusRuntimeConfig{
		TargetTopN:           500,
		LiveAdmissionEnabled: true,
		LiveAdmissionTopN:    500,
		MaxActiveIRCChannels: 300,
	}, 300)
	pipeline := HubCorpusPipeline{
		TopN:                 500,
		LiveAdmissionEnabled: true,
		LiveAdmissionTopN:    500,
		MaxActiveIRCChannels: 300,
		CollectorMax:         300,
	}
	if !publicHubRuntimeFingerprintMatches(pipeline, expected) {
		t.Fatal("legacy pipeline without fingerprint should match rebuilt fingerprint")
	}
}

func TestPublicHub30mAnd7dShareRuntimeFingerprint(t *testing.T) {
	cfg := CorpusRuntimeConfig{
		TargetTopN:           1000,
		LiveAdmissionEnabled: true,
		LiveAdmissionTopN:    1000,
		MaxActiveIRCChannels: 1200,
	}
	fp30 := publicHubCacheKey(publicHubOptions{ActivityWindowMinutes: 30}, corpusRuntimeHubCacheFingerprint(cfg, 1200))
	fp7d := publicHubCacheKey(publicHubOptions{ActivityWindowMinutes: 7 * 24 * 60}, corpusRuntimeHubCacheFingerprint(cfg, 1200))
	if fp30 == fp7d {
		t.Fatal("activity window should differentiate cache keys")
	}
	fp30cfg := stringsAfter(fp30, ":cfg:")
	fp7dcfg := stringsAfter(fp7d, ":cfg:")
	if fp30cfg != fp7dcfg {
		t.Fatalf("runtime fingerprint mismatch: 30m=%q 7d=%q", fp30cfg, fp7dcfg)
	}
}

func stringsAfter(s, sep string) string {
	i := len(s) - len(sep)
	for j := 0; j <= i; j++ {
		if len(s) >= j+len(sep) && s[j:j+len(sep)] == sep {
			return s[j+len(sep):]
		}
	}
	return ""
}
