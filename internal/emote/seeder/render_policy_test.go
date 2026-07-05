package seeder

import (
	"testing"

	"streamclone/internal/emote/render"
)

func TestSeederShouldEagerRenderUsesQueueConfig(t *testing.T) {
	s := &Seeder{render: render.NewQueue(nil, nil, render.Config{
		TwitchEager:     false,
		ThirdpartyEager: false,
	}, nil)}
	if s.shouldEagerRender(ProviderTwitch) {
		t.Fatal("twitch eager render should be disabled by default")
	}
	if s.shouldEagerRender(ProviderSevenTV) {
		t.Fatal("third-party eager render should be disabled by default")
	}

	s.render = render.NewQueue(nil, nil, render.Config{TwitchEager: true, ThirdpartyEager: true}, nil)
	if !s.shouldEagerRender(ProviderTwitch) || !s.shouldEagerRender(ProviderFFZ) {
		t.Fatal("expected eager render when config flags enabled")
	}
}

func TestSeederShouldEagerRenderLegacyWithoutQueue(t *testing.T) {
	s := &Seeder{}
	if !s.shouldEagerRender(ProviderTwitch) {
		t.Fatal("nil render queue preserves legacy eager behavior")
	}
}
