package seeder

import (
	"context"

	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/store"
	emotesync "streamclone/internal/emote/sync"
)

type SevenTVSyncer struct {
	st   *store.Store
	seed *Seeder
	d    *dict.Dict
}

func NewSevenTVSyncer(st *store.Store, seed *Seeder, d *dict.Dict) *SevenTVSyncer {
	return &SevenTVSyncer{st: st, seed: seed, d: d}
}

func (s *SevenTVSyncer) ApplyChannelSet(ctx context.Context, login, twitchID, setID string) error {
	u, err := s.seed.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return err
	}
	if u.EmoteSet == nil {
		return nil
	}
	if err := s.seed.syncSevenTVSet(ctx, setID, twitchID, u); err != nil {
		return err
	}
	return s.seed.rebuildChannelDictionary(ctx, login)
}

// sevenTVRenameOp captures a local emote whose 7TV alias/name changed remotely.
type sevenTVRenameOp struct {
	EmoteID string
	NewName string
}

// sevenTVSetDiff is the pure result of comparing a remote 7TV emote set against
// the local emote_set_items rows for the same provider set.
type sevenTVSetDiff struct {
	RemoveEmoteIDs []string          // local emote IDs no longer present remotely
	Renames        []sevenTVRenameOp // local emotes whose remote name changed
	AddProviderIDs []string          // remote provider emote IDs missing locally
}

// diffSevenTVSet computes the add/remove/rename operations needed to make the
// local set match the remote 7TV set. It is pure (no I/O) so the prune/rename
// logic can be unit tested without a database.
func diffSevenTVSet(remote []sevenTVEmote, local []store.SetProviderEmote) sevenTVSetDiff {
	remoteByID := make(map[string]sevenTVEmote, len(remote))
	for _, em := range remote {
		if em.ID != "" {
			remoteByID[em.ID] = em
		}
	}
	localByID := make(map[string]store.SetProviderEmote, len(local))
	for _, row := range local {
		if row.ProviderEmoteID != "" {
			localByID[row.ProviderEmoteID] = row
		}
	}

	var diff sevenTVSetDiff
	for id, row := range localByID {
		remoteEm, ok := remoteByID[id]
		if !ok {
			diff.RemoveEmoteIDs = append(diff.RemoveEmoteIDs, row.EmoteID)
			continue
		}
		if remoteEm.Name != "" && remoteEm.Name != row.Name {
			diff.Renames = append(diff.Renames, sevenTVRenameOp{EmoteID: row.EmoteID, NewName: remoteEm.Name})
		}
	}
	for _, em := range remote {
		if em.ID == "" {
			continue
		}
		if _, ok := localByID[em.ID]; ok {
			continue
		}
		diff.AddProviderIDs = append(diff.AddProviderIDs, em.ID)
	}
	return diff
}

func (s *Seeder) syncSevenTVSet(ctx context.Context, setID, twitchID string, u *sevenTVUser) error {
	if u.EmoteSet == nil {
		return nil
	}
	localRows, err := s.st.ListSetProviderEmotes(ctx, setID, string(ProviderSevenTV))
	if err != nil {
		return err
	}
	diff := diffSevenTVSet(u.EmoteSet.Emotes, localRows)

	for _, emoteID := range diff.RemoveEmoteIDs {
		if err := s.st.RemoveEmoteFromSet(ctx, setID, emoteID); err != nil {
			return err
		}
	}
	for _, rn := range diff.Renames {
		if err := s.st.UpdateEmoteName(ctx, rn.EmoteID, rn.NewName); err != nil {
			return err
		}
	}

	if len(diff.AddProviderIDs) == 0 {
		return nil
	}
	addIDs := make(map[string]struct{}, len(diff.AddProviderIDs))
	for _, id := range diff.AddProviderIDs {
		addIDs[id] = struct{}{}
	}
	imports := make([]remoteEmote, 0, len(diff.AddProviderIDs))
	for _, em := range u.EmoteSet.Emotes {
		if _, ok := addIDs[em.ID]; !ok {
			continue
		}
		imports = append(imports, remoteEmote{
			Provider:        ProviderSevenTV,
			ProviderEmoteID: em.ID,
			ProviderSetID:   u.EmoteSet.ID,
			Name:            em.Name,
			OwnerID:         twitchID,
			SourceURL:       s.cdnURL + "/emote/" + em.ID + "/4x.webp",
			MimeType:        "image/webp",
			Animated:        em.Data.Animated,
			ZeroWidth:       sevenTVZeroWidth(em),
		})
	}
	sortRemoteEmotes(imports)
	_, err = s.importRemoteEmotesToSet(ctx, setID, imports)
	return err
}

func sevenTVEmoteHash(u *sevenTVUser) string {
	if u == nil || u.EmoteSet == nil {
		return ""
	}
	ids := make([]string, 0, len(u.EmoteSet.Emotes))
	for _, em := range u.EmoteSet.Emotes {
		if em.ID != "" {
			ids = append(ids, em.ID)
		}
	}
	return emotesync.HashSevenTVEmoteIDs(ids)
}

func (s *Seeder) rebuildAfterSevenTVSync(ctx context.Context, login, twitchID, setID string) error {
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return err
	}
	if err := s.syncSevenTVSet(ctx, setID, twitchID, u); err != nil {
		return err
	}
	if _, err := s.SyncSevenTVEmoteFlags(ctx, twitchID); err != nil && s.log != nil {
		s.log.Warn("sync 7tv zero-width flags", "twitch_id", twitchID, "err", err)
	}
	return s.rebuildChannelDictionary(ctx, login)
}
