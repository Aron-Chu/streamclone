package analytics

import (
	"testing"

	"streamclone/internal/emoteimage"
)

func TestRewriteHostedExtensionEmoteURLs(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	providerID := "62a3bf572b964d6cc2766004"
	base := "https://api.streampulse.stream/emotes"
	in := []ExtensionEmote{{
		ID:       localID,
		Provider: "seventv",
		ImageURL: emoteimage.URL("seventv", localID, "1x"),
	}}
	out := rewriteHostedExtensionEmoteURLs(in, map[string]string{localID: providerID}, base)
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	want := "https://cdn.7tv.app/emote/" + providerID + "/4x.webp"
	if out[0].ImageURL != want {
		t.Fatalf("7tv lookup: got %q want %q", out[0].ImageURL, want)
	}
	ffz := []ExtensionEmote{{
		ID:       localID,
		Provider: "ffz",
		ImageURL: emoteimage.URL("ffz", localID, "1x"),
	}}
	outFFZ := rewriteHostedExtensionEmoteURLs(ffz, nil, base)
	if outFFZ[0].ImageURL != base+"/emotes/"+localID+"/1x.webp" {
		t.Fatalf("ffz hosted: got %q", outFFZ[0].ImageURL)
	}
}

func TestHostedEmoteCDNBaseRequiresHosted(t *testing.T) {
	h := &Handler{cdnPublicBase: "https://api.streampulse.stream/emotes"}
	if h.hostedEmoteCDNBase() != "" {
		t.Fatal("expected empty when not hosted")
	}
	h.pulseHosted.Hosted = true
	if h.hostedEmoteCDNBase() != "https://api.streampulse.stream/emotes" {
		t.Fatalf("got %q", h.hostedEmoteCDNBase())
	}
}
