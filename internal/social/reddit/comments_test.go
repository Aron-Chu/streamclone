package reddit

import (
	"reflect"
	"testing"
)

func TestDecodeCommentBodiesFiltersRemovedAndKeepsLinks(t *testing.T) {
	body := []byte(`[
		{"kind":"Listing","data":{"children":[]}},
		{"kind":"Listing","data":{"children":[
			{"kind":"t1","data":{"body":"watch this https://youtu.be/abc123"}},
			{"kind":"t1","data":{"body":"[deleted]"}},
			{"kind":"t1","data":{"body":"  "}},
			{"kind":"t1","data":{"body":"clip https://clips.twitch.tv/FunnySlug"}},
			{"kind":"t1","data":{"body":"[removed]"}}
		]}}
	]`)

	got, err := decodeCommentBodies(body)
	if err != nil {
		t.Fatalf("decodeCommentBodies returned error: %v", err)
	}
	want := []string{
		"watch this https://youtu.be/abc123",
		"clip https://clips.twitch.tv/FunnySlug",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeCommentBodies() = %#v, want %#v", got, want)
	}
}
