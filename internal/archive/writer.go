package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
)

const (
	defaultContainer = "streamclone-archive"
	defaultPrefix    = "streamclone"
)

// BlobPutResult captures upload metadata for manifest rows.
type BlobPutResult struct {
	URI                   string
	ETag                  string
	ByteSize              int64
	RowCount              int64
	ContentSHA256         string
	UncompressedSizeBytes int64
}

// BlobStore uploads bytes to cold storage.
type BlobStore interface {
	Put(ctx context.Context, key string, data []byte, contentType string) (BlobPutResult, error)
	Get(ctx context.Context, key string) ([]byte, error)
	BlobURI(key string) string
}

// AzureConfig configures Azure Blob Storage (account pattern e.g. ststreamclone3lf6tt).
type AzureConfig struct {
	StorageAccount         string
	Container              string
	Prefix                 string
	ConnectionString       string
	ConnectionStringFile   string
}

// AzureBlobStore implements BlobStore against Azure Blob Storage.
type AzureBlobStore struct {
	client    *azblob.Client
	container string
	prefix    string
	account   string
}

func LoadConnectionString(cfg AzureConfig) (string, error) {
	if cs := strings.TrimSpace(cfg.ConnectionString); cs != "" {
		return cs, nil
	}
	file := strings.TrimSpace(cfg.ConnectionStringFile)
	if file == "" {
		return "", errors.New("archive: azure connection string or connection string file is required")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("archive: read connection string file: %w", err)
	}
	cs := strings.TrimSpace(string(raw))
	if cs == "" {
		return "", errors.New("archive: connection string file is empty")
	}
	return cs, nil
}

func NewAzureBlobStore(cfg AzureConfig) (*AzureBlobStore, error) {
	cs, err := LoadConnectionString(cfg)
	if err != nil {
		return nil, err
	}
	client, err := azblob.NewClientFromConnectionString(cs, nil)
	if err != nil {
		return nil, fmt.Errorf("archive: azure client: %w", err)
	}
	container := strings.TrimSpace(cfg.Container)
	if container == "" {
		container = defaultContainer
	}
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix == "" {
		prefix = defaultPrefix
	}
	account := strings.TrimSpace(cfg.StorageAccount)
	if account == "" {
		account = parseAccountFromConnectionString(cs)
	}
	return &AzureBlobStore{
		client:    client,
		container: container,
		prefix:    prefix,
		account:   account,
	}, nil
}

func parseAccountFromConnectionString(cs string) string {
	for _, part := range strings.Split(cs, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "accountname=") {
			return strings.TrimPrefix(part, "AccountName=")
		}
	}
	return ""
}

func (s *AzureBlobStore) fullKey(key string) string {
	key = strings.TrimPrefix(strings.TrimSpace(key), "/")
	if s.prefix == "" {
		return key
	}
	return path.Join(s.prefix, key)
}

func (s *AzureBlobStore) BlobURI(key string) string {
	key = s.fullKey(key)
	if s.account == "" {
		return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", "unknown", s.container, key)
	}
	return fmt.Sprintf("https://%s.blob.core.windows.net/%s/%s", s.account, s.container, key)
}

func (s *AzureBlobStore) Put(ctx context.Context, key string, data []byte, contentType string) (BlobPutResult, error) {
	if s == nil || s.client == nil {
		return BlobPutResult{}, errors.New("archive: azure blob store is not configured")
	}
	key = s.fullKey(key)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	resp, err := s.client.UploadBuffer(ctx, s.container, key, data, &azblob.UploadBufferOptions{
		HTTPHeaders: &blob.HTTPHeaders{BlobContentType: &contentType},
	})
	if err != nil {
		return BlobPutResult{}, err
	}
	etag := ""
	if resp.ETag != nil {
		etag = string(*resp.ETag)
	}
	return BlobPutResult{
		URI:      s.BlobURI(strings.TrimPrefix(key, s.prefix+"/")),
		ETag:     etag,
		ByteSize: int64(len(data)),
	}, nil
}

func (s *AzureBlobStore) Get(ctx context.Context, key string) ([]byte, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("archive: azure blob store is not configured")
	}
	key = s.fullKey(key)
	resp, err := s.client.DownloadStream(ctx, s.container, key, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

type manifestUpserter interface {
	Upsert(ctx context.Context, rec ExportRecord) error
}

// WriterOptions gates SHA-256 hashing and sidecar manifest uploads.
type WriterOptions struct {
	ContentHashEnabled         bool
	WriteSidecarManifest       bool
	ParserVersion              string
	SilverPartialMinCoverage   float64
	SilverRawTTChartJSON       bool
	SilverRawTTMaxBytes        int
}

// Writer exports analytics artifacts to blob storage and records manifest rows.
type Writer struct {
	blob     BlobStore
	manifest manifestUpserter
	opts     WriterOptions
}

func NewWriter(blob BlobStore, manifest manifestUpserter) *Writer {
	return &Writer{blob: blob, manifest: manifest}
}

func (w *Writer) WithOptions(opts WriterOptions) *Writer {
	if w != nil {
		w.opts = opts
	}
	return w
}

func Gzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Gunzip(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, 256<<20))
}

