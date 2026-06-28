package analytics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	EmoteIdentityProviderID    = "provider_id"
	EmoteIdentityAliasFallback = "alias_fallback"
	EmoteIdentityAmbiguous     = "ambiguous"
	EmoteIdentityUnresolved    = "unresolved"
)

type EmoteSnapshotItem struct {
	Provider        string
	ProviderEmoteID string
	ProviderSetID   string
	Alias           string
	CanonicalName   string
	SourceURL       string
	AssetHash       string
	Flags           int
	Animated        bool
	ZeroWidth       bool
	SortKey         string
}

type EmoteSnapshotDiff struct {
	Added        []EmoteSnapshotItem
	Removed      []EmoteSnapshotItem
	Readded      []EmoteSnapshotItem
	AliasChanges []EmoteAliasChange
}

type EmoteAliasChange struct {
	Provider        string
	ProviderEmoteID string
	FromAlias       string
	ToAlias         string
}

type ParsedEmoteRollupKey struct {
	Provider string
	ID       string
	Name     string
	Raw      string
}

type EmoteIdentityCandidate struct {
	Provider        string
	ProviderEmoteID string
	Name            string
}

type EmoteIdentityResolution struct {
	Provider        string
	ProviderEmoteID string
	Name            string
	Resolution      string
	Confidence      float64
}

func NormalizeEmoteSnapshotItems(provider string, rows []EmoteSnapshotItem) []EmoteSnapshotItem {
	provider = normalizeProvider(provider)
	out := make([]EmoteSnapshotItem, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		item := row
		item.Provider = normalizeProvider(firstNonEmptyEmoteValue(item.Provider, provider))
		item.ProviderEmoteID = strings.TrimSpace(item.ProviderEmoteID)
		item.ProviderSetID = strings.TrimSpace(item.ProviderSetID)
		item.Alias = strings.TrimSpace(item.Alias)
		item.CanonicalName = strings.TrimSpace(item.CanonicalName)
		item.SourceURL = strings.TrimSpace(item.SourceURL)
		item.AssetHash = strings.TrimSpace(item.AssetHash)
		if item.CanonicalName == "" {
			item.CanonicalName = item.Alias
		}
		if item.Provider == "" || item.ProviderEmoteID == "" || item.Alias == "" {
			continue
		}
		item.SortKey = snapshotItemSortKey(item)
		if _, ok := seen[item.SortKey]; ok {
			continue
		}
		seen[item.SortKey] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SortKey < out[j].SortKey })
	return out
}

func StableEmoteSnapshotHash(items []EmoteSnapshotItem) string {
	normalized := NormalizeEmoteSnapshotItems("", items)
	encoded := make([]stableEmoteSnapshotItem, 0, len(normalized))
	for _, item := range normalized {
		encoded = append(encoded, stableEmoteSnapshotItem{
			Provider:        item.Provider,
			ProviderEmoteID: item.ProviderEmoteID,
			ProviderSetID:   item.ProviderSetID,
			Alias:           item.Alias,
			CanonicalName:   item.CanonicalName,
			SourceURL:       item.SourceURL,
			AssetHash:       item.AssetHash,
			Flags:           item.Flags,
			Animated:        item.Animated,
			ZeroWidth:       item.ZeroWidth,
		})
	}
	body, _ := json.Marshal(encoded)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func DiffEmoteSnapshots(previous, current []EmoteSnapshotItem, previouslySeen map[string]struct{}) EmoteSnapshotDiff {
	prevByID := snapshotIdentityMap(NormalizeEmoteSnapshotItems("", previous))
	currByID := snapshotIdentityMap(NormalizeEmoteSnapshotItems("", current))
	out := EmoteSnapshotDiff{}
	for key, curr := range currByID {
		prev, existed := prevByID[key]
		if !existed {
			if previouslySeen != nil {
				if _, seen := previouslySeen[key]; seen {
					out.Readded = append(out.Readded, curr)
					continue
				}
			}
			out.Added = append(out.Added, curr)
			continue
		}
		if prev.Alias != curr.Alias {
			out.AliasChanges = append(out.AliasChanges, EmoteAliasChange{
				Provider:        curr.Provider,
				ProviderEmoteID: curr.ProviderEmoteID,
				FromAlias:       prev.Alias,
				ToAlias:         curr.Alias,
			})
		}
	}
	for key, prev := range prevByID {
		if _, ok := currByID[key]; !ok {
			out.Removed = append(out.Removed, prev)
		}
	}
	sortSnapshotItems(out.Added)
	sortSnapshotItems(out.Removed)
	sortSnapshotItems(out.Readded)
	sort.Slice(out.AliasChanges, func(i, j int) bool {
		left := out.AliasChanges[i].Provider + ":" + out.AliasChanges[i].ProviderEmoteID
		right := out.AliasChanges[j].Provider + ":" + out.AliasChanges[j].ProviderEmoteID
		return left < right
	})
	return out
}

func ParseEmoteRollupKey(raw string) ParsedEmoteRollupKey {
	key := strings.TrimSpace(raw)
	parts := strings.SplitN(key, ":", 3)
	if len(parts) == 3 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
		return ParsedEmoteRollupKey{Provider: normalizeProvider(parts[0]), ID: strings.TrimSpace(parts[1]), Name: strings.TrimSpace(parts[2]), Raw: key}
	}
	if len(parts) == 2 && isKnownEmoteProvider(parts[0]) && strings.TrimSpace(parts[1]) != "" {
		return ParsedEmoteRollupKey{Provider: normalizeProvider(parts[0]), Name: strings.TrimSpace(parts[1]), Raw: key}
	}
	if left, right, ok := strings.Cut(key, "/"); ok && isKnownEmoteProvider(left) && strings.TrimSpace(right) != "" {
		return ParsedEmoteRollupKey{Provider: normalizeProvider(left), Name: strings.TrimSpace(right), Raw: key}
	}
	return ParsedEmoteRollupKey{Name: key, Raw: key}
}

