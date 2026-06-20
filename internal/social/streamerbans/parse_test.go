package streamerbans

import "testing"

func TestParseBanPartnerWithHandle(t *testing.T) {
	login, display, ok := ParseBan(`❌ Twitch Partner "HasanAbi" (@HasanAbi) has been banned! ❌`)
	if !ok || login != "hasanabi" || display != "HasanAbi" {
		t.Fatalf("ParseBan() = %q %q %v", login, display, ok)
	}
}

func TestParseBanPartnerPlain(t *testing.T) {
	login, display, ok := ParseBan(`Twitch Partner "frogan" has been banned!`)
	if !ok || login != "frogan" || display != "frogan" {
		t.Fatalf("ParseBan() = %q %q %v", login, display, ok)
	}
}

func TestParseBanNoMatch(t *testing.T) {
	if _, _, ok := ParseBan("random clip title"); ok {
		t.Fatal("expected no match")
	}
}
