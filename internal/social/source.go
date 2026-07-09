package social

import (
	"context"
	"time"

	"streamclone/internal/social/reliability"
)

// Provenance records bounded fetch metadata (raw-data minimization).
type Provenance struct {
	FetchedAt  time.Time `json:"fetchedAt"`
	SourceAPI  string    `json:"sourceApi"`
	RequestID  string    `json:"requestId,omitempty"`
	HTTPStatus int       `json:"httpStatus,omitempty"`
}

// MediaRef points at media URLs for the matcher (embeds only where required).
type MediaRef struct {
	Kind string `json:"kind"` // image|video|audio
	URL  string `json:"url"`
}

// Item is a normalized social post/video/comment.
type Item struct {
	Source, Kind, ExternalID, URL, Author, Text string
	FlairText                                   string // Reddit link flair when available
	CreatedAt                                   time.Time
	Metrics                                     map[string]float64
	Media                                       []MediaRef
	EntityTwitchLogin                           string
	EntityDisplayName                           string
	EntityAliases                               []Alias
	Provenance                                  Provenance
	SnapshotSHA256                              []byte
	ExpiresAt                                   time.Time
}

// Budget caps per-call cost for a source.
type Budget struct {
	MaxItems          int
	MaxCost           float64
	MaxBrowserFetches int
}

// Query drives paginated discovery.
type Query struct {
	Entity   EntityRef
	Since    time.Time
	Keywords []string
	Budget   Budget
	Cursor   string
}

// EntityRef is a lightweight entity handle for queries.
type EntityRef struct {
	TwitchLogin string
	TwitchID    string
	Aliases     []Alias
}

// Alias is a verified cross-platform handle.
type Alias struct {
	Platform string `json:"platform"`
	Handle   string `json:"handle"`
}

// Page is a paginated result set.
type Page struct {
	Items      []Item
	NextCursor string
}

// Caps advertises optional capabilities for compliance gating.
type Caps struct {
	Hydrate          bool
	Comments         bool
	RefreshMetrics   bool
	Backfill         bool
	RealtimeFirehose bool
}

// SocialSource is the core contract every platform adapter must satisfy.
type SocialSource interface {
	Name() string
	Risk() reliability.Risk
	Capabilities() Caps
	Search(ctx context.Context, q Query) (Page, error)
	Healthy(ctx context.Context) error
}

// Hydrator fetches full items by id.
type Hydrator interface {
	Hydrate(ctx context.Context, ids []string) ([]Item, error)
}

// CommentFetcher hydrates thread comments.
type CommentFetcher interface {
	Comments(ctx context.Context, itemID string, q Query) (Page, error)
}

// MetricRefresher re-polls engagement metrics for trend deltas.
type MetricRefresher interface {
	RefreshMetrics(ctx context.Context, ids []string) (map[string]map[string]float64, error)
}

// Backfiller performs historical sweeps.
type Backfiller interface {
	Backfill(ctx context.Context, q Query) (Page, error)
}

type ctor func() (SocialSource, error)

var registry = map[string]ctor{}

// Register adds a SocialSource constructor to the global registry.
func Register(name string, fn ctor) {
	registry[name] = fn
}

// Construct builds a registered source by name.
func Construct(name string) (SocialSource, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, ErrUnknownSource
	}
	return fn()
}

// Registered returns all registered source names.
func Registered() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	return out
}

// ErrUnknownSource is returned when a source name is not registered.
var ErrUnknownSource = errUnknown("unknown social source")

type errUnknown string

func (e errUnknown) Error() string { return string(e) }
