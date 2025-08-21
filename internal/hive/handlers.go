package hive

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	svc Service
}

func NewHandlers(s Service) *Handlers {
	return &Handlers{svc: s}
}

func (h *Handlers) Columns(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"columns": h.svc.GetColumns(),
	})
}

func (h *Handlers) ByColumn(w http.ResponseWriter, r *http.Request) {
	column := strings.ToLower(chi.URLParam(r, "column"))
	if err := ValidateColumn(column); err != nil {
		httpError(w, http.StatusBadRequest, err)
		return
	}
	q := Query{
		Column:        column,
		SortBy:        strFromQuery(r, "sortBy", "upvotes:desc"),
		InReview:      boolFromQuery(r, "inReview", false),
		IncludePinned: boolFromQuery(r, "includePinned", true),
		Raw:           false,
		BypassCache:   !boolFromQuery(r, "cache", true),
	}
	pages, err := h.svc.GetAll(r.Context(), q)
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	all := flattenPages(pages)
	writeJSON(w, http.StatusOK, all)
}

type hiveItemOut struct {
	ID               string `json:"id"`
	Slug             string `json:"slug"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Category         string `json:"category"`
	Upvotes          int    `json:"upvotes"`
	Date             string `json:"date"`
	LastModified     string `json:"lastModified"`
	ETA              string `json:"eta,omitempty"`
	ContentText      string `json:"contentText,omitempty"`
	HasETA           bool   `json:"hasEta"`
	DateUnix         int64  `json:"dateUnix"`
	LastModifiedUnix int64  `json:"lastModifiedUnix"`
	URL              string `json:"url,omitempty"`
	Source           string `json:"source"`
}

func flattenPages(pages []RoadmapPage) struct {
	Items []hiveItemOut `json:"items"`
} {
	out := make([]hiveItemOut, 0, 512)
	for _, p := range pages {
		for _, it := range p.Items {
			var dateUnix, lmUnix int64
			if t, err := time.Parse(time.RFC3339, it.Date); err == nil {
				dateUnix = t.Unix()
			}
			if t, err := time.Parse(time.RFC3339, it.LastModified); err == nil {
				lmUnix = t.Unix()
			}
			url := "https://updates.playhive.com/en/p/" + it.Slug
			out = append(out, hiveItemOut{
				ID:               it.ID,
				Slug:             it.Slug,
				Title:            it.Title,
				Status:           it.Status,
				Category:         it.Category,
				Upvotes:          it.Upvotes,
				Date:             it.Date,
				LastModified:     it.LastModified,
				ETA:              it.ETA,
				ContentText:      it.ContentText,
				HasETA:           it.ETA != "",
				DateUnix:         dateUnix,
				LastModifiedUnix: lmUnix,
				URL:              url,
				Source:           "hive",
			})
		}
	}
	return struct {
		Items []hiveItemOut `json:"items"`
	}{Items: out}
}

func (h *Handlers) Updates(w http.ResponseWriter, _ *http.Request) {
	entries := h.svc.Updates()
	type changeOut struct {
		ChangedAt   string      `json:"changedAt"`
		ChangedAtMS int64       `json:"changedAtMs"`
		From        string      `json:"from"`
		To          string      `json:"to"`
		Item        hiveItemOut `json:"item"`
	}
	out := make([]changeOut, 0, len(entries))
	for _, e := range entries {
		var dateUnix, lmUnix int64
		if t, err := time.Parse(time.RFC3339, e.Item.Date); err == nil {
			dateUnix = t.Unix()
		}
		if t, err := time.Parse(time.RFC3339, e.Item.LastModified); err == nil {
			lmUnix = t.Unix()
		}
		url := "https://updates.playhive.com/en/p/" + e.Item.Slug
		out = append(out, changeOut{
			ChangedAt:   e.At.Format(time.RFC3339),
			ChangedAtMS: e.At.UnixMilli(),
			From:        e.From,
			To:          e.To,
			Item: hiveItemOut{
				ID:               e.Item.ID,
				Slug:             e.Item.Slug,
				Title:            e.Item.Title,
				Status:           e.Item.Status,
				Category:         e.Item.Category,
				Upvotes:          e.Item.Upvotes,
				Date:             e.Item.Date,
				LastModified:     e.Item.LastModified,
				ETA:              e.Item.ETA,
				ContentText:      e.Item.ContentText,
				HasETA:           e.Item.ETA != "",
				DateUnix:         dateUnix,
				LastModifiedUnix: lmUnix,
				URL:              url,
				Source:           "hive",
			},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"updates": out})
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

func strFromQuery(r *http.Request, key, def string) string {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	return v
}

func httpError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{
		"error": err.Error(),
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
