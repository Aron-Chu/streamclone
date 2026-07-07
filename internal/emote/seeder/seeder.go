package seeder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/flags"
	"streamclone/internal/emote/objstore"
	"streamclone/internal/emote/render"
	"streamclone/internal/emote/store"
	"streamclone/internal/metadata/helix"
)

type Provider string

const (
	ProviderSevenTV       Provider = "seventv"
	ProviderTwitch        Provider = "twitch"
	ProviderFFZ           Provider = "ffz"
	ProviderBTTV          Provider = "bttv"
	sourceDownloadTimeout          = 6 * time.Second
)

// errProviderNotFound marks a provider 404 for a channel that never registered
// with that provider (BTTV user / FFZ room). It means "no channel emotes",
// not a seed failure — globals should still be imported.
var errProviderNotFound = errors.New("provider has no record for channel")

type ProviderResult struct {
	Provider      string
	State         string
	Count         int
	ExpectedCount int
	Error         string
	DurationMS    int64
}

type ProviderSnapshot struct {
	SetID     string
	Count     int
	EmoteHash string
}

type ProviderSnapshotItem struct {
	ProviderEmoteID string
	ProviderSetID   string
	Alias           string
	CanonicalName   string
	SourceURL       string
	Flags           int
	Animated        bool
	ZeroWidth       bool
}

type ProviderSnapshotDetail struct {
	Provider string
	SetID    string
	Items    []ProviderSnapshotItem
}

type ImportResult struct {
	Imported int
	Expected int
	Failed   int
}

type Seeder struct {
	st                *store.Store
	obj               *objstore.Client
	d                 *dict.Dict
	render            *render.Queue
	log               *slog.Logger
	apiURL            string
	cdnURL            string
	ffzURL            string
	bttvURL           string
	twitch            *helix.Client
	hc                *http.Client
	importConcurrency int
}

func New(st *store.Store, obj *objstore.Client, d *dict.Dict, log *slog.Logger, apiURL, cdnURL, ffzURL, bttvURL string, twitch *helix.Client) *Seeder {
	return NewWithImportConcurrency(st, obj, d, log, apiURL, cdnURL, ffzURL, bttvURL, twitch, 8)
}

func NewWithImportConcurrency(st *store.Store, obj *objstore.Client, d *dict.Dict, log *slog.Logger, apiURL, cdnURL, ffzURL, bttvURL string, twitch *helix.Client, importConcurrency int) *Seeder {
	return NewWithRenderQueue(st, obj, d, nil, log, apiURL, cdnURL, ffzURL, bttvURL, twitch, importConcurrency)
}

func NewWithRenderQueue(st *store.Store, obj *objstore.Client, d *dict.Dict, rq *render.Queue, log *slog.Logger, apiURL, cdnURL, ffzURL, bttvURL string, twitch *helix.Client, importConcurrency int) *Seeder {
	return &Seeder{
		st:                st,
		obj:               obj,
		d:                 d,
		render:            rq,
		log:               log,
		apiURL:            strings.TrimRight(apiURL, "/"),
		cdnURL:            strings.TrimRight(cdnURL, "/"),
		ffzURL:            strings.TrimRight(ffzURL, "/"),
		bttvURL:           strings.TrimRight(bttvURL, "/"),
		twitch:            twitch,
		hc:                &http.Client{Timeout: 20 * time.Second},
		importConcurrency: normalizeImportConcurrency(importConcurrency),
	}
}

type sevenTVUser struct {
	EmoteSet *sevenTVSet `json:"emote_set"`
	User     struct {
		ID          string `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	} `json:"user"`
}

type sevenTVSet struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Emotes []sevenTVEmote `json:"emotes"`
}

type sevenTVEmote struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Flags int    `json:"flags"`
	Data  struct {
		Animated bool `json:"animated"`
		Flags    int  `json:"flags"`
	} `json:"data"`
}

type ffzResponse struct {
	Room struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"room"`
	Sets map[string]ffzSet `json:"sets"`
}

type ffzGlobalResponse struct {
	DefaultSets []int64           `json:"default_sets"`
	Sets        map[string]ffzSet `json:"sets"`
}

type ffzSet struct {
	ID        int64      `json:"id"`
	Emoticons []ffzEmote `json:"emoticons"`
}

type ffzEmote struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	URLs     map[string]string `json:"urls"`
	Animated json.RawMessage   `json:"animated"`
	Modifier bool              `json:"modifier"`
}

