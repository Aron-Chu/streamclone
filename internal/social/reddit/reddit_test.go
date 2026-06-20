package reddit_test

import (
	"strings"
	"testing"

	"streamclone/internal/social/reddit"
)

func TestParseHTMLListing(t *testing.T) {
	posts := reddit.ParseHTMLListing(
		`<a href="/r/LivestreamFail/comments/abc123/post_slug/">Funny streamer clip</a>`,
		"https://www.reddit.com",
		"streamer",
	)
	if len(posts) != 1 || posts[0].Title == "" {
		t.Fatalf("expected one parsed post, got %+v", posts)
	}
}

func TestParseHTMLListingShredditMetrics(t *testing.T) {
	body := `<shreddit-post permalink="/r/LivestreamFail/comments/abc123/post_slug/" post-title="Streamer meltdown Announcement 0 votes • 79 comments" score="1200" comment-count="79" flair-text="Announcement"></shreddit-post>`
	posts := reddit.ParseHTMLListing(body, "https://www.reddit.com", "")
	if len(posts) != 1 {
		t.Fatalf("expected one post, got %+v", posts)
	}
	if posts[0].Score != 1200 || posts[0].Comments != 79 {
		t.Fatalf("expected parsed metrics, got score=%d comments=%d", posts[0].Score, posts[0].Comments)
	}
	if posts[0].Title != "Streamer meltdown" {
		t.Fatalf("expected cleaned title, got %q", posts[0].Title)
	}
}

func TestParseHTMLListingOldRedditThingPrefersTitleOverComments(t *testing.T) {
	body := `<div class="thing id-t3_abc123" data-subreddit="LivestreamFail">
		<p class="title"><a class="title" href="https://clips.twitch.tv/RepleteSuspiciousCattleCoolCat-beWJWU6zJ18moRSs">Mitch Jones gets cooked by the new AI summary</a></p>
		<a class="comments" href="/r/LivestreamFail/comments/abc123/mitch_jones_gets_cooked_by_the_new_ai_summary/">14 comments</a>
	</div></div>`
	posts := reddit.ParseHTMLListing(body, "https://old.reddit.com", "")
	if len(posts) != 1 {
		t.Fatalf("expected one post, got %+v", posts)
	}
	if posts[0].Title == "14 comments" {
		t.Fatalf("expected real headline, got %q", posts[0].Title)
	}
	if !strings.Contains(posts[0].Title, "Mitch Jones") {
		t.Fatalf("unexpected title %q", posts[0].Title)
	}
	if !strings.Contains(posts[0].URL, "clips.twitch.tv") {
		t.Fatalf("expected clip URL on post.URL, got %q", posts[0].URL)
	}
	if !strings.Contains(posts[0].Permalink, "/comments/abc123/") {
		t.Fatalf("expected permalink, got %q", posts[0].Permalink)
	}
	if posts[0].Comments != 14 {
		t.Fatalf("expected comment count 14, got %d", posts[0].Comments)
	}
}

func TestParseHTMLListingOldRedditSlugFallback(t *testing.T) {
	body := `<div class="thing id-t3_slug123" data-subreddit="LivestreamFail">
		<p class="title"><a class="title" href="https://clips.twitch.tv/abc">42 comments</a></p>
		<a class="comments" href="/r/livestreamfail/comments/slug123/streamer_does_something_wild/">42 comments</a>
	</div></div>`
	posts := reddit.ParseHTMLListing(body, "https://old.reddit.com", "")
	if len(posts) != 1 {
		t.Fatalf("expected one post, got %+v", posts)
	}
	if posts[0].Title != "streamer does something wild" {
		t.Fatalf("expected slug fallback title, got %q", posts[0].Title)
	}
}

func TestParseHTMLListingCaseInsensitiveSubreddit(t *testing.T) {
	body := `<a href="/r/livestreamfail/comments/xyz789/cool_clip_title/">Streamer wins tournament</a>`
	posts := reddit.ParseHTMLListing(body, "https://www.reddit.com", "")
	if len(posts) != 1 || posts[0].ID != "xyz789" {
		t.Fatalf("expected parsed post for lowercase subreddit, got %+v", posts)
	}
}

func TestParseHTMLListingDedupesThingAndAnchor(t *testing.T) {
	body := `<div class="thing id-t3_dup1" data-subreddit="LivestreamFail">
		<p class="title"><a class="title" href="https://clips.twitch.tv/DupClip">Short title</a></p>
		<a class="comments" href="/r/LivestreamFail/comments/dup1/streamer_long_form_headline/">9 comments</a>
	</div></div>
	<a href="/r/LivestreamFail/comments/dup1/streamer_long_form_headline/">9 comments</a>`
	posts := reddit.ParseHTMLListing(body, "https://old.reddit.com", "")
	if len(posts) != 1 {
		t.Fatalf("expected deduped single post, got %+v", posts)
	}
	if posts[0].Title != "Short title" {
		t.Fatalf("expected richer thing title, got %q", posts[0].Title)
	}
}

func TestParseHTMLListingOldRedditThingScore(t *testing.T) {
	body := `<div class="thing id-t3_score1" data-score="1842" data-subreddit="LivestreamFail">
		<p class="title"><a class="title" href="https://clips.twitch.tv/ScoreClip">Streamer loses it on stream</a></p>
		<div class="score unvoted" title="1842 points">1842</div>
		<a class="comments" href="/r/LivestreamFail/comments/score1/streamer_loses_it_on_stream/">88 comments</a>
	</div></div>`
	posts := reddit.ParseHTMLListing(body, "https://old.reddit.com", "")
	if len(posts) != 1 {
		t.Fatalf("expected one post, got %+v", posts)
	}
	if posts[0].Score != 1842 {
		t.Fatalf("expected score 1842, got %d", posts[0].Score)
	}
	if posts[0].Comments != 88 {
		t.Fatalf("expected 88 comments, got %d", posts[0].Comments)
	}
}

func TestParseHTMLComments(t *testing.T) {
	body := `<div class="comment"><div class="md"><p>Check https://clips.twitch.tv/FunnySlug</p></div></div>
	<div class="comment"><div class="md"><p>Second comment with link</p></div></div>`
	comments := reddit.ParseHTMLComments(body)
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %+v", comments)
	}
	if !strings.Contains(comments[0], "clips.twitch.tv") {
		t.Fatalf("expected link preserved in comment, got %q", comments[0])
	}
}
