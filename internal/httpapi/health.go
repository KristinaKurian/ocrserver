package httpapi

import (
	"fmt"
	"net/http"
)

func (a *API) handleLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"}, false)
}

func (a *API) handleReady(w http.ResponseWriter, _ *http.Request) {
	stats := a.service.Stats()
	if !a.service.Ready() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
		}, false)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ready",
		"workers":        stats.Workers,
		"queue_length":   stats.QueueLength,
		"queue_capacity": stats.QueueCapacity,
	}, false)
}

func (a *API) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	stats := a.service.Stats()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	fmt.Fprintf(w, "# TYPE ocr_http_requests_total counter\nocr_http_requests_total %d\n", a.metrics.requests.Load())
	fmt.Fprintf(w, "# TYPE ocr_http_failures_total counter\nocr_http_failures_total %d\n", a.metrics.failures.Load())
	fmt.Fprintf(w, "# TYPE ocr_http_requests_in_flight gauge\nocr_http_requests_in_flight %d\n", a.metrics.inFlight.Load())
	fmt.Fprintf(w, "# TYPE ocr_worker_queue_length gauge\nocr_worker_queue_length %d\n", stats.QueueLength)
	fmt.Fprintf(w, "# TYPE ocr_worker_queue_capacity gauge\nocr_worker_queue_capacity %d\n", stats.QueueCapacity)
	fmt.Fprintf(w, "# TYPE ocr_workers gauge\nocr_workers %d\n", stats.Workers)
	fmt.Fprintf(w, "# TYPE ocr_queue_rejected_total counter\nocr_queue_rejected_total %d\n", stats.Rejected)
}