type bttvUserResponse struct {
	ChannelEmotes []bttvEmote `json:"channelEmotes"`
	SharedEmotes  []bttvEmote `json:"sharedEmotes"`
}

type bttvEmote struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	ImageType string `json:"imageType"`
	UserID    string `json:"userId"`
	Animated  bool   `json:"animated"`
}

type remoteEmote struct {
	Provider        Provider
	ProviderEmoteID string
	ProviderSetID   string
	Name            string
	OwnerID         string
	SourceURL       string
	MimeType        string
	Animated        bool
	ZeroWidth       bool
	IsGlobal        bool
}

func (s *Seeder) SeedChannel(ctx context.Context, twitchID string) error {
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return err
	}
	login := strings.ToLower(strings.TrimSpace(u.User.Username))
	if login == "" {
		login = twitchID
	}
	setID, err := s.prepareSet(ctx, twitchID, login, u.User.DisplayName, []Provider{ProviderSevenTV})
	if err != nil {
		return err
	}
	if _, err := s.seedSevenTVUser(ctx, twitchID, setID, u); err != nil {
		return err
	}
	return s.rebuildChannelDictionary(ctx, login)
}

func (s *Seeder) SeedChannelProviders(ctx context.Context, login, twitchID string, providers []Provider) ([]ProviderResult, error) {
	return s.SeedChannelProviderSubset(ctx, login, twitchID, providers, providers)
}

func (s *Seeder) SeedChannelProviderSubset(ctx context.Context, login, twitchID string, setProviders, seedProviders []Provider) ([]ProviderResult, error) {
	if len(setProviders) == 0 {
		setProviders = []Provider{ProviderSevenTV}
	}
	if len(seedProviders) == 0 {
		seedProviders = setProviders
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		login = twitchID
	}
	setID, err := s.prepareSet(ctx, twitchID, login, login, setProviders)
	if err != nil {
		return nil, err
	}

	results := make([]ProviderResult, 0, len(seedProviders))
	failed := 0
	for _, provider := range seedProviders {
		result := ProviderResult{Provider: string(provider), State: "processing"}
		providerStarted := time.Now()
		var count int
		var err error
		var importResult ImportResult
		switch provider {
		case ProviderSevenTV:
			importResult, err = s.seedSevenTVWithResult(ctx, twitchID, setID)
			count = importResult.Imported
			result.ExpectedCount = importResult.Expected
		case ProviderTwitch:
			importResult, err = s.seedTwitchWithResult(ctx, twitchID, setID)
			count = importResult.Imported
			result.ExpectedCount = importResult.Expected
		case ProviderFFZ:
			importResult, err = s.seedFFZWithResult(ctx, login, twitchID, setID)
			count = importResult.Imported
			result.ExpectedCount = importResult.Expected
		case ProviderBTTV:
			importResult, err = s.seedBTTVWithResult(ctx, twitchID, setID)
			count = importResult.Imported
			result.ExpectedCount = importResult.Expected
		default:
			err = fmt.Errorf("unsupported provider %s", provider)
		}
		result.Count = count
		result.DurationMS = time.Since(providerStarted).Milliseconds()
		if err != nil {
			failed++
			result.State = "failed"
			result.Error = err.Error()
			if s.log != nil {
				s.log.Warn("provider seed failed", "provider", provider, "login", login, "err", err)
			}
		} else if result.ExpectedCount > 0 && result.Count < result.ExpectedCount {
			result.State = "partial"
		} else {
			result.State = "ready"
		}
		if err := s.st.UpsertChannelProviderLoad(ctx, twitchID, string(provider), result.State, result.Count, result.ExpectedCount, result.Count, result.Error); err != nil && s.log != nil {
			s.log.Warn("record provider seed state failed", "provider", provider, "login", login, "err", err)
		}
		results = append(results, result)
	}

	if err := s.rebuildChannelDictionary(ctx, login); err != nil && failed == len(seedProviders) {
		return results, err
	}
	if failed == len(seedProviders) {
		return results, fmt.Errorf("all emote providers failed")
	}
	return results, nil
}

