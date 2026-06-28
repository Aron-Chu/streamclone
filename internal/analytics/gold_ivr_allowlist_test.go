package analytics

import "testing"

func TestParseGoldIVRAllowlistLoginAndChannelID(t *testing.T) {
	allow := ParseGoldIVRAllowlist("ludwig,40934651, Jynxzi")
	if _, ok := allow.Logins["ludwig"]; !ok {
		t.Fatal("expected ludwig login")
	}
	if _, ok := allow.Logins["jynxzi"]; !ok {
		t.Fatal("expected normalized jynxzi login")
	}
	if _, ok := allow.ChannelIDs["40934651"]; !ok {
		t.Fatal("expected channel id")
	}
}

func TestGoldIVRAllowlistEmptyDeniesAll(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: true,
		Allowlist:   ParseGoldIVRAllowlist(""),
	}, nil, nil, nil)
	ok, reason := g.allowed("ludwig", "40934651")
	if ok || reason != "allowlist_empty" {
		t.Fatalf("empty allowlist should deny: ok=%v reason=%q", ok, reason)
	}
}

func TestGoldIVRAllowlistLoginHit(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: true,
		Allowlist:   ParseGoldIVRAllowlist("ludwig"),
	}, nil, nil, nil)
	ok, reason := g.allowed("Ludwig", "")
	if !ok || reason != "allowlist_login:ludwig" {
		t.Fatalf("login hit: ok=%v reason=%q", ok, reason)
	}
	ok, reason = g.allowed("jynxzi", "")
	if ok {
		t.Fatalf("miss expected: ok=%v reason=%q", ok, reason)
	}
}

func TestGoldIVRAllowlistChannelIDHit(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: true,
		Allowlist:   ParseGoldIVRAllowlist("40934651"),
	}, nil, nil, nil)
	ok, reason := g.allowed("", "40934651")
	if !ok || reason != "allowlist_channel_id:40934651" {
		t.Fatalf("channel id hit: ok=%v reason=%q", ok, reason)
	}
}

func TestGoldIVRAllowlistDisabledWithoutLite(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: false,
		ShadowMode:  false,
		Allowlist:   ParseGoldIVRAllowlist("ludwig"),
	}, nil, nil, nil)
	ok, reason := g.allowed("ludwig", "")
	if ok || reason != "ivr_lite_disabled" {
		t.Fatalf("lite off without shadow: ok=%v reason=%q", ok, reason)
	}
}
