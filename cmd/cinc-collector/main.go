// Command cinc-collector is the Cinc Collector binary: a Chef
// Automate-compatible data-collector backend, query API for Cinc Console,
// and InSpec profile store, backed by SereneDB.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cinc-project/cinc-collector/internal/store"
	"github.com/cinc-project/cinc-collector/server"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out *os.File) error {
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-version":
			_, _ = fmt.Fprintf(out, "cinc-collector %s (commit %s, built %s)\n", version, commit, buildDate)
			return nil
		case "healthcheck":
			return runHealthcheck(args[1:])
		case "migrate":
			return runMigrate(args[1:])
		case "token":
			return runToken(args[1:])
		case "serve":
			args = args[1:] // fall through to flag parsing below
		}
	}

	opts, err := parseServeFlags(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv, err := server.New(ctx, opts)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	if err := srv.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	slog.Info("cinc-collector started", "version", version, "addr", srv.Addr().String())

	<-ctx.Done()
	slog.Info("shutting down")

	srv.Drain()
	if opts.ShutdownDelay > 0 {
		time.Sleep(opts.ShutdownDelay)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
	defer cancel()
	return srv.Stop(shutdownCtx)
}

func parseServeFlags(args []string) (server.Options, error) {
	fs := flag.NewFlagSet("cinc-collector", flag.ContinueOnError)

	listenAddr := fs.String("listen-addr", envOr("CINC_COLLECTOR_LISTEN_ADDR", ":8891"), "address to listen on")
	tlsCert := fs.String("tls-cert", envOr("CINC_COLLECTOR_TLS_CERT", ""), "TLS certificate file (enables TLS with -tls-key)")
	tlsKey := fs.String("tls-key", envOr("CINC_COLLECTOR_TLS_KEY", ""), "TLS key file (enables TLS with -tls-cert)")
	dbDSN := fs.String("db-dsn", envOr("CINC_COLLECTOR_DB_DSN", ""), "SereneDB connection string (Postgres wire protocol)")
	dbMaxConns := fs.Int("db-max-conns", 10, "maximum database connections")
	profileStorage := fs.String("profile-storage", envOr("CINC_COLLECTOR_PROFILE_STORAGE", "local"), `blob storage backend for InSpec profiles: "local" or "s3"`)
	profileStoragePath := fs.String("profile-storage-path", envOr("CINC_COLLECTOR_PROFILE_STORAGE_PATH", "./data/profiles"), "local backend: directory to store profile tarballs in")
	s3Bucket := fs.String("s3-bucket", envOr("CINC_COLLECTOR_S3_BUCKET", ""), "s3 backend: bucket name")
	s3Endpoint := fs.String("s3-endpoint", envOr("CINC_COLLECTOR_S3_ENDPOINT", ""), "s3 backend: custom endpoint (for MinIO)")
	s3Region := fs.String("s3-region", envOr("CINC_COLLECTOR_S3_REGION", ""), "s3 backend: region")
	statsUser := fs.String("stats-user", envOr("CINC_COLLECTOR_STATS_USER", ""), "username for GET /_stats basic auth")
	bodyLimit := fs.Int64("body-limit", 64<<20, "maximum request body size in bytes")
	shutdownDelay := fs.Duration("shutdown-delay", 0, "delay between draining and shutting down, for load balancers to notice")
	shutdownTimeout := fs.Duration("shutdown-timeout", 30*time.Second, "time allowed for in-flight requests to finish during shutdown")

	if err := fs.Parse(args); err != nil {
		return server.Options{}, err
	}

	// Kept off the flag default and --help output deliberately, matching
	// cinc-server-ng's handling of its own data-collector token: a secret
	// should never show up in --help, nor need to appear on the command
	// line where `ps` could see it.
	statsPassword := os.Getenv("CINC_COLLECTOR_STATS_PASSWORD")

	if *dbDSN == "" {
		return server.Options{}, fmt.Errorf("-db-dsn (or CINC_COLLECTOR_DB_DSN) is required")
	}

	return server.Options{
		ListenAddr:  *listenAddr,
		TLSCertFile: *tlsCert,
		TLSKeyFile:  *tlsKey,
		DB: server.DBOptions{
			DSN:      *dbDSN,
			MaxConns: int32(*dbMaxConns),
		},
		Storage: server.StorageOptions{
			Backend:    *profileStorage,
			LocalPath:  *profileStoragePath,
			S3Bucket:   *s3Bucket,
			S3Endpoint: *s3Endpoint,
			S3Region:   *s3Region,
		},
		StatsUser:       *statsUser,
		StatsPassword:   statsPassword,
		BodyLimit:       *bodyLimit,
		ShutdownDelay:   *shutdownDelay,
		ShutdownTimeout: *shutdownTimeout,
	}, nil
}

func runHealthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	addr := fs.String("addr", "http://localhost:8891/_status", "address to probe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return probeStatus(*addr)
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	dsn := fs.String("db-dsn", envOr("CINC_COLLECTOR_DB_DSN", ""), "SereneDB connection string")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dsn == "" {
		return fmt.Errorf("-db-dsn (or CINC_COLLECTOR_DB_DSN) is required")
	}
	return store.Migrate(*dsn)
}

func runToken(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cinc-collector token <create|list|revoke> [flags]")
	}

	fs := flag.NewFlagSet("token", flag.ContinueOnError)
	dsn := fs.String("db-dsn", envOr("CINC_COLLECTOR_DB_DSN", ""), "SereneDB connection string")
	name := fs.String("name", "", "token name")
	org := fs.String("org", "", "token's home org (used to scope compliance reports, which carry no org of their own)")
	sub, rest := args[0], args[1:]
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if *dsn == "" {
		return fmt.Errorf("-db-dsn (or CINC_COLLECTOR_DB_DSN) is required")
	}

	ctx := context.Background()
	db, err := store.Open(ctx, *dsn, 2)
	if err != nil {
		return err
	}
	defer db.Close()
	tokens := store.NewTokenStore(db)

	switch sub {
	case "create":
		if *name == "" {
			*name = randomName()
		}
		token, err := tokens.Create(ctx, *name, *org)
		if err != nil {
			return err
		}
		fmt.Printf("created token %q: %s\n(this value is shown once; store it now)\n", *name, token)
		return nil
	case "list":
		list, err := tokens.List(ctx)
		if err != nil {
			return err
		}
		for _, t := range list {
			status := "active"
			if t.RevokedAt != nil {
				status = "revoked"
			}
			fmt.Printf("%s\t%s\torg=%s\t%s\tcreated=%s\n", t.ID, t.Name, t.Org, status, t.CreatedAt.Format(time.RFC3339))
		}
		return nil
	case "revoke":
		if *name == "" {
			return fmt.Errorf("-name is required")
		}
		return tokens.Revoke(ctx, *name)
	default:
		return fmt.Errorf("unknown token subcommand %q", sub)
	}
}

func randomName() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return "token-" + hex.EncodeToString(buf)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
