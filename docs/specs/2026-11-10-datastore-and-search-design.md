# Datastore and search design

Status: Accepted
Date: 2026-11-10

## Summary

Cinc Collector needs to store converge run and compliance report history queryably, and give Cinc Console real search over it, while staying "as lightweight as possible" (a workstation-friendly local dev story, not a cluster of services) and HA/DR-capable in production. This document records the evaluation trail that landed on SereneDB as the sole datastore and search index, the real compatibility gaps found by spiking against it, and the known risk being accepted deliberately.

## Goals

- One datastore, no separately-operated search engine process.
- Real full-text/relevance-ranked search for Cinc Console (not just exact-match filters).
- A permissive license consistent with the rest of the Cinc project (Apache-2.0).
- A clear path back to a more conservative design if the chosen technology doesn't work out.

## Non-goals

- Picking a final production HA/DR story for the datastore right now. The project is early (not yet "prod worthy," in the words of the person who approved this choice), and this is explicitly deferred — see "Known risk" below.

## Background: the evaluation trail

1. **ParadeDB (`pg_search`)** — a Postgres extension providing a BM25 full-text index (Tantivy/IResearch-style, via `pgrx`). Rejected: its Community edition is AGPLv3 (incompatible with this project's Apache-2.0 posture), and critically, Community edition does not replicate its BM25 index to standby replicas in a multi-instance cluster — that requires paid ParadeDB Enterprise. Adopting it would mean either paying for Enterprise or accepting no real HA for search ever.
2. **Plain PostgreSQL** (`tsvector`/GIN + `pg_trgm`) — the conservative fallback. Zero licensing questions, works on any managed/self-hosted Postgres, inherits whatever HA/DR solution is chosen for Postgres itself with no compatibility questions. Seriously considered; not chosen because a genuinely better option existed.
3. **SereneDB** (chosen) — Apache-2.0, speaks the PostgreSQL wire protocol, and ships a native inverted full-text/BM25 index as a first-class engine feature, not a bolted-on extension. Other alternatives considered and rejected along the way: Apache Solr/Vespa (Apache-2.0 but meaningfully more operational burden — Solr needs its own ZooKeeper ensemble — for no corresponding benefit here); Quickwit (AGPL, and not designed for frequently-updated data); OpenSearch (Apache-2.0, mature, and notably what real Chef Automate itself uses for this exact use case — kept as the documented fallback, not used in v1, since it's a second stateful distributed system to operate).

SereneDB was chosen deliberately as a bet on young technology, not because its maturity was mistaken for more than it is: its first "production-ready" release shipped only weeks before this decision, from a small team. That bet was made explicitly because this project is early enough to absorb the risk, with a clear mental model of what would need to change if it doesn't pan out (see "Fallback plan").

## SereneDB compatibility, confirmed by spiking against a real `serenedb/serenedb` container

This is not inferred from documentation; every item below was verified directly with `psql` against SereneDB 26.11.2, because the documentation alone was insufficient and in one case (the `@@` match syntax) actively misleading without it.

- **No `jsonb`.** `CREATE TABLE ... (data JSONB)` fails outright: "Type with name jsonb does not exist." Use `json`: it works fully with `->`/`->>`/`#>`/`#>>`. `jsonb_*` functions and jsonb-only operators (`@>`, `?`, `?|`, `?&`) are unavailable.
- **No GIN/BRIN indexes** (`CREATE INDEX ... USING GIN` fails: "Unknown index type: gin"), **no `pg_trgm`, no `CREATE EXTENSION` at all.**
- **Full-text search is a native `inverted` index**, and using it correctly requires three things the documentation alone didn't make obvious:
  1. A text search dictionary must be defined explicitly — there is no built-in "english"/"simple" dictionary. Example: `CREATE TEXT SEARCH DICTIONARY basic_dict AS split_text(case := 'lower') | normalize_tokens('en_US.UTF-8', accent := false) WITH (frequency, position);`
  2. The index is then created `USING inverted (col1 <dict>, col2 <dict>, ...)`.
  3. **Queries select `FROM` the index name, not the base table**: `SELECT ... FROM <index_name> WHERE col @@ 'text' ORDER BY BM25(<index_name>.tableoid) DESC;`. Querying `FROM` the base table with the same `WHERE` clause fails at runtime with an opaque error ("TSQUERY expression evaluated outside an `@@` match against an inverted-indexed column") that doesn't mention the real cause. The index-as-relation exposes every column of the underlying table, not just the indexed ones, so ordinary filters (status, date range) combine with the `@@` predicate in the same query.
  4. **The index is near-real-time, not immediately consistent**: a row visible to an ordinary `SELECT` immediately after insert is not necessarily visible to a `@@` match against it until the index's `refresh_interval` elapses (defaulted to `1000` ms in this project's schema). Confirmed by a test that initially failed intermittently until it was changed to poll.
- **No `ALTER TABLE ... ADD FOREIGN KEY`** ("No support for adding FOREIGN_KEY constraints with ALTER TABLE"). Foreign keys must be declared inline in the original `CREATE TABLE`, fixing migration ordering.
- **`gen_random_uuid()` works as a built-in function**, no extension needed (an earlier assumption that it might be named `get_random_uuid()` was wrong).
- **goose's own bootstrap DDL is incompatible**: its default version-tracking table uses `id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY`, which SereneDB rejects ("syntax error at or near GENERATED"). Worked around in `internal/store/migrate.go` by pre-creating that table with `SERIAL` instead, which goose's own bootstrap logic tolerates (it only creates the table if a `SELECT` against it fails first).
- **Backup/DR is more limited than plain PostgreSQL**: `pg_backup_start`/`pg_backup_stop` and volume snapshots work; pgBackRest works for full backups only (no point-in-time recovery); there is no WAL archiving and `pg_dump` is not a supported backup path.
- `TIMESTAMPTZ`, `now()`, `BEGIN`/`COMMIT`/`RETURNING`, and `INSERT ... ON CONFLICT ... DO UPDATE` all work exactly as in PostgreSQL.
- `bytea` is supported (behaves like PostgreSQL), but SereneDB's own documentation and its one S3-related feature (indexing existing Parquet/Iceberg/CSV/JSON data lake files in place) are oriented entirely around search/analytics over structured data, with no stated guidance or storage-engine detail suggesting it's built to hold many binary blobs at the sizes InSpec profile tarballs need. This is why profile tarballs live in external blob storage (`internal/storage`) with only metadata in SereneDB, not the other way around.

## Decision

Ship with SereneDB as the sole datastore and its native inverted index as the only search mechanism. The query API (`internal/api`'s query handlers) calls through `internal/store`'s `ReportStore.Search` rather than embedding raw SQL search queries inline, so the underlying search mechanism can be swapped later without touching handler code.

## Known risk, explicitly accepted

SereneDB's GitHub README claims "horizontal scalability, high availability... for production deployments," but no HA, clustering, failover, or read-replica documentation exists as of this decision, and a separate line in the README ("the complete single-node version is and always will be Apache 2.0") echoes ParadeDB's Community/Enterprise split closely enough to be a real concern: a future multi-node capability may turn out to be a paid tier, not something this project gets for free. Run single-node SereneDB in both dev and production for now; re-resolve this — what SereneDB's HA actually looks like, and whether it's free — before depending on it for a deployment that needs automatic database failover.

## Fallback plan

If SereneDB's HA story doesn't mature, or its dialect gaps prove too costly to keep working around: migrate `internal/store`'s SQL to plain PostgreSQL dialect (mechanically straightforward — `json` already works as `jsonb` would, the inline-FK and `ON CONFLICT` patterns are identical, the inverted-index-backed `Search` method is the one piece that would need a real rewrite, most likely to PostgreSQL `tsvector`/GIN/`pg_trgm`, or to an OpenSearch-backed implementation behind the same `ReportStore.Search` interface). The schema and data-access layer were designed with this portability in mind from the start, specifically because this was a known possibility when SereneDB was chosen.
