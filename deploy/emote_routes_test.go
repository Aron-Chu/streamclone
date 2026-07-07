package deploy_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestEmoteAssetRoutesProxyToEmoteService(t *testing.T) {
	files := map[string][]string{
		"Caddyfile":      {"Caddyfile", "Caddyfile.local-tunnel", "Caddyfile.pulse-api"},
		"nginx.conf":     {"../frontend/nginx.conf"},
		"vite.config.ts": {"../frontend/vite.config.ts"},
	}

	for kind, paths := range files {
		for _, rel := range paths {
			data, err := os.ReadFile(rel)
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			text := string(data)
			switch kind {
			case "Caddyfile":
				if !strings.Contains(text, "@emote_assets") {
					t.Fatalf("%s missing @emote_assets matcher", rel)
				}
				block := extractCaddyBlock(text, "@emote_assets")
				if strings.Contains(block, "minio:9000") {
					t.Fatalf("%s routes /emotes/* to minio; want emote:8080", rel)
				}
				if !strings.Contains(block, "emote:8080") {
					t.Fatalf("%s @emote_assets must reverse_proxy to emote:8080", rel)
				}
			case "nginx.conf":
				block := extractBetween(text, "location /emotes/", "}")
				if strings.Contains(block, "minio:9000") {
					t.Fatalf("%s routes /emotes/ to minio; want emote:8080", rel)
				}
				if !strings.Contains(block, "emote:8080") {
					t.Fatalf("%s /emotes/ must proxy to emote:8080", rel)
				}
			case "vite.config.ts":
				block := extractBetween(text, "'/emotes'", "},")
				if strings.Contains(block, "9000") {
					t.Fatalf("%s vite /emotes proxy targets minio port 9000; want emote 8084", rel)
				}
				if !strings.Contains(block, "8084") {
					t.Fatalf("%s vite /emotes proxy must target emote service port 8084", rel)
				}
			}
		}
	}
}

func extractCaddyBlock(text, matcher string) string {
	idx := strings.Index(text, matcher)
	if idx < 0 {
		return ""
	}
	sub := text[idx:]
	end := strings.Index(sub, "reverse_proxy")
	if end < 0 {
		return sub
	}
	lineEnd := strings.Index(sub[end:], "\n")
	if lineEnd < 0 {
		return sub[end:]
	}
	return sub[end : end+lineEnd]
}

func extractBetween(text, start, end string) string {
	idx := strings.Index(text, start)
	if idx < 0 {
		return ""
	}
	sub := text[idx:]
	stop := strings.Index(sub, end)
	if stop < 0 {
		return sub
	}
	return sub[:stop]
}

var terminalJobRequeueSQL = regexp.MustCompile(`state IN \(3, 4\)`)

func TestInsertOrRequeueJobResetsTerminalStates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "internal", "emote", "store", "store.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !terminalJobRequeueSQL.Match(data) {
		t.Fatal("InsertOrRequeueJob must reset terminal job states 3/4 back to queued")
	}
}
