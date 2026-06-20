package cluster

import (
	"context"
	"regexp"
	"strings"
	"time"

	"streamclone/internal/storygraph/matcher"
	"streamclone/internal/storygraph/store"
)

const (
	fusionLookback                  = 7 * 24 * time.Hour
	titleFusionThreshold            = 0.70
	crossSourceTitleFusionThreshold = 0.60
)

var (
	twitchBanTitlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)twitch partner[^"\n]{0,120}has been banned`),
		regexp.MustCompile(`(?i)has been banned`),
		regexp.MustCompile(`(?i)banned from twitch`),
		regexp.MustCompile(`(?i)(perma|permanently|indefinitely) banned`),
		regexp.MustCompile(`(?i)\bunbanned\b`),
		regexp.MustCompile(`(?i)\bunban\b`),
	}
	notTwitchBanTitlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)banned in`),
		regexp.MustCompile(`(?i)checking if.{0,40}banned`),
		regexp.MustCompile(`(?i)steam.{0,40}ban`),
	}
	wirePlaceholderTitlePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^\d+\s+comments?$`),
	}
)

// Service groups evidence into story clusters.
type Service struct {
	store *store.Store
}

func New(st *store.Store) *Service {
	return &Service{store: st}
}

// EnsureForMatch creates or returns a cluster for a moment fingerprint and item link.
func (s *Service) EnsureForMatch(ctx context.Context, entityID, momentFPID *int64, title string) (int64, error) {
	if momentFPID != nil {
		if id, err := s.store.ClusterByMomentFP(ctx, *momentFPID); err == nil && id > 0 {
			return id, nil
		}
	}
	return s.store.InsertCluster(ctx, store.StoryCluster{
		EntityID:   entityID,
		MomentFPID: momentFPID,
		Title:      title,
		State:      "developing",
	})
}

// EnsureWireStory creates or reuses a published cluster for a global wire item without requiring a Pulse moment link.
func (s *Service) EnsureWireStory(ctx context.Context, itemID int64, entityID *int64, title, sourceName, flairText string, canonicalURLs []string) (int64, bool, string, error) {
	evidenceTitle := evidenceHeadline(title)
	title = wireHeadlineCandidate(title, sourceName)
	if title == "" && evidenceTitle != "" {
		title = evidenceTitle
	}
	if title == "" {
		title = "Story developing"
	}
	category := wireCategory(sourceName, title, flairText)
	state := "published"
	if entityID == nil && sourceName != "streamerbans" {
		state = "unverified"
	}
	if itemID > 0 {
		if existingID, err := s.store.ClusterByItemID(ctx, itemID); err == nil && existingID > 0 {
			since := time.Now().Add(-fusionLookback)
			for _, rawURL := range canonicalURLs {
				rawURL = strings.TrimSpace(rawURL)
				if rawURL == "" {
					continue
				}
				if fusedID, err := s.store.FindClusterByCanonicalURL(ctx, rawURL, since); err == nil && fusedID > 0 && fusedID != existingID {
					if _, mergeErr := s.store.MergeDuplicateStory(ctx, existingID, fusedID, "wire_fusion", "canonical_url on item_dedup"); mergeErr == nil {
						finalTitle, finalCategory := s.mergeWireMeta(ctx, fusedID, title, category, sourceName, flairText)
						if updateErr := s.store.UpdateClusterMeta(ctx, fusedID, entityID, finalTitle, wireSummary(finalTitle), finalCategory, state); updateErr != nil {
							return 0, false, "", updateErr
						}
						return fusedID, true, "canonical_url", nil
					}
				}
			}
			finalTitle, finalCategory := s.mergeWireMeta(ctx, existingID, title, category, sourceName, flairText)
			if updateErr := s.store.UpdateClusterMeta(ctx, existingID, entityID, finalTitle, wireSummary(finalTitle), finalCategory, state); updateErr != nil {
				return 0, false, "", updateErr
			}
			return existingID, true, "item_dedup", nil
		}
	}
	since := time.Now().Add(-fusionLookback)
	for _, rawURL := range canonicalURLs {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		if id, err := s.store.FindClusterByCanonicalURL(ctx, rawURL, since); err == nil && id > 0 {
			finalTitle, finalCategory := s.mergeWireMeta(ctx, id, title, category, sourceName, flairText)
			if updateErr := s.store.UpdateClusterMeta(ctx, id, entityID, finalTitle, wireSummary(finalTitle), finalCategory, state); updateErr != nil {
				return 0, false, "", updateErr
			}
			return id, false, "canonical_url", nil
		}
	}
	if entityID != nil {
		if id, ok := s.fuseByTitleSimilarity(ctx, *entityID, title, since, entityID, category, sourceName, flairText, state); ok {
			return id, false, "title_similarity", nil
		}
	}
	if id, err := s.store.FindRecentClusterForTitle(ctx, entityID, title); err == nil && id > 0 {
		finalTitle, finalCategory := s.mergeWireMeta(ctx, id, title, category, sourceName, flairText)
		if updateErr := s.store.UpdateClusterMeta(ctx, id, entityID, finalTitle, wireSummary(finalTitle), finalCategory, state); updateErr != nil {
			return 0, false, "", updateErr
		}
		return id, false, "exact_title", nil
	}
	id, err := s.store.InsertCluster(ctx, store.StoryCluster{
		EntityID: entityID,
		Title:    title,
		Summary:  wireSummary(title),
		Category: category,
		State:    state,
	})
	if err != nil {
		return 0, false, "", err
	}
	return id, false, "new_story", nil
}

