package cubecraft

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"roadmapapi/internal/hive"
)

type Handlers struct {
	svc Service
}

func NewHandlers(s Service) *Handlers { return &Handlers{svc: s} }

func (h *Handlers) Columns(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"columns": h.svc.Columns()})
}

func (h *Handlers) ByColumn(w http.ResponseWriter, r *http.Request) {
	column := strings.ToLower(chi.URLParam(r, "column"))
	if _, ok := columnToStatus[column]; !ok {
		httpError(w, http.StatusBadRequest, "column must be one of [in-progress, coming-next, released]")
		return
	}
	sortBy := strFromQuery(r, "sortBy", "")
	pages, err := h.svc.All(r.Context(), column, defaultPageSize, sortBy)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	all := flattenPages(pages)
	writeJSON(w, http.StatusOK, all)
}

type cubeItemOut struct {
	ID               string `json:"id"`
	Slug             string `json:"slug"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Category         string `json:"category"`
	Network          string `json:"network,omitempty"`
	ProjectLead      string `json:"projectLead,omitempty"`
	Date             string `json:"date"`
	LastModified     string `json:"lastModified"`
	ETA              string `json:"eta,omitempty"`
	Released         bool   `json:"released"`
	ReleasedAt       string `json:"releasedAt,omitempty"`
	DateUnix         int64  `json:"dateUnix"`
	LastModifiedUnix int64  `json:"lastModifiedUnix"`
	URL              string `json:"url,omitempty"`
	Source           string `json:"source"`
}

func (h *Handlers) Updates(w http.ResponseWriter, _ *http.Request) {
	entries := h.svc.Updates()
	type changeOut struct {
		ChangedAt   string      `json:"changedAt"`
		ChangedAtMS int64       `json:"changedAtMs"`
		From        string      `json:"from"`
		To          string      `json:"to"`
		Item        cubeItemOut `json:"item"`
	}
	out := make([]changeOut, 0, len(entries))
	for _, e := range entries {
		createdStr := e.Item.CreatedAt.Format(time.RFC3339)
		updatedStr := e.Item.UpdatedAt.Format(time.RFC3339)
		etaStr := isoOrEmpty(e.Item.ReleasedAt)
		dateUnix := e.Item.CreatedAt.Unix()
		lmUnix := e.Item.UpdatedAt.Unix()
		released := strings.EqualFold(e.Item.Status, "Released")
		out = append(out, changeOut{
			ChangedAt:   e.At.Format(time.RFC3339),
			ChangedAtMS: e.At.UnixMilli(),
			From:        e.From,
			To:          e.To,
			Item: cubeItemOut{
				ID:               e.Item.ID,
				Slug:             e.Item.Slug,
				Title:            e.Item.Title,
				Status:           e.Item.Status,
				Category:         e.Item.Category,
				Network:          e.Item.Network,
				ProjectLead:      e.Item.ProjectLead,
				Date:             createdStr,
				LastModified:     updatedStr,
				ETA:              etaStr,
				Released:         released,
				ReleasedAt:       etaStr,
				DateUnix:         dateUnix,
				LastModifiedUnix: lmUnix,
				URL:              e.Item.URL,
				Source:           "cubecraft",
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"updates": out})
}

func flattenPages(pages []hive.RoadmapPage) struct {
	Items []cubeItemOut `json:"items"`
} {
	out := make([]cubeItemOut, 0, 512)
	for _, p := range pages {
		for _, it := range p.Items {
			var dateUnix, lmUnix int64
			if t, err := time.Parse(time.RFC3339, it.Date); err == nil {
				dateUnix = t.Unix()
			}
			if t, err := time.Parse(time.RFC3339, it.LastModified); err == nil {
				lmUnix = t.Unix()
			}
			released := strings.EqualFold(it.Status, "Released")
			releasedAt := it.ETA
			out = append(out, cubeItemOut{
				ID:               it.ID,
				Slug:             it.Slug,
				Title:            it.Title,
				Status:           it.Status,
				Category:         it.Category,
				Network:          it.Network,
				ProjectLead:      it.ProjectLead,
				Date:             it.Date,
				LastModified:     it.LastModified,
				ETA:              it.ETA,
				Released:         released,
				ReleasedAt:       releasedAt,
				DateUnix:         dateUnix,
				LastModifiedUnix: lmUnix,
				URL:              it.URL,
				Source:           "cubecraft",
			})
		}
	}
	return struct {
		Items []cubeItemOut `json:"items"`
	}{Items: out}
}

func intFromQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil || i <= 0 {
		return def
	}
	return i
}

func boolFromQuery(r *http.Request, key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func strFromQuery(r *http.Request, key string, def string) string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}
