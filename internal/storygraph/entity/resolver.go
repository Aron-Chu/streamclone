package entity

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"streamclone/internal/social"
	"streamclone/internal/storygraph/store"
)

// Resolver maps handles to streamer_entities rows.
type Resolver struct {
	store *store.Store
}

func New(st *store.Store) *Resolver {
	return &Resolver{store: st}
}

// ResolveTwitchLogin ensures an entity row for a Twitch login.
func (r *Resolver) ResolveTwitchLogin(ctx context.Context, login, twitchID, display string, aliases []Alias) (int64, bool, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return 0, false, nil
	}
	aliasesJSON, _ := json.Marshal(aliases)
	id, err := r.store.UpsertEntity(ctx, login, twitchID, display, aliasesJSON)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// Alias is a cross-platform handle alias.
type Alias struct {
	Platform string `json:"platform"`
	Handle   string `json:"handle"`
}

// MatchHandle resolves a social handle; returns entity id or 0 if ambiguous/unresolved.
func (r *Resolver) MatchHandle(ctx context.Context, platform, handle string) (*int64, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return nil, nil
	}
	if platform == "twitch" {
		ent, err := r.store.EntityByLogin(ctx, handle)
		if err != nil || ent == nil {
			return nil, err
		}
		return &ent.ID, nil
	}
	return r.store.EntityIDByAlias(ctx, platform, handle)
}

// AppendAlias merges one alias onto an entity without overwriting existing rows.
func (r *Resolver) AppendAlias(ctx context.Context, entityID int64, platform, handle string) error {
	return r.store.MergeEntityAliases(ctx, entityID, platform, handle)
}

// ResolveItem uses source hints to resolve or create an entity row for a social item.
func (r *Resolver) ResolveItem(ctx context.Context, item social.Item) (*int64, error) {
	login := normalizeLogin(item.EntityTwitchLogin)
	display := strings.TrimSpace(item.EntityDisplayName)
	aliases := make([]Alias, 0, len(item.EntityAliases)+1)
	for _, alias := range item.EntityAliases {
		if strings.TrimSpace(alias.Handle) == "" {
			continue
		}
		aliases = append(aliases, Alias{Platform: alias.Platform, Handle: alias.Handle})
		if login == "" && strings.EqualFold(alias.Platform, "twitch") {
			login = normalizeLogin(alias.Handle)
		}
	}
	if login == "" && display != "" {
		login = normalizeLogin(display)
	}
	if login == "" && item.Source == "twitch_clip" {
		login = normalizeLogin(item.Author)
		if display == "" {
			display = strings.TrimSpace(item.Author)
		}
	}
	if login == "" {
		return nil, nil
	}
	if ent, err := r.MatchHandle(ctx, "twitch", login); err != nil || ent != nil {
		return ent, err
	}
	aliases = appendUniqueAlias(aliases, Alias{Platform: "twitch", Handle: login})
	id, ok, err := r.ResolveTwitchLogin(ctx, login, "", display, aliases)
	if err != nil || !ok {
		return nil, err
	}
	return &id, nil
}

var loginBoundaryReplacer = regexp.MustCompile(`[^a-z0-9_]+`)
var titleNameBeforeVerbRe = regexp.MustCompile(`(?i)^([A-Z][a-z]+(?:\s+[A-Za-z][a-z0-9']*){0,2})\s+(?:gets?|got|has|had|is|was|are|were|banned|ban|says|said|does|did|streams?|reacts?|reacted|argues?|argued|calls?|called|leaves?|left|joins?|joined|wins?|won|loses?|lost|breaks?|broke|throws?|threw|goes?|went|comes?|came|tries?|tried|starts?|started|ends?|ended|posts?|posted|uploads?|uploaded|announces?|announced)\b`)

