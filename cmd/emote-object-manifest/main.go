// Command emote-object-manifest emits a private JSONL inventory of the
// configured primary emote bucket. It is read-only and never prints
// credentials. Use --sha256 for the slower full-byte verification pass.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"streamclone/internal/config"
	"streamclone/internal/emote/objstore"
)

func main() {
	includeSHA256 := flag.Bool("sha256", false, "download each object and include its SHA-256")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		exitf("load config: %v", err)
	}
	if strings.TrimSpace(cfg.S3Endpoint) == "" ||
		strings.TrimSpace(cfg.S3Bucket) == "" ||
		strings.TrimSpace(cfg.S3AccessKey) == "" ||
		strings.TrimSpace(cfg.S3SecretKey) == "" {
		exitf("S3 endpoint, bucket, access key, and secret key are required")
	}

	useSSL := strings.HasPrefix(strings.TrimSpace(cfg.S3Endpoint), "https://")
	endpoint := strings.TrimSpace(cfg.S3Endpoint)
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	client, err := objstore.New(
		endpoint,
		cfg.S3AccessKey,
		cfg.S3SecretKey,
		cfg.S3Bucket,
		cfg.S3Prefix,
		useSSL,
	)
	if err != nil {
		exitf("initialize object store: %v", err)
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := client.Inventory(context.Background(), *includeSHA256, func(entry objstore.ManifestEntry) error {
		return encoder.Encode(entry)
	}); err != nil {
		exitf("inventory failed: %v", err)
	}
}

func exitf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "emote-object-manifest: "+format+"\n", args...)
	os.Exit(1)
}
