package reliability

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Risk classifies how a source was obtained.
type Risk string

const (
	RiskOfficial   Risk = "official"
	RiskPublicAPI  Risk = "public_api"
	RiskScraper    Risk = "scraper"
	RiskUnofficial Risk = "unofficial"
)

// Entry is a registry row for a source type.
type Entry struct {
	SourceType       string  `json:"sourceType"`
	SourceRisk       Risk    `json:"sourceRisk"`
	ConfidenceWeight float64 `json:"confidenceWeight"`
}

// Registry resolves reliability weights for evidence scoring.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Entry
	pool    *pgxpool.Pool
}

// NewRegistry creates an empty registry (seeded from DB on Load).
func NewRegistry(pool *pgxpool.Pool) *Registry {
	return &Registry{
		entries: make(map[string]Entry),
		pool:    pool,
	}
}

// Load reads source_reliability from Postgres.
func (r *Registry) Load(ctx context.Context) error {
	if r.pool == nil {
		return nil
	}
	rows, err := r.pool.Query(ctx, `SELECT source_type, source_risk, confidence_weight FROM source_reliability`)
	if err != nil {
		return err
	}
	defer rows.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	for rows.Next() {
		var e Entry
		var risk string
		if err := rows.Scan(&e.SourceType, &risk, &e.ConfidenceWeight); err != nil {
			return err
		}
		e.SourceRisk = Risk(risk)
		r.entries[e.SourceType] = e
	}
	return rows.Err()
}

// Override applies config overrides without a deploy.
func (r *Registry) Override(sourceType string, weight float64, risk Risk) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[sourceType]
	if !ok {
		e = Entry{SourceType: sourceType}
	}
	if weight > 0 {
		e.ConfidenceWeight = weight
	}
	if risk != "" {
		e.SourceRisk = risk
	}
	r.entries[sourceType] = e
}

// Get returns the entry for a source type.
func (r *Registry) Get(sourceType string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[sourceType]
	return e, ok
}

// Weight returns the confidence weight, defaulting to 0.5 when unknown.
func (r *Registry) Weight(sourceType string) float64 {
	e, ok := r.Get(sourceType)
	if !ok || e.ConfidenceWeight <= 0 {
		return 0.5
	}
	return e.ConfidenceWeight
}

// All returns all entries.
func (r *Registry) All() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

// DefaultEntries returns seed weights when DB is unavailable.
func DefaultEntries() []Entry {
	return []Entry{
		{SourceType: "pulse_origin", SourceRisk: RiskOfficial, ConfidenceWeight: 1.00},
		{SourceType: "twitch_clip", SourceRisk: RiskOfficial, ConfidenceWeight: 0.95},
		{SourceType: "news_article", SourceRisk: RiskPublicAPI, ConfidenceWeight: 0.75},
		{SourceType: "reddit_thread", SourceRisk: RiskPublicAPI, ConfidenceWeight: 0.70},
		{SourceType: "youtube_video", SourceRisk: RiskPublicAPI, ConfidenceWeight: 0.70},
		{SourceType: "x_post", SourceRisk: RiskPublicAPI, ConfidenceWeight: 0.60},
		{SourceType: "tiktok_video", SourceRisk: RiskPublicAPI, ConfidenceWeight: 0.60},
		{SourceType: "instagram_post", SourceRisk: RiskPublicAPI, ConfidenceWeight: 0.55},
		{SourceType: "kick_clip", SourceRisk: RiskPublicAPI, ConfidenceWeight: 0.55},
		{SourceType: "streamerbans_post", SourceRisk: RiskUnofficial, ConfidenceWeight: 0.72},
		{SourceType: "manual_curation", SourceRisk: RiskUnofficial, ConfidenceWeight: 0.40},
	}
}

// SeedDefaults populates the in-memory map from defaults.
func (r *Registry) SeedDefaults() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range DefaultEntries() {
		r.entries[e.SourceType] = e
	}
}

// ValidateWeight clamps weight to [0.2, 1.0].
func ValidateWeight(w float64) float64 {
	if w < 0.2 {
		return 0.2
	}
	if w > 1.0 {
		return 1.0
	}
	return w
}

// FormatRisk returns a human-readable risk label.
func FormatRisk(r Risk) string {
	switch r {
	case RiskOfficial:
		return "official"
	case RiskPublicAPI:
		return "public_api"
	case RiskScraper:
		return "scraper"
	case RiskUnofficial:
		return "unofficial"
	default:
		return fmt.Sprintf("%s", r)
	}
}
