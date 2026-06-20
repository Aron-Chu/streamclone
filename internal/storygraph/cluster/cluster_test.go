package cluster

import "testing"

func TestWireCategoryStreamerBans(t *testing.T) {
	if got := wireCategory("streamerbans", "short", ""); got != "bans" {
		t.Fatalf("streamerbans source should force bans, got %q", got)
	}
}

func TestClassifyCategory(t *testing.T) {
	cases := map[string]string{
		"Streamer ban appeal clip goes viral":                                                                 "news",
		"Twitch Partner \"HasanAbi\" has been banned":                                                         "bans",
		"xQc leaks his 2nd steam account & almost gets his IP taken while checking if hes banned in Forza H6": "news",
		"Funny fail clip spreads across Shorts":                                                               "funny",
		"Esports major finals reaction hits record":                                                           "records",
		"Streamer drama thread keeps escalating":                                                              "drama",
		"Championship esports roster reveal reaction":                                                         "esports",
		"General streaming news update":                                                                       "news",
	}
	for title, want := range cases {
		if got := classifyCategory(title, ""); got != want {
			t.Fatalf("classifyCategory(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestTitleSuggestsTwitchBan(t *testing.T) {
	if titleSuggestsTwitchBan("xqc checking if hes banned in forza h6") {
		t.Fatal("game ban check should not classify as twitch ban")
	}
	if titleSuggestsTwitchBan("xQc leaks his 2nd steam account & almost gets his IP taken while checking if hes banned in Forza H6") {
		t.Fatal("xQc Forza/Steam false-ban clip should not classify as twitch ban")
	}
	if titleSuggestsTwitchBan("pro player banned in valorant after ranked match") {
		t.Fatal("game/platform-specific ban should not classify as twitch ban")
	}
	if !titleSuggestsTwitchBan(`twitch partner "xqc" has been banned`) {
		t.Fatal("streamerbans-style headline should classify as twitch ban")
	}
	if !titleSuggestsTwitchBan(`Twitch Partner "forsen" has been banned!`) {
		t.Fatal("real StreamerBans headline should classify as twitch ban")
	}
}

func TestClassifyCategoryFromFlair(t *testing.T) {
	if got := classifyCategory("Some post", "Drama"); got != "drama" {
		t.Fatalf("expected drama from flair, got %q", got)
	}
}

func TestTitleQualityOK(t *testing.T) {
	if titleQualityOK("YOINK") {
		t.Fatal("short clip slug should fail quality gate")
	}
	if !titleQualityOK("CaseOh banned after marathon") {
		t.Fatal("long title should pass")
	}
	if !titleQualityOK("one two three") {
		t.Fatal("three words should pass")
	}
}

func TestWireHeadlineCandidateAllowsShortClip(t *testing.T) {
	if got := wireHeadlineCandidate("YOINK", "twitchclips"); got != "YOINK" {
		t.Fatalf("short clip slug should pass for twitchclips: %q", got)
	}
	if got := wireHeadlineCandidate("CaseOh reacts to chat ban appeal", "twitchclips"); got == "" {
		t.Fatal("descriptive clip title should pass")
	}
}

func TestEvidenceHeadlineStripsURL(t *testing.T) {
	got := evidenceHeadline("Big drama thread https://reddit.com/r/lsf/comments/abc")
	if got != "Big drama thread" {
		t.Fatalf("unexpected evidence headline %q", got)
	}
}

func TestPickWireTitleKeepsRedditHeadline(t *testing.T) {
	existing := "CaseOh banned from Twitch after marathon stream drama"
	incoming := "YOINK"
	if got := pickWireTitle(existing, incoming, "twitchclips"); got != existing {
		t.Fatalf("clip title overwrote reddit headline: %q", got)
	}
}

func TestPickWireTitleRedditWins(t *testing.T) {
	existing := "short clip"
	incoming := "Full Reddit thread title about the incident"
	if got := pickWireTitle(existing, incoming, "reddit"); got != incoming {
		t.Fatalf("reddit title should win: %q", got)
	}
}

func TestPickWireTitleRejectsPlaceholderIncoming(t *testing.T) {
	existing := "Mitch Jones gets cooked by the new AI summary"
	incoming := "17 comments"
	if got := pickWireTitle(existing, incoming, "reddit"); got != existing {
		t.Fatalf("placeholder incoming must not replace good title: %q", got)
	}
	if got := pickWireTitle("14 comments", incoming, "reddit"); got != "14 comments" {
		t.Fatalf("both placeholders should keep existing: %q", got)
	}
}

func TestWireHeadlineCandidateRejectsCommentCount(t *testing.T) {
	if got := wireHeadlineCandidate("17 comments", "reddit"); got != "" {
		t.Fatalf("comment-count placeholder should be rejected: %q", got)
	}
}

func TestCleanWireTitleTrimsAndBounds(t *testing.T) {
	got := cleanWireTitle("  CaseOh   reacts   to chat   ")
	if got != "CaseOh reacts to chat" {
		t.Fatalf("unexpected cleaned title %q", got)
	}
}