func (s *Service) fuseByTitleSimilarity(ctx context.Context, entityID int64, title string, since time.Time, mergeEntityID *int64, category, sourceName, flairText, state string) (int64, bool) {
	candidates, err := s.store.ListRecentClusterIDsForEntity(ctx, entityID, since, 10)
	if err != nil || len(candidates) == 0 {
		return 0, false
	}
	bestID := int64(0)
	bestScore := 0.0
	for _, id := range candidates {
		titles := []string{}
		if existingTitle, _, err := s.store.ClusterMeta(ctx, id); err == nil {
			titles = append(titles, existingTitle)
		}
		if headlines, err := s.store.ListEvidenceHeadlinesForCluster(ctx, id, 8); err == nil {
			titles = append(titles, headlines...)
		}
		threshold := titleFusionThreshold
		if clipRedditFusionEligible(sourceName, id, s.store, ctx) {
			threshold = crossSourceTitleFusionThreshold
		}
		for _, existingTitle := range titles {
			existingTitle = strings.TrimSpace(existingTitle)
			if existingTitle == "" {
				continue
			}
			score := matcher.TitleSimilarity(existingTitle, title)
			if score >= threshold && score > bestScore {
				bestScore = score
				bestID = id
			}
		}
	}
	if bestID == 0 {
		return 0, false
	}
	finalTitle, finalCategory := s.mergeWireMeta(ctx, bestID, title, category, sourceName, flairText)
	if updateErr := s.store.UpdateClusterMeta(ctx, bestID, mergeEntityID, finalTitle, wireSummary(finalTitle), finalCategory, state); updateErr != nil {
		return 0, false
	}
	return bestID, true
}

func clipRedditFusionEligible(sourceName string, clusterID int64, st *store.Store, ctx context.Context) bool {
	if sourceName != "reddit" && sourceName != "twitchclips" {
		return false
	}
	sources, err := st.ListClusterEvidenceSources(ctx, clusterID)
	if err != nil {
		return false
	}
	hasReddit, hasClips := false, false
	for _, src := range sources {
		switch strings.ToLower(strings.TrimSpace(src)) {
		case "reddit":
			hasReddit = true
		case "twitchclips", "twitch_clip":
			hasClips = true
		}
	}
	if sourceName == "reddit" && hasClips {
		return true
	}
	if sourceName == "twitchclips" && hasReddit {
		return true
	}
	return false
}

func (s *Service) mergeWireMeta(ctx context.Context, clusterID int64, incomingTitle, incomingCategory, sourceName, flairText string) (string, string) {
	existingTitle, existingCategory, _ := s.store.ClusterMeta(ctx, clusterID)
	finalTitle := pickWireTitle(existingTitle, incomingTitle, sourceName)
	finalCategory := incomingCategory
	if sourceName == "streamerbans" {
		finalCategory = "bans"
	} else if cat := categoryFromFlair(flairText); cat != "" {
		finalCategory = cat
	} else if existingCategory != "" && (incomingCategory == "" || incomingCategory == "news") {
		finalCategory = existingCategory
	}
	return finalTitle, finalCategory
}

