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
	"github.com/jackc/pgx/v5/pgtype"
)

type PulseBookmark struct {
	ID            string    `json:"id"`
	UserID        *string   `json:"userId,omitempty"`
	PrincipalID   *string   `json:"principalId,omitempty"`
	PrincipalKind *string   `json:"principalKind,omitempty"`
	Login         string    `json:"login"`
	StreamID      *string   `json:"streamId,omitempty"`
	VodID         *string   `json:"vodId,omitempty"`
	OffsetSeconds int       `json:"offsetSeconds"`
	Label         string    `json:"label"`
	Notes         string    `json:"notes"`
	Score         *int      `json:"score,omitempty"`
	Source        string    `json:"source"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type PulseBookmarkListResponse struct {
	Items      []PulseBookmark `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type CreatePulseBookmarkRequest struct {
	Login         string `json:"login"`
	StreamID      string `json:"streamId"`
	VodID         string `json:"vodId"`
	OffsetSeconds int    `json:"offsetSeconds"`
	Label         string `json:"label"`
	Notes         string `json:"notes"`
	Score         *int   `json:"score"`
	Source        string `json:"source"`
}

type UpdatePulseBookmarkRequest struct {
	Label *string `json:"label"`
	Notes *string `json:"notes"`
}

type ListPulseBookmarksFilter struct {
	Login       string
	StreamID    string
	VodID       string
	PrincipalID string
	Limit       int
	Before      time.Time
}

func (h *Handler) PulseRoutes(r chi.Router) {
	r.Route("/v1/pulse", func(r chi.Router) {
		if h.pulseHosted.Hosted {
			r.Use(h.pulseHostedAuthMiddleware)
		}
		h.registerPulseWatchlistRoutes(r)
		h.registerPulseClipRoutes(r)
		r.Get("/bookmarks", h.listPulseBookmarks)
		r.Post("/bookmarks", h.createPulseBookmark)
		r.Patch("/bookmarks/{id}", h.updatePulseBookmark)
		r.Delete("/bookmarks/{id}", h.deletePulseBookmark)
		r.Get("/streams/{streamID}/recap", h.getPulseStreamRecap)
	})
}

func (h *Handler) listPulseBookmarks(w http.ResponseWriter, r *http.Request) {
	if h.pulseHosted.Hosted && h.store == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	filter := ListPulseBookmarksFilter{
		Login:    strings.TrimSpace(strings.ToLower(r.URL.Query().Get("login"))),
		StreamID: strings.TrimSpace(r.URL.Query().Get("streamId")),
		VodID:    strings.TrimSpace(r.URL.Query().Get("vodId")),
		Limit:    parseBookmarkLimit(r.URL.Query().Get("limit")),
	}
	if cursor := strings.TrimSpace(r.URL.Query().Get("cursor")); cursor != "" {
		t, err := time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		filter.Before = t
	}
	if filter.Login != "" {
		if _, ok := validLogin(filter.Login); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_login"})
			return
		}
	}
	if h.pulseHosted.Hosted {
		principal, ok := h.requireHostedUserStatePrincipal(w, r)
		if !ok {
			return
		}
		filter.PrincipalID = principal.ID
	}
	items, nextCursor, err := h.store.ListPulseBookmarks(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, PulseBookmarkListResponse{Items: items, NextCursor: nextCursor})
}

