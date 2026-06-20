package matcher

import (
	"math"
	"strings"
	"unicode"

	"streamclone/internal/storygraph/fingerprint"
)

// Config holds match thresholds.
type Config struct {
	LinkThreshold   float64
	ReviewThreshold float64
}

// Input pairs a moment fingerprint with an external item caption.
type Input struct {
	Quotes      []string
	TopEmotes   []string
	ItemText    string
	EntityMatch bool
	TimingScore float64 // 0..1 proximity
}

// Result is same_story_confidence with explainability.
type Result struct {
	Confidence float64            `json:"confidence"`
	Factors    map[string]float64 `json:"factors"`
	Decision   string             `json:"decision"` // link|review|discard
}

// LexicalMatcher scores text/emote/timing overlap (Phase 1).
type LexicalMatcher struct {
	cfg Config
}

func NewLexical(cfg Config) *LexicalMatcher {
	if cfg.LinkThreshold <= 0 {
		cfg.LinkThreshold = 0.65
	}
	if cfg.ReviewThreshold <= 0 {
		cfg.ReviewThreshold = 0.40
	}
	return &LexicalMatcher{cfg: cfg}
}

// Score computes same_story_confidence for Phase 1 (no visual/audio).
func (m *LexicalMatcher) Score(in Input) Result {
	quoteSim := quoteSimilarity(in.Quotes, in.ItemText)
	emoteOverlap := emoteKeywordOverlap(in.TopEmotes, in.ItemText)
	entity := 0.0
	if in.EntityMatch {
		entity = 1.0
	}
	timing := clamp01(in.TimingScore)

	conf := 0.30*quoteSim + 0.15*timing + 0.10*entity + 0.10*emoteOverlap
	// Phase 1: redistribute visual/audio weights to quote/timing
	conf = clamp01(conf)

	factors := map[string]float64{
		"quote_similarity": quoteSim,
		"timing_proximity": timing,
		"entity_match":     entity,
		"emote_overlap":    emoteOverlap,
	}
	decision := "discard"
	if conf >= m.cfg.LinkThreshold {
		decision = "link"
	} else if conf >= m.cfg.ReviewThreshold {
		decision = "review"
	}
	return Result{Confidence: conf, Factors: factors, Decision: decision}
}

// BuildInputFromFingerprint maps store fingerprint to matcher input.
func BuildInputFromFingerprint(fp fingerprint.MomentFingerprint, itemText string, entityMatch bool, timing float64) Input {
	emotes := make([]string, 0, len(fp.TopEmotes))
	for _, e := range fp.TopEmotes {
		emotes = append(emotes, e.Name)
	}
	return Input{
		Quotes:      fp.Quotes,
		TopEmotes:   emotes,
		ItemText:    itemText,
		EntityMatch: entityMatch,
		TimingScore: timing,
	}
}

func quoteSimilarity(quotes []string, text string) float64 {
	text = normalize(text)
	if text == "" {
		return 0
	}
	best := 0.0
	for _, q := range quotes {
		s := TitleSimilarity(q, text)
		if s > best {
			best = s
		}
	}
	return best
}

// TitleSimilarity exposes the matcher trigram score for story fusion.
func TitleSimilarity(a, b string) float64 {
	a = normalize(a)
	b = normalize(b)
	if a == "" || b == "" {
		return 0
	}
	score := trigramJaccard(a, b)
	if strings.Contains(a, b) || strings.Contains(b, a) {
		score = math.Max(score, 0.85)
	}
	return score
}

func emoteKeywordOverlap(emotes []string, text string) float64 {
	text = normalize(text)
	if text == "" || len(emotes) == 0 {
		return 0
	}
	hits := 0
	for _, e := range emotes {
		if e != "" && strings.Contains(text, normalize(e)) {
			hits++
		}
	}
	return float64(hits) / float64(len(emotes))
}

func trigramJaccard(a, b string) float64 {
	ga := trigrams(a)
	gb := trigrams(b)
	if len(ga) == 0 || len(gb) == 0 {
		return 0
	}
	inter := 0
	for g := range ga {
		if gb[g] {
			inter++
		}
	}
	union := len(ga) + len(gb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func trigrams(s string) map[string]bool {
	s = "  " + s + "  "
	out := map[string]bool{}
	for i := 0; i+3 <= len(s); i++ {
		out[s[i:i+3]] = true
	}
	return out
}

func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
