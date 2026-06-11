package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeClipperAuthSyncFile(path, clientID, accessToken, refreshToken string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("clipper auth sync mkdir: %w", err)
	}

	lines := []string{
		"# Written by chat service after Twitch sign-in — merged into .env by setup-control",
		"CLIPPER_TWITCH_CLIENT_ID=" + clientID,
		"CLIPPER_TWITCH_USER_ACCESS_TOKEN=" + accessToken,
	}
	if refresh := strings.TrimSpace(refreshToken); refresh != "" {
		lines = append(lines, "CLIPPER_TWITCH_REFRESH_TOKEN="+refresh)
	}
	content := strings.Join(lines, "\n") + "\n"

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("clipper auth sync write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("clipper auth sync rename: %w", err)
	}
	return nil
}

func (h *Handler) syncClipperAuthFile(session Session) {
	if h.cfg.ClipperAuthSyncPath == "" {
		return
	}
	if err := writeClipperAuthSyncFile(
		h.cfg.ClipperAuthSyncPath,
		h.cfg.ClientID,
		session.AccessToken,
		session.RefreshToken,
	); err != nil {
		h.log.Warn("clipper auth sync failed", "err", err)
	}
}
