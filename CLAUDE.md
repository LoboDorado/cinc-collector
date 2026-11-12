# CLAUDE.md

Architecture and conventions reference for agents (and humans) working on Cinc Collector. Read `docs/specs/` for the full reasoning behind the decisions summarized here.

## What this is

A Chef Automate-compatible data-collector backend: it accepts chef-client converge reports and InSpec compliance reports over HTTP, stores them in SereneDB, serves them to Cinc Console's query API, and stores/serves InSpec profile tarballs. It is modeled on the sibling project `cinc-server-ng`'s engineering conventions (Go idioms, stdlib-first, minimal dependencies, JSON error shape, graceful drain) but diverges where `cinc-server-ng` is explicitly single-node/non-HA and this project is not: see "Divergences from cinc-server-ng" below.

## Request flow

```text
cmd/cinc-collector (flag parsing, signals)
  └─ server.New(Options)        server/        - connects to SereneDB, wires blob storage, builds the API
       limitBody                  internal/api - caps request body size
         └─ api.Instrument-equivalent (not yet built, see internal/metrics)
              └─ auth.Middleware.Require        internal/auth - bearer token, stashes org in context
                   └─ mux                       internal/api/api.go - http.ServeMux, one handler set per surface
```

## Package responsibilities

- `cmd/cinc-collector` - CLI entrypoint: `serve` (default), `migrate`, `token create|list|revoke`, `version`, `healthcheck`. No cobra; hand-dispatch on `os.Args[0]`, same as cinc-server-ng.
- `server` - the public, embeddable library package: `server.New(ctx, Options)` / `Start` / `Drain` / `Stop`. Wires the database, blob storage, and `internal/api` together.
- `internal/api` - HTTP handlers, one file per surface: `ingest.go` (data-collector), `query.go` (Cinc Console reads), `profiles.go` (InSpec profile store), `system.go` (`/_status`, `/_stats`). `api.go` assembles the mux and middleware chain.
- `internal/automate` - Chef Automate's data-collector wire format: `RunStartMessage`, `RunConvergeMessage`, `ComplianceReport`, and `Detect()` to tell them apart. These shapes were taken directly from `chef/chef`'s `lib/chef/data_collector/*.rb` and `inspec/inspec`'s `lib/inspec/reporters/{automate,json,json_automate}.rb` source, not reconstructed from memory or docs - if a field is missing or wrong, go back to that source, don't guess.
- `internal/store` - the SereneDB access layer: `pgx`/`pgxpool` over the Postgres wire protocol, hand-written SQL (no ORM), goose migrations in `migrations/`. **The SQL dialect here is SereneDB's, not plain PostgreSQL** - see the next section before writing a query or migration.
- `internal/storage` - the blob storage abstraction (`Backend` interface) for InSpec profile tarballs, with `local/` (filesystem, dev default) and `s3/` (AWS S3 or any S3-compatible service, production) implementations.
- `internal/auth` - bearer-token hashing/generation/verification. One flat model: any valid token can do anything; a token's optional `org` scopes requests (notably compliance reports, which carry no org of their own).
- `internal/metrics` - a small hand-rolled Prometheus-text-exposition registry, not the official client library, matching cinc-server-ng's own restraint.

## Working against SereneDB, not plain PostgreSQL

SereneDB speaks the Postgres wire protocol (so `pgx` and `goose` work unmodified), but its SQL dialect has real gaps, confirmed by spiking against a real `serenedb/serenedb` container (not just inferred from docs):

- **No `jsonb`.** Use `json`. `jsonb_*` functions and operators (`@>`, `?`, `?|`, `?&`) don't exist; `->`/`->>`/`#>`/`#>>` do.
- **No GIN/BRIN indexes, no `pg_trgm`, no `CREATE EXTENSION` at all.** Full-text search is a native **inverted index** instead:
  1. `CREATE TEXT SEARCH DICTIONARY <name> AS split_text(...) | normalize_tokens(...) WITH (frequency, position);` (there's no built-in "english"/"simple" dictionary - see `internal/store/migrations/00004_create_compliance_reports.sql` for the one this project defines).
  2. `CREATE INDEX <name> ON <table> USING inverted (col1 <dict>, col2 <dict>, ...);`
  3. **Query by selecting `FROM` the index name, not the base table**: `SELECT ... FROM <index_name> WHERE col @@ 'query' ORDER BY BM25(<index_name>.tableoid) DESC;`. The index-as-relation exposes every column of the underlying table, so ordinary filters combine with `@@` in the same query (see `internal/store/reports.go`'s `Search` method).
  4. **The index is near-real-time, not immediately consistent** (a default `refresh_interval` of ~1000ms): a row inserted and immediately searched for may not appear yet. Don't assume ingest-then-search is synchronous; `internal/store/store_test.go`'s `TestReportStoreSearch` polls for exactly this reason.
- **No `ALTER TABLE ... ADD FOREIGN KEY`.** Foreign keys must be declared inline in the original `CREATE TABLE`, which fixes migration ordering (a referenced table's migration must come first) and means a FK can't be retrofitted without recreating the table.
- **`gen_random_uuid()` works as a built-in**, no extension needed.
- **goose's own bootstrap is incompatible**: its default version-table DDL uses `GENERATED BY DEFAULT AS IDENTITY`, which SereneDB rejects. `internal/store/migrate.go`'s `ensureGooseVersionTable` pre-creates that table with `SERIAL` instead, which goose then treats as already-bootstrapped. Don't remove this; `goose.Up` will fail cold without it.
- `TIMESTAMPTZ`, `now()`, `BEGIN`/`COMMIT`/`RETURNING`, and `INSERT ... ON CONFLICT ... DO UPDATE` all work normally.

## Divergences from cinc-server-ng

cinc-server-ng is explicitly single-process and non-HA (SQLite backend, auto-migrate-on-every-boot, "run one instance, never several" deployment guidance). Cinc Collector needs real HA/DR, so:

- **Migrations are a separate, explicit step** (`cinc-collector migrate`), not run automatically on every boot: N replicas starting concurrently against a shared database would race if migrations auto-ran.
- **The app tier is stateless and horizontally scalable** - run N replicas behind a load balancer, which cinc-server-ng's own docs explicitly say never to do with *it*.
- Structured logging uses stdlib `log/slog` rather than cinc-server-ng's plain `fmt.Fprintf` banners, since this is meant to run as an unattended production service from day one, not just a dev convenience binary.

## Known risks, tracked deliberately

- **SereneDB's HA/clustering story is unproven.** Its README claims "high availability" but no HA/failover/clustering docs exist, and a line in it ("the complete single-node version is and always will be Apache 2.0") hints a multi-node capability may eventually be a paid tier. Run single-node for now; this needs to be re-resolved before any real production rollout. See `docs/specs/`.
- **Backup/DR is more limited than plain Postgres**: no `pg_dump`, no WAL archiving, no point-in-time recovery. Only online file-copy backup and volume snapshots.

## Testing

- `go test ./...` needs no external services by default (matches cinc-server-ng's convention) - DB-touching tests in `internal/store` skip themselves unless `CINC_COLLECTOR_TEST_DB_DSN` is set.
- `internal/automate`'s tests use real example payloads in `testdata/` as golden fixtures, built from the actual Chef/InSpec source field lists, not synthetic data.
- Local dev DB: `make dev-up && make dev-migrate`, then `CINC_COLLECTOR_TEST_DB_DSN=postgres://postgres:postgres@localhost:7890/postgres?sslmode=disable go test ./internal/store/...`.
