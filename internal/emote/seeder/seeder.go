package seeder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"streamclone/internal/emote/objstore"
	"streamclone/internal/emote/store"
	"streamclone/internal/metadata/helix"
)

type Provider string

const (
	ProviderSevenTV       Provider = "seventv"
	ProviderTwitch        Provider = "twitch"
	ProviderFFZ           Provider = "ffz"
	sourceDownloadTimeout          = 6 * time.Second
)

type ProviderResult struct {
	Provider   string
	State      string
	Count      int
	Error      string
	DurationMS int64
}

type ProviderSnapshot struct {
	SetID string
	Count int
}

type Seeder struct {
	st                *store.Store
	obj               *objstore.Client
	d                 *dict.Dict
	log               *slog.Logger
	apiURL            string
	cdnURL            string
	ffzURL            string
	twitch            *helix.Client
	hc                *http.Client
	importConcurrency int
}

func New(st *store.Store, obj *objstore.Client, d *dict.Dict, log *slog.Logger, apiURL, cdnURL, ffzURL string, twitch *helix.Client) *Seeder {
	return NewWithImportConcurrency(st, obj, d, log, apiURL, cdnURL, ffzURL, twitch, 8)
}

func NewWithImportConcurrency(st *store.Store, obj *objstore.Client, d *dict.Dict, log *slog.Logger, apiURL, cdnURL, ffzURL string, twitch *helix.Client, importConcurrency int) *Seeder {
	return &Seeder{
		st:                st,
		obj:               obj,
		d:                 d,
		log:               log,
		apiURL:            strings.TrimRight(apiURL, "/"),
		cdnURL:            strings.TrimRight(cdnURL, "/"),
		ffzURL:            strings.TrimRight(ffzURL, "/"),
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
	ID   string `json:"id"`
	Name string `json:"name"`
	Data struct {
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
		switch provider {
		case ProviderSevenTV:
			count, err = s.seedSevenTV(ctx, twitchID, setID)
		case ProviderTwitch:
			count, err = s.seedTwitch(ctx, twitchID, setID)
		case ProviderFFZ:
			count, err = s.seedFFZ(ctx, login, twitchID, setID)
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
		} else {
			result.State = "ready"
		}
		if err := s.st.UpsertChannelProviderLoad(ctx, twitchID, string(provider), result.State, result.Count, result.Error); err != nil && s.log != nil {
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
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return 0, err
	}
	if u.User.Username != "" {
		_ = s.st.UpsertChannel(ctx, twitchID, strings.ToLower(u.User.Username), u.User.DisplayName)
	}
	return s.seedSevenTVUser(ctx, twitchID, setID, u)
}

func (s *Seeder) SevenTVSnapshot(ctx context.Context, twitchID string) (ProviderSnapshot, error) {
	u, err := s.fetchSevenTVUser(ctx, twitchID)
	if err != nil {
		return ProviderSnapshot{}, err
	}
	if u.EmoteSet == nil {
		return ProviderSnapshot{}, nil
	}
	return ProviderSnapshot{SetID: u.EmoteSet.ID, Count: len(u.EmoteSet.Emotes)}, nil
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("7tv returned %d", resp.StatusCode)
	}
	var u sevenTVUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("decode 7tv user: %w", err)
	}
	return &u, nil
}

func (s *Seeder) seedSevenTVUser(ctx context.Context, twitchID, setID string, u *sevenTVUser) (int, error) {
	if u.EmoteSet == nil {
		return 0, nil
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
			ZeroWidth:       em.Data.Flags&1 != 0,
		})
	}
	sortRemoteEmotes(emotes)
	return s.importRemoteEmotesToSet(ctx, setID, emotes)
}

func (s *Seeder) seedFFZ(ctx context.Context, login, twitchID, setID string) (int, error) {
	resp, err := s.fetchFFZRoom(ctx, login, twitchID)
	if err != nil {
		return 0, err
	}
	if resp.Room.ID != "" || resp.Room.DisplayName != "" {
		roomLogin := strings.ToLower(strings.TrimSpace(resp.Room.ID))
		if roomLogin == "" {
			roomLogin = login
		}
		_ = s.st.UpsertChannel(ctx, twitchID, roomLogin, resp.Room.DisplayName)
	}
	var emotes []remoteEmote
	for key, set := range resp.Sets {
		providerSetID := key
		if set.ID != 0 {
			providerSetID = strconv.FormatInt(set.ID, 10)
		}
		for _, em := range set.Emoticons {
			if err := ctx.Err(); err != nil {
				return 0, err
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
			})
		}
	}
	sortRemoteEmotes(emotes)
	return s.importRemoteEmotesToSet(ctx, setID, emotes)
}

func (s *Seeder) seedTwitch(ctx context.Context, twitchID, setID string) (int, error) {
	if s.twitch == nil || !s.twitch.Enabled() {
		return 0, fmt.Errorf("twitch helix unavailable")
	}
	channelEmotes, err := s.twitch.ChannelEmotes(ctx, twitchID)
	if err != nil {
		return 0, err
	}
	globalEmotes, err := s.twitch.GlobalEmotes(ctx)
	if err != nil {
		return 0, err
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
		lastErr = err
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
	if existing, err := s.st.GetProviderEmote(ctx, string(em.Provider), em.ProviderEmoteID); err == nil {
		if existing.Status != 1 && existing.SourceHash != "" {
			_, _ = s.st.InsertJob(ctx, existing.ID, existing.SourceHash)
		}
		return existing.ID, true, nil
	} else if err != pgx.ErrNoRows {
		return "", false, err
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

	flags := 0
	if em.ZeroWidth {
		flags |= 1
	}
	if em.Animated {
		flags |= 2
	}
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
	if _, err := s.st.InsertJob(ctx, emoteID, e.SourceHash); err != nil {
		return "", false, err
	}
	return emoteID, false, nil
}

func (s *Seeder) importRemoteEmotesToSet(ctx context.Context, setID string, emotes []remoteEmote) (int, error) {
	if len(emotes) == 0 {
		return 0, nil
	}
	concurrency := minInt(s.importConcurrency, len(emotes))
	jobs := make(chan remoteEmote)

	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
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
					continue
				}
				emoteID, _, err := s.importRemoteEmote(ctx, em)
				if err != nil {
					if s.log != nil {
						s.log.Warn("skip provider emote", "provider", em.Provider, "name", em.Name, "err", err)
					}
					continue
				}
				if setID != "" {
					if err := s.st.AddEmoteToSet(ctx, setID, emoteID, nil); err != nil {
						setFirstErr(err)
						continue
					}
				}
				mu.Lock()
				count++
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
	return count, firstErr
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
			Name:      e.Name,
			EmoteID:   e.EmoteID,
			ZeroWidth: e.Flags&1 != 0,
			Provider:  e.Provider,
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
