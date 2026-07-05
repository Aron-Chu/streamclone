package analytics

import "testing"

func TestDetectStreamTogetherFromTitle(t *testing.T) {
	info := detectStreamTogether("Streaming together with cucurucho!", nil)
	if !info.Together {
		t.Fatal("expected together from title")
	}
	if info.PartnerHint != "cucurucho" {
		t.Fatalf("partner = %q, want cucurucho", info.PartnerHint)
	}
}

func TestDetectStreamTogetherFromTag(t *testing.T) {
	info := detectStreamTogether("", []string{"StreamingTogether", "English"})
	if !info.Together {
		t.Fatal("expected together from tag")
	}
}

func TestCategoryForTogetherStreamUsesHostCategory(t *testing.T) {
	roster := map[string]Top500Current{
		"hostchan": {
			Login:        "hostchan",
			IsLive:       true,
			CategoryName: "Just Chatting",
		},
		"guestchan": {
			Login:        "guestchan",
			IsLive:       true,
			CategoryName: "",
			Title:        "Streaming together with hostchan",
		},
	}
	cat := categoryForTogetherStream(
		"guestchan",
		"",
		"Streaming together with hostchan",
		nil,
		roster,
	)
	if cat != "Just Chatting" {
		t.Fatalf("category = %q, want Just Chatting", cat)
	}
}
