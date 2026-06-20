package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

func HashSevenTVEmoteIDs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, ",")))
	return hex.EncodeToString(sum[:])
}

func ProviderSnapshotNeedsRefresh(localFound bool, localSetID, localHash, remoteSetID, remoteHash string, localCount, remoteCount int) bool {
	if remoteSetID == "" {
		return false
	}
	if !localFound {
		return true
	}
	if localSetID != remoteSetID {
		return true
	}
	if remoteHash != "" && localHash != remoteHash {
		return true
	}
	if remoteCount > 0 && localCount != remoteCount {
		return true
	}
	return false
}

func FormatSnapshotMismatch(localSetID, localHash string, localCount int, remoteSetID, remoteHash string, remoteCount int) string {
	return fmt.Sprintf("local(set=%s hash=%s count=%d) remote(set=%s hash=%s count=%d)", localSetID, localHash, localCount, remoteSetID, remoteHash, remoteCount)
}
