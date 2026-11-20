# cinc-collector

Cinc Collector is a Chef Automate-compatible data-collector backend, written in Go. It accepts chef-client converge reports and InSpec compliance scan reports, stores them queryably, serves that data to Cinc Console, and acts as a store for InSpec profiles that Cinc Auditor and chef-client pull for compliance runs.

It is designed the same way `cinc-server-ng` (its sibling project) is: a single Go binary, minimal dependencies, and the same code path for a developer's workstation and a horizontally-scaled production deployment.

## Why Cinc Collector

`cinc-server-ng` can reverse-proxy chef-client converge reports and InSpec compliance-scan reports to a downstream data-collector service, the same way Chef Infra Server's `data_collector` configuration does. It never stores this data itself, it only forwards it. Cinc Collector is that downstream service.

## Architecture

- **One datastore**: [SereneDB](https://serenedb.com), an Apache-2.0, Postgres-wire-protocol-compatible database with a native full-text/BM25 search index built in. No separate search engine to operate. See `docs/specs/` for why this was chosen over plain PostgreSQL and over ParadeDB.
- **Stateless app tier**: the collector binary holds no state of its own, so N replicas can run behind a load balancer for horizontal scaling and rolling deploys.
- **Pluggable blob storage** for InSpec profile tarballs: local filesystem for dev, S3-compatible (AWS S3 or MinIO-alike) for production.
- **A single bearer-token auth model** across every API surface: ingest, query, and the profile store.

See `CLAUDE.md` for a deeper architecture walkthrough and `docs/specs/` for the design decisions behind the above.

## Quickstart

```sh
make dev-up       # starts a local SereneDB in Docker
make dev-migrate  # applies the schema
make run          # starts cinc-collector against it, on :8891
```

Then, in another terminal:

```sh
go run ./cmd/cinc-collector token create -db-dsn "postgres://postgres:postgres@localhost:7890/postgres?sslmode=disable" -name my-client -org acme
```

Use the printed token as a bearer token against the API below.

## API

- `POST /data-collector/v0` - Chef Automate-compatible ingest endpoint. Accepts chef-client `run_start`/`run_converge` messages and InSpec `automate`-reporter compliance reports. Requires a bearer token.
- `GET /api/v0/nodes`, `/api/v0/nodes/{name}`, `/api/v0/nodes/{name}/runs`, `/api/v0/nodes/{name}/compliance`, `/api/v0/runs/{run_id}`, `/api/v0/compliance/reports/{report_id}`, `/api/v0/compliance/search?q=...` - the query API Cinc Console uses. Requires a bearer token.
- `POST /api/v0/profiles?name=...&version=...` (body: the tarball), `GET /api/v0/profiles`, `GET /api/v0/profiles/{name}/{version}/tar`, `DELETE /api/v0/profiles/{name}/{version}` - the InSpec profile store. Requires a bearer token.
- `GET /_status` - unauthenticated health/readiness probe.
- `GET /_stats` - Prometheus-format metrics, behind HTTP Basic auth (`-stats-user` / `CINC_COLLECTOR_STATS_PASSWORD`).

Every error response is JSON: `{"error": ["message", ...]}`.

### Auth

Every API surface shares one bearer-token model, but accepts it over two different header names depending on the caller: the query API and profile store (cinc-dashboard's own client) use `Authorization: Bearer <token>`; the ingest endpoint also accepts `X-Data-Collector-Token: <token>`, since neither of its real callers -- chef-client's `data_collector` handler (reaching this endpoint via cinc-server-ng's reverse proxy, which injects this header) nor InSpec's own `automate` reporter (which sends it directly) -- can be reconfigured to send `Authorization: Bearer` instead. There are no scopes; any valid token can ingest, query, and manage profiles. A token has an optional home org, used to scope requests that don't carry an org of their own (notably InSpec compliance reports, which Chef Automate's own wire format doesn't org-scope at all). Manage tokens with:

```sh
cinc-collector token create -name <name> -org <org>
cinc-collector token list
cinc-collector token revoke -name <name>
```

## Configuration

No config file: flags with `CINC_COLLECTOR_*` environment variable fallbacks, matching cinc-server-ng's own approach. Run `cinc-collector -h` for the full list. Secrets (`CINC_COLLECTOR_STATS_PASSWORD`) are read from the environment only, never from a flag, so they never show up in `--help` output or a process listing.

## Development

See `docs/DEVELOPMENT.md`.

## Deployment / HA / DR

See `docs/DEPLOYMENT.md`.

## License

Apache-2.0, matching cinc-server-ng and the rest of the Cinc project. Note that SereneDB (the datastore Cinc Collector depends on) is itself Apache-2.0 for its Community edition; see `docs/specs/` for the full licensing trail considered when choosing it.