func RollupsBlobKey(streamID string) string {
	return fmt.Sprintf("rollups/stream_id=%s/part-000.jsonl.gz", streamID)
}

func StreamSessionBlobKey(streamID string) string {
	return fmt.Sprintf("streams/stream_id=%s/session.json.gz", streamID)
}

// StreamChannelBlobKey is an idempotent per-stream index object under channels/{login}/.
func StreamChannelBlobKey(login, streamID string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	return fmt.Sprintf("streams/channels/%s/stream_id=%s.jsonl.gz", login, streamID)
}

// TTDetailBlobKey stores optional raw TwitchTracker HTML for re-parse/debug.
func TTDetailBlobKey(login, streamID string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	return fmt.Sprintf("tt-detail/%s/%s/page.html.gz", login, streamID)
}

func PostgresNightlyBlobKey(date string) string {
	return fmt.Sprintf("postgres/nightly/%s.sql.gz", date)
}

func TopRosterBlobKey() string {
	return "channels/top200.json.gz"
}

func Top500BlobKey() string {
	return "channels/top500.json.gz"
}

func ChannelSummaryBlobKey(login string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	return fmt.Sprintf("channels/summary/%s.json", login)
}

func VODIndexBlobKey(login string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	return fmt.Sprintf("channels/vod_index/%s.jsonl.gz", login)
}

func VODChatBlobKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	return fmt.Sprintf("vod_chat/stream_id=%s/messages.jsonl.gz", streamID)
}

func BronzeVODCatalogBlobKey(login, date string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	date = strings.TrimSpace(date)
	return fmt.Sprintf("vod_catalog/v1/login=%s/date=%s/part-000.jsonl.gz", login, date)
}

func ChannelIdentityBlobKey(login, date string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	date = strings.TrimSpace(date)
	return fmt.Sprintf("channels/identity/login=%s/date=%s/identity.json.gz", login, date)
}

func ProviderCrosswalkBlobKey(login, date string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	date = strings.TrimSpace(date)
	return fmt.Sprintf("channels/crosswalk/login=%s/date=%s/crosswalk.json.gz", login, date)
}

func RosterTier0BlobKey(date string) string {
	date = strings.TrimSpace(date)
	return fmt.Sprintf("rosters/tier0/date=%s/part-000.json.gz", date)
}

func TTChartJSONBlobKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	return fmt.Sprintf("raw/twitchtracker/stream_id=%s/chart.json.gz", streamID)
}

func GoldLiteChatBlobKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	return fmt.Sprintf("rollups/chat/stream_id=%s/minute.jsonl.gz", streamID)
}

func (w *Writer) putGzip(ctx context.Context, key string, raw []byte) (BlobPutResult, error) {
	gz, err := Gzip(raw)
	if err != nil {
		return BlobPutResult{}, err
	}
	res, err := w.blob.Put(ctx, key, gz, "application/gzip")
	if err != nil {
		return BlobPutResult{}, err
	}
	res.ByteSize = int64(len(gz))
	res.UncompressedSizeBytes = int64(len(raw))
	if w != nil && w.opts.ContentHashEnabled {
		sum := sha256.Sum256(gz)
		res.ContentSHA256 = hex.EncodeToString(sum[:])
	}
	if w != nil && w.opts.WriteSidecarManifest {
		_ = w.writeSidecar(ctx, key, res)
	}
	return res, nil
}

func (w *Writer) writeSidecar(ctx context.Context, blobKey string, res BlobPutResult) error {
	sidecar := map[string]any{
		"blobKey":               blobKey,
		"uri":                   res.URI,
		"etag":                  res.ETag,
		"byteSize":              res.ByteSize,
		"contentSha256":         res.ContentSHA256,
		"uncompressedSizeBytes": res.UncompressedSizeBytes,
		"parserVersion":         w.opts.ParserVersion,
		"exportedAt":            time.Now().UTC(),
	}
	rawJSON, err := json.Marshal(sidecar)
	if err != nil {
		return err
	}
	sidecarKey := fmt.Sprintf("manifests/%s.json", strings.TrimSuffix(blobKey, ".gz"))
	_, err = w.blob.Put(ctx, sidecarKey, rawJSON, "application/json")
	return err
}

func (w *Writer) confirmManifest(ctx context.Context, artifactType, naturalKey string, res BlobPutResult) error {
	if w.manifest == nil {
		return nil
	}
	rec := ExportRecord{
		ArtifactType:          artifactType,
		NaturalKey:            naturalKey,
		GCSURI:                res.URI,
		ETag:                  res.ETag,
		RowCount:              res.RowCount,
		ByteSize:              res.ByteSize,
		Status:                StatusConfirmed,
		ExportedAt:            time.Now().UTC(),
		ContentSHA256:         res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if w.opts.ParserVersion != "" {
		rec.Metadata = map[string]any{"parserVersion": w.opts.ParserVersion}
	}
	err := w.manifest.Upsert(ctx, rec)
	if err == nil {
		recordArchiveExportConfirmed(artifactType)
	}
	return err
}
