package xrecent

import (
	"context"
	"fmt"

	"streamclone/internal/social"
	"streamclone/internal/storygraph/reliability"
)

func init() {
	social.Register("xrecent", func() (social.SocialSource, error) {
		return &Source{}, nil
	})
}

// Source is a Phase 2 X recent-search adapter (budgeted).
type Source struct{}

func (s *Source) Name() string { return "xrecent" }
func (s *Source) Risk() reliability.Risk { return reliability.RiskPublicAPI }
func (s *Source) Capabilities() social.Caps {
	return social.Caps{RefreshMetrics: true} // no RealtimeFirehose
}
func (s *Source) Healthy(ctx context.Context) error { return fmt.Errorf("x: phase 2 not enabled") }
func (s *Source) Search(ctx context.Context, q social.Query) (social.Page, error) {
	return social.Page{}, nil
}
