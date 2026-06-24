package analytics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type PulseWatchlistEntry struct {
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	AlwaysTrack bool      `json:"alwaysTrack"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type PulseWatchlistResponse struct {
	Items []PulseWatchlistEntry `json:"items"`
}

type addPulseWatchlistRequest struct {
	Login       string `json:"login"`
	AlwaysTrack bool   `json:"alwaysTrack"`
}

func (h *Handler) registerPulseWatchlistRoutes(r chi.Router) {
	r.Get("/watchlist/summary", h.getPulseWatchlistSummary)
	r.Get("/watchlist", h.listPulseWatchlist)
	r.Post("/watchlist", h.addPulseWatchlist)
	r.Delete("/watchlist/{login}", h.deletePulseWatchlist)
}

func (h *Handler) listPulseWatchlist(w http.ResponseWriter, r *http.Request) {
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	items, err := h.store.ListPulseWatchlist(r.Context(), principal.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, PulseWatchlistResponse{Items: items})
}

func (h *Handler) addPulseWatchlist(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req addPulseWatchlistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	login, ok := validLogin(req.Login)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_login"})
		return
	}
	if err := h.validateWatchlistLogin(r.Context(), login); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if req.AlwaysTrack {
		if !h.enforceWatchRateLimit(w, r) {
			return
		}
	}
	maxPerPrincipal := h.pulseHosted.MaxChannelsPerPrincipal
	if maxPerPrincipal > 0 {
		count, err := h.store.CountPulseWatchlist(r.Context(), principal.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		exists, err := h.store.PulseWatchlistHasLogin(r.Context(), principal.ID, login)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !exists && count >= maxPerPrincipal {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "watchlist_cap_reached"})
			return
		}
	}
	globalLimit := h.pulseRuntimeConfig().ProtectedGlobalLimit
	var entry PulseWatchlistEntry
	var err error
	if req.AlwaysTrack {
		entry, err = h.store.UpsertPulseWatchlistWithCap(r.Context(), principal.ID, principal.Kind, login, req.AlwaysTrack, globalLimit)
	} else {
		entry, err = h.store.UpsertPulseWatchlist(r.Context(), principal.ID, principal.Kind, login, req.AlwaysTrack)
	}
	if err != nil {
		if isProtectedCapError(err) {
			writeProtectedCapReached(w)
			return
		}
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "duplicate_login"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if req.AlwaysTrack && h.collector != nil {
		h.collector.WatchWithPriority(r.Context(), login, principal.ID, TrackPriorityPrincipalAlwaysTrack)
		h.collector.SetPoolAlwaysTrack(login, true)
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) deletePulseWatchlist(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_login"})
		return
	}
	entry, err := h.store.GetPulseWatchlistEntry(r.Context(), principal.ID, login)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "watchlist_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.store.DeletePulseWatchlist(r.Context(), principal.ID, login); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if entry.AlwaysTrack && h.collector != nil {
		h.collector.SetPoolAlwaysTrack(login, false)
		h.collector.ReleaseForPrincipal(login, principal.ID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) validateWatchlistLogin(ctx context.Context, login string) error {
	if h.pulseRuntimeConfig().HelixMetadataEnabled && h.helix != nil && h.helix.Enabled() {
		users, err := h.helix.UsersByLogin(ctx, []string{login})
		if err == nil {
			if profile, ok := users[login]; ok && strings.TrimSpace(profile.ID) != "" {
				return nil
			}
		}
	}
	if h.store != nil {
		if _, err := h.store.LatestStreamByLogin(ctx, login); err == nil {
			return nil
		}
	}
	return errors.New("unknown_login")
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func newPulseWatchlistID() string {
	var buf [10]byte
	_, _ = rand.Read(buf[:])
	ms := time.Now().UTC().UnixMilli()
	return strconv.FormatInt(ms, 36) + hex.EncodeToString(buf[:])
}

func (s *Store) ListPulseWatchlist(ctx context.Context, principalID string) ([]PulseWatchlistEntry, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, login, always_track, created_at, updated_at
		FROM pulse_watchlist
		WHERE principal_id = $1
		ORDER BY created_at DESC, login ASC`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]PulseWatchlistEntry, 0, 8)
	for rows.Next() {
		var item PulseWatchlistEntry
		if err := rows.Scan(&item.ID, &item.Login, &item.AlwaysTrack, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CountPulseWatchlist(ctx context.Context, principalID string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM pulse_watchlist WHERE principal_id = $1`, principalID).Scan(&count)
	return count, err
}

func (s *Store) PulseWatchlistHasLogin(ctx context.Context, principalID, login string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pulse_watchlist WHERE principal_id = $1 AND login = $2
		)`, principalID, login).Scan(&exists)
	return exists, err
}

func (s *Store) GetPulseWatchlistEntry(ctx context.Context, principalID, login string) (PulseWatchlistEntry, error) {
	var item PulseWatchlistEntry
	err := s.db.QueryRow(ctx, `
		SELECT id, login, always_track, created_at, updated_at
		FROM pulse_watchlist
		WHERE principal_id = $1 AND login = $2`, principalID, login).Scan(
		&item.ID, &item.Login, &item.AlwaysTrack, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) UpsertPulseWatchlist(ctx context.Context, principalID, principalKind, login string, alwaysTrack bool) (PulseWatchlistEntry, error) {
	id := newPulseWatchlistID()
	var item PulseWatchlistEntry
	err := s.db.QueryRow(ctx, `
		INSERT INTO pulse_watchlist (id, principal_id, principal_kind, login, always_track)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (principal_id, login) DO UPDATE SET
			always_track = EXCLUDED.always_track,
			updated_at = now()
		RETURNING id, login, always_track, created_at, updated_at`,
		id, principalID, principalKind, login, alwaysTrack).Scan(
		&item.ID, &item.Login, &item.AlwaysTrack, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (s *Store) DeletePulseWatchlist(ctx context.Context, principalID, login string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM pulse_watchlist WHERE principal_id = $1 AND login = $2`,
		principalID, login)
	return err
}