func (h *Handler) createPulseBookmark(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	var req CreatePulseBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	bookmark, err := h.preparePulseBookmark(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if h.pulseHosted.Hosted {
		principal, ok := h.requireHostedUserStatePrincipal(w, r)
		if !ok {
			return
		}
		bookmark.PrincipalID = &principal.ID
		bookmark.PrincipalKind = &principal.Kind
	}
	created, err := h.store.CreatePulseBookmark(r.Context(), bookmark)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) updatePulseBookmark(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	var req UpdatePulseBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	label, notes, err := normalizeBookmarkPatch(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	principalID := scopedPrincipalID(r, h)
	if h.pulseHosted.Hosted {
		principal, ok := h.requireHostedUserStatePrincipal(w, r)
		if !ok {
			return
		}
		principalID = principal.ID
	}
	updated, err := h.store.UpdatePulseBookmark(r.Context(), id, label, notes, principalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "bookmark_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deletePulseBookmark(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	principalID := scopedPrincipalID(r, h)
	if h.pulseHosted.Hosted {
		principal, ok := h.requireHostedUserStatePrincipal(w, r)
		if !ok {
			return
		}
		principalID = principal.ID
	}
	if err := h.store.DeletePulseBookmark(r.Context(), id, principalID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) preparePulseBookmark(ctx context.Context, req CreatePulseBookmarkRequest) (PulseBookmark, error) {
	login := strings.TrimSpace(strings.ToLower(req.Login))
	streamID := trimOptional(req.StreamID)
	vodID := trimOptional(req.VodID)

	if login == "" && streamID != nil {
		stream, err := h.store.StreamByID(ctx, *streamID)
		if err == nil {
			login = stream.Login
			if vodID == nil && stream.VodID != "" {
				vodID = &stream.VodID
			}
		}
	}
	if _, ok := validLogin(login); !ok {
		return PulseBookmark{}, errors.New("missing_or_invalid_login")
	}
	if req.OffsetSeconds < 0 {
		return PulseBookmark{}, errors.New("invalid_offset")
	}
	source := normalizeBookmarkSource(req.Source)
	if source == "" {
		return PulseBookmark{}, errors.New("invalid_source")
	}
	if req.Score != nil && (*req.Score < 0 || *req.Score > 100) {
		return PulseBookmark{}, errors.New("invalid_score")
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = formatBookmarkDefaultLabel(req.OffsetSeconds)
	}
	return PulseBookmark{
		ID:            newPulseBookmarkID(),
		Login:         login,
		StreamID:      streamID,
		VodID:         vodID,
		OffsetSeconds: req.OffsetSeconds,
		Label:         clampText(label, 160),
		Notes:         clampText(strings.TrimSpace(req.Notes), 1000),
		Score:         req.Score,
		Source:        source,
	}, nil
}

func scopedPrincipalID(r *http.Request, h *Handler) string {
	if h == nil || !h.pulseHosted.Hosted {
		return ""
	}
	if p, ok := pulsePrincipalFromContext(r.Context()); ok {
		return p.ID
	}
	return ""
}

func (s *Store) ListPulseBookmarks(ctx context.Context, filter ListPulseBookmarksFilter) ([]PulseBookmark, string, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	args := []any{}
	clauses := []string{"1=1"}
	if filter.Login != "" {
		args = append(args, filter.Login)
		clauses = append(clauses, "login=$"+strconv.Itoa(len(args)))
	}
	if filter.StreamID != "" {
		args = append(args, filter.StreamID)
		clauses = append(clauses, "stream_id=$"+strconv.Itoa(len(args)))
	}
	if filter.VodID != "" {
		args = append(args, filter.VodID)
		clauses = append(clauses, "vod_id=$"+strconv.Itoa(len(args)))
	}
	if filter.PrincipalID != "" {
		args = append(args, filter.PrincipalID)
		clauses = append(clauses, "principal_id=$"+strconv.Itoa(len(args)))
	}
	if !filter.Before.IsZero() {
		args = append(args, filter.Before)
		clauses = append(clauses, "created_at<$"+strconv.Itoa(len(args)))
	}
	args = append(args, filter.Limit+1)
	query := `
		SELECT id, user_id, principal_id, principal_kind, login, stream_id, vod_id, offset_seconds, label, notes, score, source, created_at, updated_at
		FROM pulse_bookmarks
		WHERE ` + strings.Join(clauses, " AND ") + `
		ORDER BY created_at DESC, id DESC
		LIMIT $` + strconv.Itoa(len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	items := make([]PulseBookmark, 0, filter.Limit)
	for rows.Next() {
		item, err := scanPulseBookmark(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(items) > filter.Limit {
		nextCursor = items[filter.Limit-1].CreatedAt.Format(time.RFC3339Nano)
		items = items[:filter.Limit]
	}
	return items, nextCursor, nil
}

func (s *Store) CreatePulseBookmark(ctx context.Context, bookmark PulseBookmark) (PulseBookmark, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO pulse_bookmarks (id, principal_id, principal_kind, login, stream_id, vod_id, offset_seconds, label, notes, score, source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, user_id, principal_id, principal_kind, login, stream_id, vod_id, offset_seconds, label, notes, score, source, created_at, updated_at`,
		bookmark.ID, bookmark.PrincipalID, bookmark.PrincipalKind, bookmark.Login, bookmark.StreamID, bookmark.VodID, bookmark.OffsetSeconds,
		bookmark.Label, bookmark.Notes, bookmark.Score, bookmark.Source)
	return scanPulseBookmark(row)
}

func (s *Store) UpdatePulseBookmark(ctx context.Context, id string, label, notes *string, principalID string) (PulseBookmark, error) {
	args := []any{id, label, notes}
	query := `
		UPDATE pulse_bookmarks
		SET label=COALESCE($2, label),
			notes=COALESCE($3, notes),
			updated_at=now()
		WHERE id=$1`
	if principalID != "" {
		args = append(args, principalID)
		query += ` AND principal_id=$` + strconv.Itoa(len(args))
	}
	query += `
		RETURNING id, user_id, principal_id, principal_kind, login, stream_id, vod_id, offset_seconds, label, notes, score, source, created_at, updated_at`
	row := s.db.QueryRow(ctx, query, args...)
	return scanPulseBookmark(row)
}

func (s *Store) DeletePulseBookmark(ctx context.Context, id string, principalID string) error {
	if principalID != "" {
		tag, err := s.db.Exec(ctx, `DELETE FROM pulse_bookmarks WHERE id=$1 AND principal_id=$2`, id, principalID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	}
	_, err := s.db.Exec(ctx, `DELETE FROM pulse_bookmarks WHERE id=$1`, id)
	return err
}

type pulseBookmarkScanner interface {
	Scan(dest ...any) error
}

func scanPulseBookmark(row pulseBookmarkScanner) (PulseBookmark, error) {
	var item PulseBookmark
	var userID pgtype.Text
	var principalID pgtype.Text
	var principalKind pgtype.Text
	var streamID pgtype.Text
	var vodID pgtype.Text
	var score pgtype.Int4
	if err := row.Scan(
		&item.ID,
		&userID,
		&principalID,
		&principalKind,
		&item.Login,
		&streamID,
		&vodID,
		&item.OffsetSeconds,
		&item.Label,
		&item.Notes,
		&score,
		&item.Source,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return PulseBookmark{}, err
	}
	item.UserID = textPtr(userID)
	item.PrincipalID = textPtr(principalID)
	item.PrincipalKind = textPtr(principalKind)
	item.StreamID = textPtr(streamID)
	item.VodID = textPtr(vodID)
	if score.Valid {
		v := int(score.Int32)
		item.Score = &v
	}
	return item, nil
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func parseBookmarkLimit(raw string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	if n <= 0 || n > 100 {
		return 50
	}
	return n
}

func normalizeBookmarkPatch(req UpdatePulseBookmarkRequest) (*string, *string, error) {
	var label *string
	var notes *string
	if req.Label != nil {
		v := clampText(strings.TrimSpace(*req.Label), 160)
		if v == "" {
			return nil, nil, errors.New("invalid_label")
		}
		label = &v
	}
	if req.Notes != nil {
		v := clampText(strings.TrimSpace(*req.Notes), 1000)
		notes = &v
	}
	if label == nil && notes == nil {
		return nil, nil, errors.New("empty_patch")
	}
	return label, notes, nil
}

func normalizeBookmarkSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "", "web":
		return "web"
	case "extension":
		return "extension"
	default:
		return ""
	}
}

func trimOptional(value string) *string {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	return &v
}

func clampText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func formatBookmarkDefaultLabel(offsetSeconds int) string {
	return "Moment at " + strconv.Itoa(offsetSeconds) + "s"
}

func newPulseBookmarkID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "bk_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "bk_" + strconv.FormatInt(time.Now().UnixNano(), 36) + hex.EncodeToString(buf[:])
}
