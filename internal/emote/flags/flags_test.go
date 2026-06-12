package flags

import "testing"

func TestFromSevenTVZeroWidth(t *testing.T) {
	tests := []struct {
		name     string
		item     int
		data     int
		wantZero bool
	}{
		{name: "data flag 256 RainTime style", item: 1, data: 256, wantZero: true},
		{name: "legacy bit 1 only", item: 1, data: 0, wantZero: false},
		{name: "regular emote", item: 0, data: 0, wantZero: false},
		{name: "set item flag 256", item: 256, data: 0, wantZero: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FromSevenTV(tt.item, tt.data); got != tt.wantZero {
				t.Fatalf("FromSevenTV(%d, %d) = %v want %v", tt.item, tt.data, got, tt.wantZero)
			}
		})
	}
}

func TestPackAndIsZeroWidth(t *testing.T) {
	f := Pack(true, true)
	if !IsZeroWidth(f) {
		t.Fatalf("expected zero width flag")
	}
	if f&Animated == 0 {
		t.Fatalf("expected animated flag")
	}
}
