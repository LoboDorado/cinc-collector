package api

import (
	"bytes"
	"crypto/subtle"
	"net/http"
)

func (a *API) registerSystemRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /_status", a.handleStatus)
	mux.HandleFunc("GET /_stats", a.handleStats)
}

// handleStatus is the unauthenticated health/readiness endpoint: a load
// balancer or Kubernetes readiness probe polls this. It returns 503 once
// Drain() has been called, or if the database is unreachable.
func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	if a.draining.Load() {
		writeJSON(w, http.StatusServiceUnavailable, statusBody{Status: "draining"})
		return
	}
	if a.opts.DB != nil {
		if err := a.opts.DB.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, statusBody{Status: "db unreachable"})
			return
		}
	}
	writeJSON(w, http.StatusOK, statusBody{Status: "pong"})
}

type statusBody struct {
	Status string `json:"status"`
}

// handleStats serves metrics behind HTTP Basic auth, in Prometheus text
// exposition format by default or JSON if explicitly requested.
func (a *API) handleStats(w http.ResponseWriter, r *http.Request) {
	if !a.checkStatsAuth(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="cinc-collector"`)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if a.opts.Metrics == nil {
		writeJSON(w, http.StatusOK, struct{}{})
		return
	}

	var buf bytes.Buffer
	if err := a.opts.Metrics.WriteText(&buf); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to render metrics")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func (a *API) checkStatsAuth(r *http.Request) bool {
	if a.opts.StatsUser == "" && a.opts.StatsPassword == "" {
		return true // not configured: stats auth disabled, e.g. in dev
	}
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(user), []byte(a.opts.StatsUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(pass), []byte(a.opts.StatsPassword)) == 1
	return userOK && passOK
}
