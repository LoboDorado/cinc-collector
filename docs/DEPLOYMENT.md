# Deployment

## Topology

Cinc Collector's app tier is stateless: run N replicas behind a load balancer. This is the opposite of cinc-server-ng's own deployment guidance ("put one active instance behind a load balancer, never several") — that's safe here specifically because all durable state lives in SereneDB and blob storage, not in-process.

```
                     ┌──────────────┐
  chef-client / ──►  │ load balancer│ ──► cinc-collector (N replicas, stateless)
  InSpec / Console   └──────────────┘            │
                                                   ▼
                                     SereneDB (single-node)   blob storage (S3/MinIO-alike or local disk)
```

## Database: SereneDB

**Run single-node for now.** SereneDB's own README claims "horizontal scalability, high availability... for production deployments," but as of this writing there is no published HA/clustering/failover/read-replica documentation, and a separate line in the README ("the complete single-node version is and always will be Apache 2.0") suggests a future multi-node capability may be a paid tier rather than something available for free. Re-evaluate this before depending on SereneDB for a deployment that needs automatic database failover; until then, treat a SereneDB outage as something that needs a backup/restore response, not automatic failover.

### Backup/restore

SereneDB's backup story is more limited than plain PostgreSQL's:

- **Online file-copy backup**: `pg_backup_start()` / `pg_backup_stop()`, which lets you copy the data directory while the server keeps serving reads and writes.
- **Volume snapshots** of the underlying disk work.
- **pgBackRest** works, but for **full backups only** — no incremental backups, no point-in-time recovery.
- **No WAL archiving**, and `pg_dump` is not a supported backup path.

Plan your RPO/RTO around full-backup-or-snapshot recovery, not point-in-time recovery.

### Tuning

SereneDB's full-text search is IResearch-backed. On startup it warns if `vm.max_map_count` is below 262144 ("large segment counts may fail to mmap"); set it on the host:

```sh
sudo sysctl -w vm.max_map_count=262144
```

### Search consistency

The inverted index backing `GET /api/v0/compliance/search` is near-real-time, not immediately consistent (a default `refresh_interval` of about 1 second). A compliance report ingested and immediately searched for may not appear in search results for up to roughly a second. Design any "just happened" UI around this (e.g. polling briefly, or reading the node/report detail endpoints directly rather than search, right after ingest).

## Blob storage

- **Local filesystem** (`-profile-storage=local`, the default): fine for a single-node/dev deployment; durability is whatever the underlying disk/volume provides.
- **S3-compatible** (`-profile-storage=s3`): point `-s3-bucket`/`-s3-endpoint`/`-s3-region` at AWS S3 or any S3-compatible service you self-host. Credentials come from the AWS SDK's default credential chain (environment variables, shared config, IAM role) — there are no custom credential flags.

Note: MinIO's Community Edition stopped publishing usable public Docker images at some point in 2026 (Community Edition became source-distribution-only); it is not bundled in `dev/docker-compose.yml` for this reason. If you want to exercise the S3 backend locally, point `-s3-endpoint` at whatever S3-compatible service you have available.

## TLS

`-tls-cert`/`-tls-key` enable TLS termination directly in the binary. Otherwise, terminate TLS at a reverse proxy/load balancer in front of it, same as you would for cinc-server-ng.

## Health checks and graceful shutdown

- `GET /_status` is the readiness/liveness probe: 200 normally, 503 while draining or if SereneDB is unreachable.
- On `SIGTERM`/`SIGINT`, the process calls `Drain()` (flips `/_status` to 503 and disables keep-alives) then, after `-shutdown-delay`, `Stop()` (graceful HTTP shutdown bounded by `-shutdown-timeout`). Set your orchestrator's termination grace period above `shutdown-delay + shutdown-timeout` so in-flight requests always finish before the process is killed.
- `cinc-collector healthcheck` self-probes `/_status` and exits non-zero on failure — used as the Docker `HEALTHCHECK` and usable as a Kubernetes exec probe too, since the distroless runtime image has no shell/curl.

## Migrations

Migrations are **not** applied automatically on boot (unlike cinc-server-ng's SQLite backend, which is safe to auto-migrate because it's single-process). Run `cinc-collector migrate -db-dsn "$DSN"` as an explicit step before rolling out a new version — a CI job, a Kubernetes `Job`/init container, or by hand — never let replicas race to migrate concurrently.
