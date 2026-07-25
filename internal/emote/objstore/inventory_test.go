package objstore

import "testing"

func TestManifestObjectKind(t *testing.T) {
	tests := map[string]string{
		"prefix/id/src":     "source",
		"prefix/id/1x.webp": "render",
		"prefix/id/2X.WEBP": "render",
		"prefix/other.json": "other",
	}
	for key, want := range tests {
		if got := manifestObjectKind(key); got != want {
			t.Errorf("manifestObjectKind(%q) = %q, want %q", key, got, want)
		}
	}
}
