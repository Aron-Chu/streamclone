package analytics

import (
	"context"
	"errors"
	"testing"
)

func TestValidatePulseVodIDRejectsBroadcastID(t *testing.T) {
	stream := StreamRecord{StreamID: "123456789", CanonicalStreamID: "987654321"}
	if _, err := validatePulseVODCandidate(stream, "987654321"); !errors.Is(err, ErrPulseInvalidVODID) {
		t.Fatalf("err = %v, want ErrPulseInvalidVODID", err)
	}
}

func TestValidatePulseVodIDRejectsStreamID(t *testing.T) {
	stream := StreamRecord{StreamID: "123456789"}
	if _, err := validatePulseVODCandidate(stream, "123456789"); !errors.Is(err, ErrPulseInvalidVODID) {
		t.Fatalf("err = %v, want ErrPulseInvalidVODID", err)
	}
}

func TestValidatePulseVodIDRejectsInvalidFormat(t *testing.T) {
	stream := StreamRecord{StreamID: "123456789"}
	for _, vodID := range []string{"", "abc123", "12345", "123456789012345678901"} {
		if _, err := validatePulseVODCandidate(stream, vodID); !errors.Is(err, ErrPulseInvalidVODID) {
			t.Fatalf("vodID %q err = %v, want ErrPulseInvalidVODID", vodID, err)
		}
	}
}

func TestValidatePulseVodIDRequiresHelixForCanonicalHint(t *testing.T) {
	stream := StreamRecord{StreamID: "123456789", Login: "channel", BroadcasterID: "42"}
	if _, err := validatePulseVodViaHelix(context.Background(), nil, stream, "channel", "1234567890", true); !errors.Is(err, ErrPulseVODValidationUnavailable) {
		t.Fatalf("err = %v, want ErrPulseVODValidationUnavailable", err)
	}
}
