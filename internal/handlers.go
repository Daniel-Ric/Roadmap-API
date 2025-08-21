package hive

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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
		Page:          intFromQuery(r, "page", 1),
		SortBy:        strFromQuery(r, "sortBy", "upvotes:desc"),
		InReview:      boolFromQuery(r, "inReview", false),
		IncludePinned: boolFromQuery(r, "includePinned", true),
		Raw:           boolFromQuery(r, "raw", false),
		BypassCache:   !boolFromQuery(r, "cache", true),
	}
	if boolFromQuery(r, "all", false) {
		pages, err := h.svc.GetAll(r.Context(), q)
		if err != nil {
			httpError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, RoadmapAggregate{
			Column: column,
			Pages:  pages,
		})
		return
	}
	page, raw, err := h.svc.GetPage(r.Context(), q)
	if err != nil {
		httpError(w, http.StatusBadGateway, err)
		return
	}
	if q.Raw {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
		return
	}
	writeJSON(w, http.StatusOK, page)
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
