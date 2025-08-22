package routes

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"roadmapapi/internal/cubecraft"
	"roadmapapi/internal/hive"
)

func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(colorLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	hiveClient := hive.NewClient(
		hive.DefaultBaseURL,
		&http.Client{Timeout: 12 * time.Second},
		hive.WithCacheTTL(30*time.Second),
		hive.WithMaxConcurrency(4),
	)
	h := hive.NewHandlers(hive.NewService(hiveClient))

	ccClient := cubecraft.NewClient(
		cubecraft.WithCacheTTL(2 * time.Minute),
	)
	cc := cubecraft.NewHandlers(cubecraft.NewService(ccClient))

	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		type serviceHealth struct {
			OK        bool   `json:"ok"`
			Status    int    `json:"status"`
			LatencyMs int64  `json:"latencyMs"`
			Items     int    `json:"items"`
			Error     string `json:"error,omitempty"`
		}
		ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
		defer cancel()

		hiveStart := time.Now()
		hiveStatus, hiveItems, hiveErr := hiveClient.Probe(ctx)
		hiveRes := serviceHealth{
			OK:        hiveErr == nil && hiveStatus >= 200 && hiveStatus < 300,
			Status:    hiveStatus,
			LatencyMs: time.Since(hiveStart).Milliseconds(),
			Items:     hiveItems,
		}
		if hiveErr != nil {
			hiveRes.Error = hiveErr.Error()
		}

		notionStart := time.Now()
		notionStatus, notionItems, notionErr := ccClient.Probe(ctx)
		notionRes := serviceHealth{
			OK:        notionErr == nil && notionStatus >= 200 && notionStatus < 300,
			Status:    notionStatus,
			LatencyMs: time.Since(notionStart).Milliseconds(),
			Items:     notionItems,
		}
		if notionErr != nil {
			notionRes.Error = notionErr.Error()
		}

		ok := hiveRes.OK && notionRes.OK
		resp := map[string]any{
			"ok":        ok,
			"timestamp": time.Now().Format(time.RFC3339),
			"services": map[string]serviceHealth{
				"hive":      hiveRes,
				"cubecraft": notionRes,
			},
		}
		code := http.StatusOK
		if !ok {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	})

	r.Route("/hive", func(r chi.Router) {
		r.Get("/columns", h.Columns)
		r.Get("/{column}", h.ByColumn)
		r.Get("/updates", h.Updates)
	})

	r.Route("/cubecraft", func(r chi.Router) {
		r.Get("/columns", cc.Columns)
		r.Get("/{column}", cc.ByColumn)
		r.Get("/updates", cc.Updates)
	})

	return r
}

func colorLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		duration := time.Since(start)

		status := ww.Status()
		color := ""
		reset := "\033[0m"
		switch {
		case status >= 200 && status < 300:
			color = "\033[32m"
		case status >= 400 && status < 500:
			color = "\033[33m"
		case status >= 500:
			color = "\033[31m"
		default:
			color = "\033[37m"
		}

		log.Printf("%s %s %s%d%s %s",
			r.Method,
			r.URL.Path,
			color, status, reset,
			duration,
		)
	})
}
