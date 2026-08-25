package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/KristinaKurian/ocrserver/internal/ocr"
)

type Config struct {
	MaxImageBytes int64
}

type API struct {
	service *ocr.Service
	config  Config
	logger  *slog.Logger
	metrics *Metrics
}

func New(service *ocr.Service, config Config, logger *slog.Logger) http.Handler {
	api := &API{
		service: service,
		config:  config,
		logger:  logger,
		metrics: &Metrics{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ocr", api.handleOCR)
	mux.HandleFunc("GET /health/live", api.handleLive)
	mux.HandleFunc("GET /health/ready", api.handleReady)
	mux.HandleFunc("GET /metrics", api.handleMetrics)

	return api.loggingMiddleware(api.metricsMiddleware(mux))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (a *API) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		a.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

func (a *API) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.metrics.requestStarted()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		a.metrics.requestFinished(sw.status >= http.StatusBadRequest)
	})
}
