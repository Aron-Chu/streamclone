package archive

import (
	"encoding/json"
	"fmt"
	"time"
)

const EmoteSnapshotSchemaVersion = "emote_snapshot/v1"

// EmoteSnapshotDocument is the provider-agnostic cold-storage snapshot envelope (TASK-011B).
type EmoteSnapshotDocument struct {
	SchemaVersion string              `json:"schemaVersion"`
	Provider      string              `json:"provider"`
	Login         string              `json:"login"`
	ProviderSetID string              `json:"providerSetId,omitempty"`
	EmoteHash     string              `json:"emoteHash,omitempty"`
	Count         int                 `json:"count"`
	Strategy      string              `json:"strategy,omitempty"`
	ExportedAt    time.Time           `json:"exportedAt"`
	Emotes        []EmoteSnapshotLine `json:"emotes"`
}

func NewEmoteSnapshotDocument(meta EmoteSnapshotMeta, lines []EmoteSnapshotLine) EmoteSnapshotDocument {
	return EmoteSnapshotDocument{
		SchemaVersion: EmoteSnapshotSchemaVersion,
		Provider:      meta.Provider,
		Login:         meta.Login,
		ProviderSetID: meta.ProviderSetID,
		EmoteHash:     meta.EmoteHash,
		Count:         meta.Count,
		Strategy:      meta.Strategy,
		ExportedAt:    meta.ExportedAt,
		Emotes:        lines,
	}
}

func ValidateEmoteSnapshotDocument(raw []byte) error {
	var doc EmoteSnapshotDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if doc.SchemaVersion != EmoteSnapshotSchemaVersion && doc.SchemaVersion != "" {
		return fmt.Errorf("unsupported emote snapshot schema %q", doc.SchemaVersion)
	}
	return nil
}
