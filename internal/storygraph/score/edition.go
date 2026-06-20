package score



import (

	"strings"



	"streamclone/internal/storygraph/store"

)



// EditionMode describes the product mode for a window.

func EditionMode(window string) string {

	switch strings.ToLower(strings.TrimSpace(window)) {

	case "today":

		return "breaking"

	case "7d":

		return "weekly"

	default:

		return "daily"

	}

}



// EditionSection is one curated block in a window edition.

type EditionSection struct {

	ID       string                 `json:"id"`

	Title    string                 `json:"title"`

	Subtitle string                 `json:"subtitle,omitempty"`

	Stories  []store.StoryCard      `json:"stories,omitempty"`

	Movers   []store.RisingStreamer `json:"movers,omitempty"`

}



// WireEdition is the window-aware Pulse Wire edition payload.

type WireEdition struct {

	Window           string                 `json:"window"`

	Since            any                    `json:"since"`

	RankModel        string                 `json:"rankModel"`

	Mode             string                 `json:"mode"`

	TotalLive        *int                   `json:"totalLive,omitempty"`

	TotalViewers     *int                   `json:"totalViewers,omitempty"`

	SampleStatus     map[string]any         `json:"sampleStatus,omitempty"`

	Sections         []EditionSection       `json:"sections,omitempty"`

	Breaking         []store.StoryCard      `json:"breaking,omitempty"`

	TopCorroborated  []store.StoryCard      `json:"topCorroborated,omitempty"`

	BiggestMover     *store.RisingStreamer  `json:"biggestMover,omitempty"`

	Bans             []store.StoryCard      `json:"bans,omitempty"`

	BansOfTheDay     []store.StoryCard      `json:"bansOfTheDay,omitempty"`

	FastestSpreading []store.StoryCard      `json:"fastestSpreading,omitempty"`

	WeeklyRecap      []store.StoryCard      `json:"weeklyRecap,omitempty"`

	NewEntrants      []store.RisingStreamer `json:"newEntrants,omitempty"`

	TopGainers       []store.RisingStreamer `json:"topGainers,omitempty"`

}



// FilterCorroborated keeps stories with at least two distinct source types in-window.

func FilterCorroborated(cards []store.StoryCard) []store.StoryCard {

	out := make([]store.StoryCard, 0, len(cards))

	for _, card := range cards {

		if card.WindowScores != nil && card.WindowScores.SourceCount >= 2 {

			out = append(out, card)

		}

	}

	return out

}



// BuildEditionSections maps edition fields into UI sections for the active window.

func BuildEditionSections(ed WireEdition) []EditionSection {

	window := strings.ToLower(strings.TrimSpace(ed.Window))

	gainers := ed.TopGainers

	entrants := ed.NewEntrants

	switch window {

	case "today":

		sections := []EditionSection{

			{

				ID:       "breaking",

				Title:    "Breaking now",

				Subtitle: "Fresh evidence and velocity",

				Stories:  trimStories(ed.Breaking, 3),

			},

		}

		if ed.BiggestMover != nil {

			sections = append(sections, EditionSection{

				ID:     "biggest_gainer",

				Title:  "Biggest gainer",

				Movers: []store.RisingStreamer{*ed.BiggestMover},

			})

		} else if len(gainers) > 0 {

			sections = append(sections, EditionSection{

				ID:     "biggest_gainer",

				Title:  "Biggest gainer",

				Movers: gainers[:1],

			})

		}

		if len(entrants) > 0 {

			sections = append(sections, EditionSection{

				ID:     "new_entrants",

				Title:  "New entrants",

				Movers: trimMovers(entrants, 3),

			})

		}

		return sections

	case "7d":

		return []EditionSection{

			{ID: "weekly_risers", Title: "Weekly risers", Movers: trimMovers(gainers, 5)},

			{

				ID:       "most_covered",

				Title:    "Most covered",

				Subtitle: "Multi-source corroboration",

				Stories:  trimStories(firstNonEmpty(ed.TopCorroborated, ed.Breaking), 3),

			},

			{ID: "resolved", Title: "Settled stories", Stories: trimStories(ed.WeeklyRecap, 3)},

		}

	default:

		return []EditionSection{

			{

				ID:       "top_corroborated",

				Title:    "Top corroborated",

				Subtitle: "Two or more source types",

				Stories:  trimStories(ed.TopCorroborated, 3),

			},

			{ID: "fastest_spreading", Title: "Fastest spreading", Stories: trimStories(ed.FastestSpreading, 3)},

			{ID: "bans", Title: "Bans & moderation", Stories: trimStories(ed.Bans, 3)},

		}

	}

}



func trimStories(cards []store.StoryCard, limit int) []store.StoryCard {

	if limit <= 0 || len(cards) == 0 {

		return nil

	}

	if len(cards) <= limit {

		return cards

	}

	return cards[:limit]

}



func trimMovers(rows []store.RisingStreamer, limit int) []store.RisingStreamer {

	if limit <= 0 || len(rows) == 0 {

		return nil

	}

	if len(rows) <= limit {

		return rows

	}

	return rows[:limit]

}



func firstNonEmpty(groups ...[]store.StoryCard) []store.StoryCard {

	for _, g := range groups {

		if len(g) > 0 {

			return g

		}

	}

	return nil

}