func (s *Seeder) prepareSet(ctx context.Context, twitchID, login, displayName string, providers []Provider) (string, error) {
	if displayName == "" {
		displayName = login
	}
	if err := s.st.UpsertChannel(ctx, twitchID, login, displayName); err != nil {
		return "", err
	}
	setID, err := s.st.UpsertEmoteSetByOwnerName(ctx, providerSetName(login, providers), twitchID)
	if err != nil {
		return "", err
	}
	if err := s.st.SetActiveEmoteSet(ctx, twitchID, setID); err != nil {
		return "", err
	}
	return setID, nil
}

func providerSetName(login string, providers []Provider) string {
	parts := make([]string, 0, len(providers))
	for _, provider := range providers {
		parts = append(parts, string(provider))
	}
	return fmt.Sprintf("%s provider emotes (%s)", login, strings.Join(parts, "+"))
}

func (s *Seeder) seedSevenTV(ctx context.Context, twitchID, setID string) (int, error) {
	result, err := s.seedSevenTVWithResult(ctx, twitchID, setID)
	return result.Imported, err
}

func (s *Seeder) seedSevenTVWithResult(ctx context.Context, twitchID, setID string) (ImportResult, error) {
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		if !errors.Is(err, errProviderNotFound) {
			return ImportResult{}, err
		}
		// Channel has no 7TV account — still import 7TV globals below.
		u = &sevenTVUser{}
		if s.log != nil {
			s.log.Info("7tv user not registered; importing globals only", "twitch_id", twitchID)
		}
	}
	if u.User.Username != "" {
		_ = s.st.UpsertChannel(ctx, twitchID, strings.ToLower(u.User.Username), u.User.DisplayName)
	}
	userResult, err := s.seedSevenTVUserWithResult(ctx, twitchID, setID, u)
	if err != nil {
		return userResult, err
	}
	global, gErr := s.fetchSevenTVGlobal(ctx)
	if gErr != nil && s.log != nil {
		s.log.Warn("7tv global fetch failed", "twitch_id", twitchID, "err", gErr)
		return userResult, nil
	}
	if len(global) == 0 {
		return userResult, nil
	}
	globalEmotes := make([]remoteEmote, 0, len(global))
	for _, em := range global {
		if em.ID == "" || em.Name == "" {
			continue
		}
		globalEmotes = append(globalEmotes, remoteEmote{
			Provider:        ProviderSevenTV,
			ProviderEmoteID: em.ID,
			ProviderSetID:   "global",
			Name:            em.Name,
			OwnerID:         twitchID,
			SourceURL:       fmt.Sprintf("%s/emote/%s/4x.webp", s.cdnURL, em.ID),
			MimeType:        "image/webp",
			Animated:        em.Data.Animated,
			ZeroWidth:       sevenTVZeroWidth(em),
			IsGlobal:        true,
		})
	}
	sortRemoteEmotes(globalEmotes)
	globalResult, importErr := s.importRemoteEmotesToSet(ctx, setID, globalEmotes)
	userResult.Imported += globalResult.Imported
	userResult.Expected += globalResult.Expected
	userResult.Failed += globalResult.Failed
	if importErr != nil {
		return userResult, importErr
	}
	return userResult, nil
}

func (s *Seeder) SevenTVSnapshot(ctx context.Context, twitchID string) (ProviderSnapshot, error) {
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return ProviderSnapshot{}, err
	}
	if u.EmoteSet == nil {
		return ProviderSnapshot{}, nil
	}
	return ProviderSnapshot{
		SetID:     u.EmoteSet.ID,
		Count:     len(u.EmoteSet.Emotes),
		EmoteHash: sevenTVEmoteHash(u),
	}, nil
}

func (s *Seeder) SevenTVSnapshotDetail(ctx context.Context, twitchID string) (ProviderSnapshotDetail, error) {
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return ProviderSnapshotDetail{}, err
	}
	if u.EmoteSet == nil {
		return ProviderSnapshotDetail{Provider: string(ProviderSevenTV)}, nil
	}
	items := make([]ProviderSnapshotItem, 0, len(u.EmoteSet.Emotes))
	for _, em := range u.EmoteSet.Emotes {
		if em.ID == "" || em.Name == "" {
			continue
		}
		items = append(items, ProviderSnapshotItem{
			ProviderEmoteID: em.ID,
			ProviderSetID:   u.EmoteSet.ID,
			Alias:           em.Name,
			CanonicalName:   em.Name,
			SourceURL:       fmt.Sprintf("%s/emote/%s/4x.webp", s.cdnURL, em.ID),
			Flags:           remoteEmoteFlags(remoteEmote{ZeroWidth: sevenTVZeroWidth(em), Animated: em.Data.Animated}),
			Animated:        em.Data.Animated,
			ZeroWidth:       sevenTVZeroWidth(em),
		})
	}
	return ProviderSnapshotDetail{Provider: string(ProviderSevenTV), SetID: u.EmoteSet.ID, Items: items}, nil
}

