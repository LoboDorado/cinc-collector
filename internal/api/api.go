// Package api implements Cinc Collector's HTTP handlers: the Chef
// Automate-compatible data-collector ingest endpoint, the query API for
// Cinc Console, the InSpec profile store, and system/health endpoints.
package api

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/cinc-project/cinc-collector/internal/auth"
	"github.com/cinc-project/cinc-collector/internal/metrics"
	"github.com/cinc-project/cinc-collector/internal/storage"
	"github.com/cinc-project/cinc-collector/internal/store"
)

// Pinger is satisfied by store.DB; declared as an interface here so tests
// can fake it without a real database.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Options configures the API.
type Options struct {
	DB            Pinger
	Tokens        *store.TokenStore
	Nodes         *store.NodeStore
	Runs          *store.RunStore
	Reports       *store.ReportStore
	Profiles      *store.ProfileStore
	Blobs         storage.Backend
	Metrics       *metrics.Registry
	StatsUser     string
	StatsPassword string
	BodyLimit     int64
	Now           func() time.Time
}

// API holds the dependencies for all HTTP handlers and builds the final
// http.Handler.
type API struct {
	opts     Options
	draining atomic.Bool
	auth     *auth.Middleware
}

// New constructs an API from Options.
func New(opts Options) *API {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	a := &API{opts: opts}
	if opts.Tokens != nil {
		a.auth = auth.NewMiddleware(opts.Tokens)
	}
	return a
}

// Drain marks the API as draining: /_status starts returning 503 so a load
// balancer stops routing new traffic, while in-flight and already-accepted
// requests continue to be served normally.
func (a *API) Drain() { a.draining.Store(true) }

// Handler assembles the full middleware chain and route table.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	a.registerSystemRoutes(mux)
	a.registerIngestRoutes(mux)
	a.registerQueryRoutes(mux)
	a.registerProfileRoutes(mux)

	h := withJSONErrors(mux)
	h = a.instrument(h)
	if a.opts.BodyLimit > 0 {
		h = limitBody(h, a.opts.BodyLimit)
	}
	return h
}

func limitBody(next http.Handler, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