func ResolveEmoteIdentityAt(key ParsedEmoteRollupKey, candidates []EmoteIdentityCandidate) EmoteIdentityResolution {
	if key.Provider != "" && key.ID != "" {
		return EmoteIdentityResolution{
			Provider:        key.Provider,
			ProviderEmoteID: key.ID,
			Name:            key.Name,
			Resolution:      EmoteIdentityProviderID,
			Confidence:      1,
		}
	}
	name := strings.TrimSpace(key.Name)
	if name == "" {
		return EmoteIdentityResolution{Name: name, Resolution: EmoteIdentityUnresolved, Confidence: 0}
	}
	matches := make([]EmoteIdentityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Provider = normalizeProvider(candidate.Provider)
		if key.Provider != "" && candidate.Provider != key.Provider {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(candidate.Name), name) {
			candidate.ProviderEmoteID = strings.TrimSpace(candidate.ProviderEmoteID)
			candidate.Name = strings.TrimSpace(candidate.Name)
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 1 {
		return EmoteIdentityResolution{
			Provider:        matches[0].Provider,
			ProviderEmoteID: matches[0].ProviderEmoteID,
			Name:            name,
			Resolution:      EmoteIdentityAliasFallback,
			Confidence:      0.65,
		}
	}
	if len(matches) > 1 {
		return EmoteIdentityResolution{Name: name, Resolution: EmoteIdentityAmbiguous, Confidence: 0.25}
	}
	return EmoteIdentityResolution{Name: name, Resolution: EmoteIdentityUnresolved, Confidence: 0}
}

func SnapshotShouldCreateHistory(previousHash, currentHash string, fetchOK bool) bool {
	if !fetchOK {
		return false
	}
	currentHash = strings.TrimSpace(currentHash)
	if currentHash == "" {
		return false
	}
	return strings.TrimSpace(previousHash) != currentHash
}

func emoteHistoryNow() time.Time {
	return time.Now().UTC()
}

type stableEmoteSnapshotItem struct {
	Provider        string `json:"provider"`
	ProviderEmoteID string `json:"providerEmoteId"`
	ProviderSetID   string `json:"providerSetId"`
	Alias           string `json:"alias"`
	CanonicalName   string `json:"canonicalName"`
	SourceURL       string `json:"sourceUrl"`
	AssetHash       string `json:"assetHash"`
	Flags           int    `json:"flags"`
	Animated        bool   `json:"animated"`
	ZeroWidth       bool   `json:"zeroWidth"`
}

func snapshotIdentityMap(items []EmoteSnapshotItem) map[string]EmoteSnapshotItem {
	out := make(map[string]EmoteSnapshotItem, len(items))
	for _, item := range items {
		out[snapshotIdentityKey(item)] = item
	}
	return out
}

func sortSnapshotItems(items []EmoteSnapshotItem) {
	sort.Slice(items, func(i, j int) bool { return items[i].SortKey < items[j].SortKey })
}

func snapshotItemSortKey(item EmoteSnapshotItem) string {
	return normalizeProvider(item.Provider) + ":" + strings.TrimSpace(item.ProviderEmoteID) + ":" + strings.TrimSpace(item.Alias)
}

func snapshotIdentityKey(item EmoteSnapshotItem) string {
	return normalizeProvider(item.Provider) + ":" + strings.TrimSpace(item.ProviderEmoteID)
}

func normalizeProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "7tv" {
		return "seventv"
	}
	return provider
}

func isKnownEmoteProvider(provider string) bool {
	switch normalizeProvider(provider) {
	case "seventv", "bttv", "ffz", "twitch":
		return true
	default:
		return false
	}
}

func firstNonEmptyEmoteValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