func (s *Seeder) fetchSevenTVGlobal(ctx context.Context) ([]sevenTVEmote, error) {
	url := fmt.Sprintf("%s/emote-sets/global", s.apiURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch 7tv global: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("7tv global returned %d", resp.StatusCode)
	}
	var payload struct {
		Emotes []sevenTVEmote `json:"emotes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode 7tv global: %w", err)
	}
	return payload.Emotes, nil
}

func (s *Seeder) fetchSevenTVUser(ctx context.Context, twitchID string) (*sevenTVUser, error) {
	url := fmt.Sprintf("%s/users/twitch/%s", s.apiURL, twitchID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch 7tv user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("7tv user %s: %w", twitchID, errProviderNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("7tv returned %d", resp.StatusCode)
	}
	var u sevenTVUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("decode 7tv user: %w", err)
	}
	return &u, nil
}

func sevenTVZeroWidth(em sevenTVEmote) bool {
	return flags.FromSevenTV(em.Flags, em.Data.Flags)
}

func remoteEmoteFlags(em remoteEmote) int {
	return flags.Pack(em.ZeroWidth, em.Animated)
}

func (s *Seeder) SyncSevenTVEmoteFlags(ctx context.Context, twitchID string) (int, error) {
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return 0, err
	}
	if u.EmoteSet == nil {
		return 0, nil
	}
	updated := 0
	for _, em := range u.EmoteSet.Emotes {
		existing, err := s.st.GetProviderEmote(ctx, string(ProviderSevenTV), em.ID)
		if err != nil {
			continue
		}
		want := remoteEmoteFlags(remoteEmote{
			ZeroWidth: sevenTVZeroWidth(em),
			Animated:  em.Data.Animated,
		})
		if existing.Flags == want {
			continue
		}
		if err := s.st.UpdateEmoteFlags(ctx, existing.ID, want); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func (s *Seeder) seedSevenTVUser(ctx context.Context, twitchID, setID string, u *sevenTVUser) (int, error) {
	result, err := s.seedSevenTVUserWithResult(ctx, twitchID, setID, u)
	return result.Imported, err
}

func (s *Seeder) seedSevenTVUserWithResult(ctx context.Context, twitchID, setID string, u *sevenTVUser) (ImportResult, error) {
	if u.EmoteSet == nil {
		return ImportResult{}, nil
	}
	emotes := make([]remoteEmote, 0, len(u.EmoteSet.Emotes))
	for _, em := range u.EmoteSet.Emotes {
		emotes = append(emotes, remoteEmote{
			Provider:        ProviderSevenTV,
			ProviderEmoteID: em.ID,
			ProviderSetID:   u.EmoteSet.ID,
			Name:            em.Name,
			OwnerID:         twitchID,
			SourceURL:       fmt.Sprintf("%s/emote/%s/4x.webp", s.cdnURL, em.ID),
			MimeType:        "image/webp",
			Animated:        em.Data.Animated,
			ZeroWidth:       sevenTVZeroWidth(em),
		})
	}
	sortRemoteEmotes(emotes)
	result, err := s.importRemoteEmotesToSet(ctx, setID, emotes)
	if err != nil {
		return result, err
	}
	if err := s.syncSevenTVSet(ctx, setID, twitchID, u); err != nil {
		return result, err
	}
	if _, err := s.SyncSevenTVEmoteFlags(ctx, twitchID); err != nil && s.log != nil {
		s.log.Warn("sync 7tv zero-width flags", "twitch_id", twitchID, "err", err)
	}
	return result, nil
}

func (s *Seeder) seedFFZ(ctx context.Context, login, twitchID, setID string) (int, error) {
	result, err := s.seedFFZWithResult(ctx, login, twitchID, setID)
	return result.Imported, err
}

func (s *Seeder) seedFFZWithResult(ctx context.Context, login, twitchID, setID string) (ImportResult, error) {
	resp, err := s.fetchFFZRoom(ctx, login, twitchID)
	if err != nil {
		if !errors.Is(err, errProviderNotFound) {
			return ImportResult{}, err
		}
		// Channel has no FFZ room — still import FFZ globals below.
		resp = &ffzResponse{}
		if s.log != nil {
			s.log.Info("ffz room not registered; importing globals only", "login", login, "twitch_id", twitchID)
		}
	}
	if resp.Room.ID != "" || resp.Room.DisplayName != "" {
		roomLogin := strings.ToLower(strings.TrimSpace(resp.Room.ID))
		if roomLogin == "" {
			roomLogin = login
		}
		_ = s.st.UpsertChannel(ctx, twitchID, roomLogin, resp.Room.DisplayName)
	}
	var emotes []remoteEmote
	appendFFZSets := func(sets map[string]ffzSet, isGlobal bool) error {
		for key, set := range sets {
			providerSetID := key
			if set.ID != 0 {
				providerSetID = strconv.FormatInt(set.ID, 10)
			}
			for _, em := range set.Emoticons {
				if err := ctx.Err(); err != nil {
					return err
				}
				sourceURL := bestFFZURL(em.URLs)
				if sourceURL == "" || em.Name == "" || em.ID == 0 {
					continue
				}
				emotes = append(emotes, remoteEmote{
					Provider:        ProviderFFZ,
					ProviderEmoteID: strconv.FormatInt(em.ID, 10),
					ProviderSetID:   providerSetID,
					Name:            em.Name,
					OwnerID:         twitchID,
					SourceURL:       sourceURL,
					Animated:        rawTruthy(em.Animated),
					ZeroWidth:       em.Modifier,
					IsGlobal:        isGlobal,
				})
			}
		}
		return nil
	}
	if err := appendFFZSets(resp.Sets, false); err != nil {
		return ImportResult{}, err
	}
	globalSets, gErr := s.fetchFFZGlobalSets(ctx)
	if gErr != nil {
		if s.log != nil {
			s.log.Warn("ffz global fetch failed", "twitch_id", twitchID, "err", gErr)
		}
	} else if err := appendFFZSets(globalSets, true); err != nil {
		return ImportResult{}, err
	}
	sortRemoteEmotes(emotes)
	return s.importRemoteEmotesToSet(ctx, setID, emotes)
}

// fetchFFZGlobalSets returns the FFZ global emote sets (default_sets only).
func (s *Seeder) fetchFFZGlobalSets(ctx context.Context) (map[string]ffzSet, error) {
	url := fmt.Sprintf("%s/set/global", s.ffzURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ffz global: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ffz global returned %d", resp.StatusCode)
	}
	var out ffzGlobalResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode ffz global: %w", err)
	}
	if len(out.DefaultSets) == 0 {
		return out.Sets, nil
	}
	wanted := make(map[string]struct{}, len(out.DefaultSets))
	for _, id := range out.DefaultSets {
		wanted[strconv.FormatInt(id, 10)] = struct{}{}
	}
	sets := make(map[string]ffzSet, len(out.DefaultSets))
	for key, set := range out.Sets {
		setKey := key
		if set.ID != 0 {
			setKey = strconv.FormatInt(set.ID, 10)
		}
		if _, ok := wanted[setKey]; ok {
			sets[key] = set
		}
	}
	return sets, nil
}

func (s *Seeder) seedBTTV(ctx context.Context, twitchID, setID string) (int, error) {
	result, err := s.seedBTTVWithResult(ctx, twitchID, setID)
	return result.Imported, err
}

func (s *Seeder) seedBTTVWithResult(ctx context.Context, twitchID, setID string) (ImportResult, error) {
	user, err := s.fetchBTTVUser(ctx, twitchID)
	if err != nil {
		if !errors.Is(err, errProviderNotFound) {
			return ImportResult{}, err
		}
		// Channel has no BTTV account — still import BTTV globals below.
		user = &bttvUserResponse{}
		if s.log != nil {
			s.log.Info("bttv user not registered; importing globals only", "twitch_id", twitchID)
		}
	}
	global, err := s.fetchBTTVGlobal(ctx)
	if err != nil && s.log != nil {
		s.log.Warn("bttv global fetch failed", "twitch_id", twitchID, "err", err)
	}
	emotes := make([]remoteEmote, 0, len(user.ChannelEmotes)+len(user.SharedEmotes)+len(global))
	appendBTTV := func(rows []bttvEmote, isGlobal bool) {
		for _, em := range rows {
			if em.ID == "" || em.Code == "" {
				continue
			}
			emotes = append(emotes, remoteEmote{
				Provider:        ProviderBTTV,
				ProviderEmoteID: em.ID,
				ProviderSetID:   em.UserID,
				Name:            em.Code,
				OwnerID:         twitchID,
				SourceURL:       bttvEmoteURL(em.ID),
				Animated:        em.Animated,
				IsGlobal:        isGlobal,
			})
		}
	}
	appendBTTV(user.ChannelEmotes, false)
	appendBTTV(user.SharedEmotes, false)
	appendBTTV(global, true)
	sortRemoteEmotes(emotes)
	return s.importRemoteEmotesToSet(ctx, setID, emotes)
}

func bttvEmoteURL(id string) string {
	return fmt.Sprintf("https://cdn.betterttv.net/emote/%s/3x", id)
}

func (s *Seeder) fetchBTTVUser(ctx context.Context, twitchID string) (*bttvUserResponse, error) {
	url := fmt.Sprintf("%s/users/twitch/%s", s.bttvURL, twitchID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch bttv user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("bttv user %s: %w", twitchID, errProviderNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bttv returned %d", resp.StatusCode)
	}
	var out bttvUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode bttv user: %w", err)
	}
	return &out, nil
}

func (s *Seeder) fetchBTTVGlobal(ctx context.Context) ([]bttvEmote, error) {
	url := fmt.Sprintf("%s/emotes/global", s.bttvURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch bttv global: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bttv global returned %d", resp.StatusCode)
	}
	var out []bttvEmote
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode bttv global: %w", err)
	}
	return out, nil
}

func (s *Seeder) seedTwitch(ctx context.Context, twitchID, setID string) (int, error) {
	result, err := s.seedTwitchWithResult(ctx, twitchID, setID)
	return result.Imported, err
}

func (s *Seeder) seedTwitchWithResult(ctx context.Context, twitchID, setID string) (ImportResult, error) {
	if s.twitch == nil || !s.twitch.Enabled() {
		return ImportResult{}, fmt.Errorf("twitch helix unavailable")
	}
	channelEmotes, err := s.twitch.ChannelEmotes(ctx, twitchID)
	if err != nil {
		return ImportResult{}, err
	}
	globalEmotes, err := s.twitch.GlobalEmotes(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	emotes := make([]remoteEmote, 0, len(channelEmotes)+len(globalEmotes))
	appendRows := func(rows []helix.ChatEmote) {
		for _, em := range rows {
			if em.ID == "" || em.Name == "" {
				continue
			}
			sourceURL := em.URL4X
			if sourceURL == "" {
				sourceURL = em.URL2X
			}
			if sourceURL == "" {
				sourceURL = em.URL1X
			}
			if sourceURL == "" {
				continue
			}
			emotes = append(emotes, remoteEmote{
				Provider:        ProviderTwitch,
				ProviderEmoteID: em.ID,
				ProviderSetID:   em.EmoteSetID,
				Name:            em.Name,
				OwnerID:         twitchID,
				SourceURL:       sourceURL,
				Animated:        em.Animated,
				IsGlobal:        em.IsGlobal,
			})
		}
	}
	appendRows(channelEmotes)
	appendRows(globalEmotes)
	sortRemoteEmotes(emotes)
	return s.importRemoteEmotesToSet(ctx, setID, emotes)
}

func (s *Seeder) fetchFFZRoom(ctx context.Context, login, twitchID string) (*ffzResponse, error) {
	urls := []string{fmt.Sprintf("%s/room/id/%s", s.ffzURL, twitchID)}
	if login != "" {
		urls = append(urls, fmt.Sprintf("%s/room/%s", s.ffzURL, login))
	}
	var lastErr error
	for _, url := range urls {
		out, err := s.fetchFFZURL(ctx, url)
		if err == nil {
			return out, nil
		}
		// Prefer surfacing a real failure over a not-registered 404.
		if lastErr == nil || !errors.Is(err, errProviderNotFound) {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ffz room unavailable")
	}
	return nil, lastErr
}

func (s *Seeder) fetchFFZURL(ctx context.Context, url string) (*ffzResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch ffz room: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("ffz room %s: %w", url, errProviderNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ffz returned %d", resp.StatusCode)
	}
	var out ffzResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode ffz room: %w", err)
	}
	return &out, nil
}

func (s *Seeder) importRemoteEmote(ctx context.Context, em remoteEmote) (string, bool, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		id, existing, err := s.importRemoteEmoteOnce(ctx, em)
		if err == nil {
			return id, existing, nil
		}
		lastErr = err
		if !isTransientImportErr(err) || attempt == maxAttempts {
			break
		}
		time.Sleep(time.Duration(attempt*100) * time.Millisecond)
	}
	return "", false, lastErr
}

func isTransientImportErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "download:") ||
		strings.Contains(msg, "store src:") ||
		strings.Contains(msg, "cdn returned 5") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout")
}

func (s *Seeder) importRemoteEmoteOnce(ctx context.Context, em remoteEmote) (string, bool, error) {
	wantFlags := remoteEmoteFlags(em)
	if existing, err := s.st.GetProviderEmote(ctx, string(em.Provider), em.ProviderEmoteID); err == nil {
		if existing.Name != em.Name && em.Name != "" {
			_ = s.st.UpdateEmoteName(ctx, existing.ID, em.Name)
		}
		if existing.Flags != wantFlags {
			_ = s.st.UpdateEmoteFlags(ctx, existing.ID, wantFlags)
		}
		if existing.SourceURL != em.SourceURL && em.SourceURL != "" {
			_ = s.st.UpdateEmoteSourceURL(ctx, existing.ID, em.SourceURL)
		}
		if s.shouldEagerRender(em.Provider) && existing.Status != 1 && existing.SourceHash != "" {
			_, _ = s.enqueueRender(ctx, existing.ID, string(em.Provider), em.ProviderEmoteID, "", render.ReasonEnsure, "")
		} else if !s.shouldEagerRender(em.Provider) && existing.Status == 0 && em.ProviderEmoteID != "" {
			_ = s.st.SetEmoteMetadataReady(ctx, existing.ID)
		}
		return existing.ID, true, nil
	} else if err != pgx.ErrNoRows {
		return "", false, err
	}

	if !s.shouldEagerRender(em.Provider) {
		return s.importMetadataOnly(ctx, em, wantFlags)
	}

	sourceCtx, cancel := context.WithTimeout(ctx, sourceDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(sourceCtx, http.MethodGet, em.SourceURL, nil)
	if err != nil {
		return "", false, err
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("cdn returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}

	flags := wantFlags
	mimeType := em.MimeType
	if mimeType == "" {
		mimeType = strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}

	sum := sha256.Sum256(data)
	e := store.Emote{
		Name:            em.Name,
		OwnerID:         em.OwnerID,
		IsGlobal:        em.IsGlobal,
		Flags:           flags,
		Animated:        em.Animated,
		MimeType:        mimeType,
		SourceHash:      hex.EncodeToString(sum[:]),
		Provider:        string(em.Provider),
		ProviderEmoteID: em.ProviderEmoteID,
		ProviderSetID:   em.ProviderSetID,
		SourceURL:       em.SourceURL,
		Status:          0,
	}

	emoteID, existing, err := s.st.UpsertEmoteByHash(ctx, e)
	if err != nil {
		return "", false, err
	}
	if existing {
		return emoteID, true, nil
	}

	if err := s.obj.PutSrc(ctx, emoteID, data, mimeType); err != nil {
		return "", false, fmt.Errorf("store src: %w", err)
	}
	if _, err := s.enqueueRender(ctx, emoteID, string(em.Provider), em.ProviderEmoteID, em.SourceURL, render.ReasonEnsure, e.SourceHash); err != nil {
		return "", false, err
	}
	return emoteID, false, nil
}

func (s *Seeder) shouldEagerRender(provider Provider) bool {
	if s.render == nil {
		return true
	}
	return s.render.ShouldEagerRender(string(provider))
}

func (s *Seeder) importMetadataOnly(ctx context.Context, em remoteEmote, wantFlags int) (string, bool, error) {
	e := store.Emote{
		Name:            em.Name,
		OwnerID:         em.OwnerID,
		IsGlobal:        em.IsGlobal,
		Flags:           wantFlags,
		Animated:        em.Animated,
		MimeType:        em.MimeType,
		Provider:        string(em.Provider),
		ProviderEmoteID: em.ProviderEmoteID,
		ProviderSetID:   em.ProviderSetID,
		SourceURL:       em.SourceURL,
		Status:          1,
	}
	if e.MimeType == "" {
		e.MimeType = "image/webp"
	}
	emoteID, existing, err := s.st.UpsertEmoteByHash(ctx, e)
	if err != nil {
		return "", false, err
	}
	return emoteID, existing, nil
}

func (s *Seeder) enqueueRender(ctx context.Context, emoteID, provider, providerEmoteID, sourceURL string, reason render.Reason, sourceHash string) (bool, error) {
	if s.render == nil {
		_, err := s.st.InsertJob(ctx, emoteID, render.JobSourceKey(sourceHash, s.defaultRenderScales("")))
		return err == nil, err
	}
	return s.render.Enqueue(ctx, render.Request{
		EmoteID:         emoteID,
		Provider:        provider,
		ProviderEmoteID: providerEmoteID,
		SourceURL:       sourceURL,
		SourceHash:      sourceHash,
		Reason:          reason,
	})
}

func (s *Seeder) defaultRenderScales(scale string) []string {
	if s.render == nil {
		return []string{"1x", "2x", "3x", "4x"}
	}
	return s.render.DefaultScales(scale)
}

func (s *Seeder) importRemoteEmotesToSet(ctx context.Context, setID string, emotes []remoteEmote) (ImportResult, error) {
	result := ImportResult{Expected: len(emotes)}
	if len(emotes) == 0 {
		return result, nil
	}
	concurrency := minInt(s.importConcurrency, len(emotes))
	jobs := make(chan remoteEmote)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	setFirstErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for em := range jobs {
				if err := ctx.Err(); err != nil {
					setFirstErr(err)
					mu.Lock()
					result.Failed++
					mu.Unlock()
					continue
				}
				emoteID, _, err := s.importRemoteEmote(ctx, em)
				if err != nil {
					if s.log != nil {
						s.log.Warn("skip provider emote", "provider", em.Provider, "name", em.Name, "err", err)
					}
					mu.Lock()
					result.Failed++
					mu.Unlock()
					continue
				}
				if setID != "" {
					if err := s.st.AddEmoteToSet(ctx, setID, emoteID, nil); err != nil {
						setFirstErr(err)
						mu.Lock()
						result.Failed++
						mu.Unlock()
						continue
					}
				}
				mu.Lock()
				result.Imported++
				mu.Unlock()
			}
		}()
	}

	for _, em := range emotes {
		if err := ctx.Err(); err != nil {
			setFirstErr(err)
			break
		}
		jobs <- em
	}
	close(jobs)
	wg.Wait()
	return result, firstErr
}

func (s *Seeder) rebuildChannelDictionary(ctx context.Context, login string) error {
	if s.d == nil {
		return nil
	}
	emotes, err := s.st.GetChannelEmotes(ctx, login)
	if err != nil {
		return err
	}
	entries := make([]dict.EmoteEntry, 0, len(emotes))
	for _, e := range emotes {
		entries = append(entries, dict.EmoteEntry{
			Name:            e.Name,
			EmoteID:         e.EmoteID,
			ProviderEmoteID: e.ProviderEmoteID,
			ZeroWidth:       flags.IsZeroWidth(e.Flags),
			Provider:        e.Provider,
		})
	}
	return s.d.Rebuild(ctx, login, entries)
}

func bestFFZURL(urls map[string]string) string {
	bestScale := -1
	bestURL := ""
	for scale, rawURL := range urls {
		parsed, err := strconv.Atoi(scale)
		if err != nil {
			parsed = 0
		}
		if parsed >= bestScale {
			bestScale = parsed
			bestURL = normalizeURL(rawURL)
		}
	}
	return bestURL
}

func normalizeURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	return rawURL
}

func rawTruthy(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value != "" && value != "null" && value != "false" && value != "{}" && value != "[]"
}

func sortRemoteEmotes(emotes []remoteEmote) {
	sort.SliceStable(emotes, func(i, j int) bool {
		left := emotes[i]
		right := emotes[j]
		if left.ZeroWidth != right.ZeroWidth {
			return !left.ZeroWidth
		}
		if left.Animated != right.Animated {
			return !left.Animated
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
}

func normalizeImportConcurrency(value int) int {
	if value < 1 {
		return 1
	}
	if value > 32 {
		return 32
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