// ResolveLoginFromTitle uses known Twitch logins as hints and upserts an entity when the title clearly mentions one.
func (r *Resolver) ResolveLoginFromTitle(ctx context.Context, title string, knownLogins []string) (*int64, string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, "", nil
	}
	lowerTitle := strings.ToLower(title)
	for _, login := range knownLogins {
		login = normalizeLogin(login)
		if login == "" {
			continue
		}
		if !titleMentionsLogin(lowerTitle, login) {
			if ent, err := r.store.EntityByLogin(ctx, login); err == nil && ent != nil {
				for _, alias := range store.ParseEntityAliases(ent.Aliases) {
					if titleMentionsLogin(lowerTitle, normalizeLogin(alias.Handle)) {
						id, ok, err := r.ResolveTwitchLogin(ctx, login, "", login, nil)
						if err != nil {
							return nil, "", err
						}
						if !ok {
							return nil, "", nil
						}
						return &id, login, nil
					}
				}
			}
			continue
		}
		id, ok, err := r.ResolveTwitchLogin(ctx, login, "", login, nil)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return nil, "", nil
		}
		return &id, login, nil
	}
	for _, candidate := range titleLoginCandidates(title) {
		for _, login := range knownLogins {
			login = normalizeLogin(login)
			if login == "" {
				continue
			}
			if candidate != login {
				continue
			}
			id, ok, err := r.ResolveTwitchLogin(ctx, login, "", login, nil)
			if err != nil {
				return nil, "", err
			}
			if !ok {
				return nil, "", nil
			}
			return &id, login, nil
		}
	}
	return nil, "", nil
}

func titleLoginCandidates(title string) []string {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(value string) {
		value = normalizeLogin(value)
		if value == "" || len(value) < 3 {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if match := titleNameBeforeVerbRe.FindStringSubmatch(title); len(match) >= 2 {
		phrase := strings.TrimSpace(match[1])
		add(phrase)
		add(strings.ReplaceAll(phrase, " ", ""))
		add(strings.ReplaceAll(strings.ToLower(phrase), " ", "_"))
	}
	for _, token := range strings.Fields(loginBoundaryReplacer.ReplaceAllString(title, " ")) {
		if len(token) >= 4 && token[0] >= 'A' && token[0] <= 'Z' {
			add(token)
		}
	}
	return out
}

func titleMentionsLogin(title, login string) bool {
	if title == "" || login == "" {
		return false
	}
	if title == login {
		return true
	}
	compact := strings.ReplaceAll(title, " ", "")
	if compact == login || strings.Contains(compact, login) {
		return true
	}
	for _, field := range strings.Fields(loginBoundaryReplacer.ReplaceAllString(title, " ")) {
		if field == login {
			return true
		}
	}
	return false
}

func normalizeLogin(login string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	login = strings.TrimPrefix(login, "@")
	return strings.TrimSpace(loginBoundaryReplacer.ReplaceAllString(login, ""))
}

func appendUniqueAlias(aliases []Alias, next Alias) []Alias {
	for _, alias := range aliases {
		if strings.EqualFold(strings.TrimSpace(alias.Platform), strings.TrimSpace(next.Platform)) &&
			strings.EqualFold(strings.TrimSpace(alias.Handle), strings.TrimSpace(next.Handle)) {
			return aliases
		}
	}
	return append(aliases, next)
}

// LearnAliasesFromItem records flair/display handles learned from Reddit ingest.
func (r *Resolver) LearnAliasesFromItem(ctx context.Context, entityID int64, item social.Item) {
	if entityID <= 0 || item.Source != "reddit" {
		return
	}
	if flair := strings.TrimSpace(item.FlairText); flair != "" && strings.TrimSpace(item.EntityTwitchLogin) != "" {
		_ = r.store.MergeEntityAliases(ctx, entityID, "reddit", flair)
	}
	if display := strings.TrimSpace(item.EntityDisplayName); display != "" {
		login := normalizeLogin(item.EntityTwitchLogin)
		if login != "" && !strings.EqualFold(normalizeLogin(display), login) {
			_ = r.store.MergeEntityAliases(ctx, entityID, "reddit", display)
		}
	}
}
