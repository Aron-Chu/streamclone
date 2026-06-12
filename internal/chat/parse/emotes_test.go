package parse

import "testing"

func TestParseEmotesTagSingle(t *testing.T) {
	ranges := ParseEmotesTag("25:0-4")
	if len(ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(ranges))
	}
	if ranges[0].ID != "25" || ranges[0].Start != 0 || ranges[0].End != 4 {
		t.Fatalf("unexpected range: %+v", ranges[0])
	}
}

func TestParseEmotesTagMultipleSameID(t *testing.T) {
	ranges := ParseEmotesTag("25:0-4,12-16")
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d (%+v)", len(ranges), ranges)
	}
	if ranges[0].Start != 0 || ranges[0].End != 4 {
		t.Fatalf("first range = %+v", ranges[0])
	}
	if ranges[1].Start != 12 || ranges[1].End != 16 {
		t.Fatalf("second range = %+v", ranges[1])
	}
}

func TestParseEmotesTagMultipleIDs(t *testing.T) {
	ranges := ParseEmotesTag("25:0-4,12-16/1902:6-10")
	if len(ranges) != 3 {
		t.Fatalf("expected 3 ranges, got %d (%+v)", len(ranges), ranges)
	}
	if ranges[0].ID != "25" || ranges[0].Start != 0 {
		t.Fatalf("first range = %+v", ranges[0])
	}
	// Output is sorted by start position so the enricher can walk left to right.
	if ranges[1].ID != "1902" || ranges[1].Start != 6 || ranges[1].End != 10 {
		t.Fatalf("second range = %+v", ranges[1])
	}
	if ranges[2].ID != "25" || ranges[2].Start != 12 {
		t.Fatalf("third range = %+v", ranges[2])
	}
}

func TestParseEmotesTagEmpty(t *testing.T) {
	if ParseEmotesTag("") != nil {
		t.Fatal("expected nil for empty tag")
	}
	if ParseEmotesTag("   ") != nil {
		t.Fatal("expected nil for whitespace tag")
	}
}

func TestParseEmotesTagSkipsInvalid(t *testing.T) {
	ranges := ParseEmotesTag("25:bad/1902:6-10")
	if len(ranges) != 1 || ranges[0].ID != "1902" {
		t.Fatalf("expected one valid range, got %+v", ranges)
	}
}

func TestPrivmsgParsesEmotesTag(t *testing.T) {
	line := `@badge-info=;badges=;color=#FF0000;display-name=Viewer;emotes=25:0-4;id=abc;tmi-sent-ts=1730000000000 :viewer!viewer@viewer.tmi.twitch.tv PRIVMSG #channel :Kappa`
	msg, ok := ParseLine(line)
	if !ok {
		t.Fatal("expected ok")
	}
	if len(msg.Emotes) != 1 || msg.Emotes[0].ID != "25" {
		t.Fatalf("emotes = %+v", msg.Emotes)
	}
	if msg.Text != "Kappa" {
		t.Fatalf("text = %q", msg.Text)
	}
}
