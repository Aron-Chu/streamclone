package enrich

import (
	"testing"

	"streamclone/internal/chat/batch"
)

func TestNotifyObservedInvokesCallbackForThirdParty(t *testing.T) {
	e := New(nil, 0, nil)
	var seen []batch.Fragment
	e.SetEmoteObserver(func(_ string, frag batch.Fragment) {
		seen = append(seen, frag)
	})
	e.notifyObserved("xqc", []batch.Fragment{
		{T: "text", C: "hello"},
		{T: "emote", C: "Pog", ID: "75f49395-d5fc-41da-998c-880c6d8fddcb", Provider: "seventv"},
		{T: "emote", C: "Kappa", ID: "22639", Provider: "twitch"},
	})
	if len(seen) != 1 {
		t.Fatalf("expected 1 observed emote, got %d", len(seen))
	}
	if seen[0].Provider != "seventv" {
		t.Fatalf("provider = %q", seen[0].Provider)
	}
}

func TestNotifyObservedNoCallback(t *testing.T) {
	e := New(nil, 0, nil)
	e.notifyObserved("xqc", []batch.Fragment{{T: "emote", ID: "1", Provider: "ffz"}})
}
