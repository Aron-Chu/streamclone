package archive

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type BlobVerifyReport struct {
	MissingInAzure []string `json:"missingInAzure"`
	MissingInIndex []string `json:"missingInIndex"`
	Checked        int      `json:"checked"`
}

// VerifyManifestBlobs compares Postgres manifest URIs against blob store keys (scoped sample).
func VerifyManifestBlobs(ctx context.Context, pool *pgxpool.Pool, blob BlobStore, limit int) (*BlobVerifyReport, error) {
	if pool == nil || blob == nil {
		return nil, fmt.Errorf("verify-blobs: pool and blob store required")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := pool.Query(ctx, `
		SELECT artifact_type, natural_key, gcs_uri
		FROM archive_exports
		WHERE export_status IN ('confirmed','partial','complete')
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	report := &BlobVerifyReport{}
	for rows.Next() {
		var artifactType, naturalKey, uri string
		if err := rows.Scan(&artifactType, &naturalKey, &uri); err != nil {
			return nil, err
		}
		report.Checked++
		key := blobKeyFromURI(uri)
		if key == "" {
			report.MissingInAzure = append(report.MissingInAzure, fmt.Sprintf("%s:%s (bad uri)", artifactType, naturalKey))
			continue
		}
		_, err := blob.Get(ctx, key)
		if err != nil {
			report.MissingInAzure = append(report.MissingInAzure, fmt.Sprintf("%s:%s", artifactType, naturalKey))
		}
	}
	return report, rows.Err()
}

func blobKeyFromURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	// https://account.blob.core.windows.net/container/prefix/key
	idx := strings.Index(uri, ".blob.core.windows.net/")
	if idx < 0 {
		return ""
	}
	rest := uri[idx+len(".blob.core.windows.net/"):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	// strip container name
	path := parts[1]
	// strip streamclone prefix if present for Get()
	if strings.HasPrefix(path, "streamclone/") {
		return strings.TrimPrefix(path, "streamclone/")
	}
	return path
}
