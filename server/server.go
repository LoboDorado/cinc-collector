package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/cinc-project/cinc-collector/internal/api"
	"github.com/cinc-project/cinc-collector/internal/metrics"
	"github.com/cinc-project/cinc-collector/internal/storage"
	"github.com/cinc-project/cinc-collector/internal/storage/local"
	"github.com/cinc-project/cinc-collector/internal/store"
)

// Server is a running (or ready-to-start) Cinc Collector instance. New
// constructs it (opening the database, wiring storage, building the HTTP
// handler) without binding a socket; Start binds and serves.
type Server struct {
	opts    Options
	db      *store.DB
	api     *api.API
	httpSrv *http.Server
	ln      net.Listener
}

// New constructs a Server: connects to SereneDB, wires the blob storage
// backend, and builds the HTTP handler. It does not bind a listener or
// start serving; call Start for that.
func New(ctx context.Context, opts Options) (*Server, error) {
	opts = opts.withDefaults()

	db, err := store.Open(ctx, opts.DB.DSN, opts.DB.MaxConns)
	if err != nil {
		return nil, fmt.Errorf("server: open database: %w", err)
	}

	blobs, err := newBlobBackend(ctx, opts.Storage)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("server: init blob storage: %w", err)
	}

	a := api.New(api.Options{
		DB:            db,
		Tokens:        store.NewTokenStore(db),
		Nodes:         store.NewNodeStore(db),
		Runs:          store.NewRunStore(db),
		Reports:       store.NewReportStore(db),
		Profiles:      store.NewProfileStore(db),
		Blobs:         blobs,
		Metrics:       metrics.NewRegistry(),
		StatsUser:     opts.StatsUser,
		StatsPassword: opts.StatsPassword,
		BodyLimit:     opts.BodyLimit,
		Now:           opts.Now,
	})

	return &Server{opts: opts, db: db, api: a}, nil
}

func newBlobBackend(ctx context.Context, opts StorageOptions) (storage.Backend, error) {
	switch opts.Backend {
	case "", "local":
		return local.New(opts.LocalPath)
	case "s3":
		return newS3Backend(ctx, opts)
	default:
		return nil, fmt.Errorf("unknown storage backend %q", opts.Backend)
	}
}

// Start binds the configured listen address and begins serving in the
// background. It returns once the listener is bound.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.opts.ListenAddr)
	if err != nil {
		return fmt.Errorf("server: listen: %w", err)
	}
	s.ln = ln

	s.httpSrv = &http.Server{
		Handler:           s.api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		var serveErr error
		if s.opts.TLSCertFile != "" && s.opts.TLSKeyFile != "" {
			s.httpSrv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			serveErr = s.httpSrv.ServeTLS(ln, s.opts.TLSCertFile, s.opts.TLSKeyFile)
		} else {
			serveErr = s.httpSrv.Serve(ln)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Error("http server stopped unexpectedly", "error", serveErr)
		}
	}()

	return nil
}

// Addr returns the bound listener's address. Only valid after Start.
func (s *Server) Addr() net.Addr {
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Drain marks the server as draining: /_status starts returning 503 (so a
// load balancer stops routing new traffic) while in-flight and
// already-accepted requests continue to be served. Call this before Stop,
// with a delay in between, for a zero-dropped-request rolling restart.
func (s *Server) Drain() {
	s.api.Drain()
	s.httpSrv.SetKeepAlivesEnabled(false)
}

// Stop gracefully shuts down the HTTP server (bounded by ctx's deadline)
// and closes the database connection pool.
func (s *Server) Stop(ctx context.Context) error {
	var err error
	if s.httpSrv != nil {
		err = s.httpSrv.Shutdown(ctx)
	}
	s.db.Close()
	return err
}
