# Development

## Setup

You need Go (see `go.mod` for the version) and Docker (for the local SereneDB instance). No other local services are required: profile storage defaults to the local filesystem.

```sh
make dev-up       # starts SereneDB in Docker (dev/docker-compose.yml)
make dev-migrate  # applies internal/store/migrations/*.sql
make run          # runs cinc-collector against it on :8891
```

`make dev-down` tears the stack down; `docker compose -f dev/docker-compose.yml down -v` also removes its volume if you want a truly clean slate.

## Build / test / lint

```sh
make build   # ./bin/cinc-collector
make test    # go test ./... -race -cover, no external services required
make vet
make fmt
```

Tests that touch a real database are gated behind `CINC_COLLECTOR_TEST_DB_DSN` and skip themselves otherwise, so `make test` alone needs nothing running. To run them locally:

```sh
make dev-up
CINC_COLLECTOR_TEST_DB_DSN="postgres://postgres:postgres@localhost:7890/postgres?sslmode=disable" \
  go test ./internal/store/... -race -cover
```

CI runs both: it provisions a SereneDB service container and sets that env var itself.

## Working against SereneDB

See `CLAUDE.md`'s "Working against SereneDB, not plain PostgreSQL" section before writing a migration or a query. The short version: no `jsonb`, no GIN/`pg_trgm`, full-text search is a native `inverted` index you query by selecting `FROM` the index name, and foreign keys must be declared inline at `CREATE TABLE` time.

A quick way to poke at it directly:

```sh
docker compose -f dev/docker-compose.yml exec serenedb psql -h localhost -p 7890 -U postgres -d postgres
```

## Issuing yourself a token

There's no admin UI; the CLI is the only way to provision bearer tokens:

```sh
go run ./cmd/cinc-collector token create -db-dsn "$DSN" -name my-client -org acme
go run ./cmd/cinc-collector token list -db-dsn "$DSN"
go run ./cmd/cinc-collector token revoke -db-dsn "$DSN" -name my-client
```

The plaintext token is only ever shown once, at creation.

## Building the Docker image locally

`vendor/` is not committed to this repo (see "Why vendoring" below), but the `Dockerfile` builds with `-mod=vendor` so it doesn't need network access from inside the build container. Before `docker build .`/`docker compose build` locally, run:

```sh
make tidy   # go mod tidy && go mod vendor
```

CI's release workflow does this automatically; it's only a manual step for local builds.

### Why vendoring

This matters more than it might on a typical project: if you're on a network that proxies/inspects outbound TLS at the host level (e.g. Zscaler) but doesn't extend that into Docker's container network namespace, `RUN go mod download` inside a `docker build` will hang or fail with TLS errors even though `go build` on the host works fine. Vendoring sidesteps this by making the build need no network access at all. If you don't have this problem, you can ignore `make tidy` entirely for local `go build`/`go test`/`go run` — it's only needed before a local `docker build`.

## Running the whole stack in containers

The default `make dev-up` only starts SereneDB — the collector itself is meant to run natively on the host (`make run`) for the fast edit/rerun loop. If you want to exercise the actual container image instead (closer to how it runs in production), `dev/docker-compose.yml` also has `migrate` (a one-shot job, matching the "migrations are a deliberate, separate step" rule in `CLAUDE.md` — not auto-run on boot) and `collector` services:

```sh
make tidy              # vendor/ must exist and be current before a Docker build
make dev-collector-up  # docker compose build + up: serenedb -> migrate -> collector, on :8891
make dev-collector-logs
```

`collector` only starts after `migrate` exits successfully, and `serenedb` only reports healthy once `pg_isready` succeeds inside the container. `make dev-down` tears all three down.

## Testing against real Chef/InSpec payloads

`internal/automate/testdata/*.json` are real-shaped fixtures for `run_start`, `run_converge`, and InSpec compliance report messages, built from the actual field lists in `chef/chef`'s `lib/chef/data_collector/*.rb` and `inspec/inspec`'s `lib/inspec/reporters/*.rb` source. If you add a new field or message variant, go back to that source rather than guessing the shape, and add/update a fixture.
