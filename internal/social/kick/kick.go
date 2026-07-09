package kick

import (
	"context"
	"fmt"

	"streamclone/internal/social"
	"streamclone/internal/social/reliability"
)

func init() {
	social.Register("kick", func() (social.SocialSource, error) {
		return &Source{}, nil
	})
}

// Source is a Phase 3 Kick official API adapter skeleton.
type Source struct{}

func (s *Source) Name() string                      { return "kick" }
func (s *Source) Risk() reliability.Risk            { return reliability.RiskOfficial }
func (s *Source) Capabilities() social.Caps         { return social.Caps{} }
func (s *Source) Healthy(ctx context.Context) error { return fmt.Errorf("kick: phase 3 not enabled") }
func (s *Source) Search(ctx context.Context, q social.Query) (social.Page, error) {
	return social.Page{}, nil
}