func pickWireTitle(existing, incoming, sourceName string) string {
	incoming = wireHeadlineCandidate(incoming, sourceName)
	existing = cleanWireTitle(existing)
	if isPlaceholderWireTitle(existing) && !isPlaceholderWireTitle(incoming) && incoming != "" {
		return incoming
	}
	if existing == "" {
		return incoming
	}
	if incoming == "" {
		return existing
	}
	if sourceName == "reddit" || sourceName == "streamerbans" {
		if isPlaceholderWireTitle(incoming) {
			return existing
		}
		return incoming
	}
	if sourceName == "twitchclips" {
		if len(existing) > len(incoming)+6 || strings.Count(existing, " ") > strings.Count(incoming, " ") {
			return existing
		}
	}
	if len(incoming) > len(existing)+10 {
		return incoming
	}
	return existing
}

func wireHeadlineCandidate(title, sourceName string) string {
	title = cleanWireTitle(title)
	if title == "" {
		return ""
	}
	if isPlaceholderWireTitle(title) {
		return ""
	}
	if sourceName == "twitchclips" || sourceName == "reddit" {
		if headline := evidenceHeadline(title); headline != "" && len(headline) >= 8 && !isPlaceholderWireTitle(headline) {
			return headline
		}
	}
	if sourceName == "twitchclips" {
		return title
	}
	if !titleQualityOK(title) {
		return ""
	}
	return title
}

func evidenceHeadline(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, "https://"); idx > 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "http://"); idx > 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	return cleanWireTitle(raw)
}

func titleQualityOK(title string) bool {
	title = cleanWireTitle(title)
	if title == "" {
		return false
	}
	if len(title) >= 12 {
		return true
	}
	return len(strings.Fields(title)) >= 3
}

func isPlaceholderWireTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return true
	}
	for _, re := range wirePlaceholderTitlePatterns {
		if re.MatchString(title) {
			return true
		}
	}
	return false
}

func cleanWireTitle(title string) string {
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if len(title) > 220 {
		return strings.TrimSpace(title[:220])
	}
	return title
}

func wireSummary(title string) string {
	title = cleanWireTitle(title)
	if title == "" {
		return ""
	}
	return "Wire-native social evidence grouped from global source ingest."
}

// ClassifyCategory maps title/flair hints to a feed filter category.
func ClassifyCategory(title, flairText string) string {
	return classifyCategory(title, flairText)
}

func wireCategory(sourceName, title, flairText string) string {
	if sourceName == "streamerbans" {
		return "bans"
	}
	return classifyCategory(title, flairText)
}

func classifyCategory(title, flairText string) string {
	if cat := categoryFromFlair(flairText); cat != "" {
		return cat
	}
	lower := strings.ToLower(title)
	switch {
	case titleSuggestsTwitchBan(lower):
		return "bans"
	case strings.Contains(lower, "record"), strings.Contains(lower, "peak"), strings.Contains(lower, "milestone"):
		return "records"
	case strings.Contains(lower, "tournament"), strings.Contains(lower, "major"), strings.Contains(lower, "esports"):
		return "esports"
	case strings.Contains(lower, "drama"), strings.Contains(lower, "beef"), strings.Contains(lower, "exposed"), strings.Contains(lower, "called out"):
		return "drama"
	case strings.Contains(lower, "funny"), strings.Contains(lower, "fails"):
		return "funny"
	default:
		return "news"
	}
}

func titleSuggestsTwitchBan(lowerTitle string) bool {
	lowerTitle = strings.TrimSpace(lowerTitle)
	if lowerTitle == "" {
		return false
	}
	for _, re := range notTwitchBanTitlePatterns {
		if re.MatchString(lowerTitle) {
			return false
		}
	}
	for _, re := range twitchBanTitlePatterns {
		if re.MatchString(lowerTitle) {
			return true
		}
	}
	return false
}

func categoryFromFlair(flair string) string {
	lower := strings.ToLower(strings.TrimSpace(flair))
	if strings.HasPrefix(lower, "wire-category:") {
		return strings.TrimPrefix(lower, "wire-category:")
	}
	switch {
	case strings.Contains(lower, "drama"), strings.Contains(lower, "controversy"):
		return "drama"
	case strings.Contains(lower, "funny"), strings.Contains(lower, "humor"), strings.Contains(lower, "meme"):
		return "funny"
	case strings.Contains(lower, "suspended"):
		return "bans"
	case strings.Contains(lower, "record"), strings.Contains(lower, "milestone"):
		return "records"
	case strings.Contains(lower, "esport"), strings.Contains(lower, "tournament"):
		return "esports"
	default:
		return ""
	}
}
