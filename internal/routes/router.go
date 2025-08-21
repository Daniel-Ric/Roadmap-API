package routes

import (
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

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	hiveClient := hive.NewClient(
		hive.DefaultBaseURL,
		&http.Client{Timeout: 12 * time.Second},
		hive.WithCacheTTL(30*time.Second),
		hive.WithMaxConcurrency(4),
	)
	h := hive.NewHandlers(hive.NewService(hiveClient))
	r.Route("/hive", func(r chi.Router) {
		r.Get("/columns", h.Columns)
		r.Get("/{column}", h.ByColumn)
		r.Get("/updates", h.Updates)
	})

	ccClient := cubecraft.NewClient(
		cubecraft.WithCacheTTL(2 * time.Minute),
	)
	cc := cubecraft.NewHandlers(cubecraft.NewService(ccClient))
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
