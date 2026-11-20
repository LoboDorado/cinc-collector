# Ingest API design

Status: Accepted
Date: 2026-11-10

## Summary

`cinc-server-ng`'s data-collector reverse proxy is a pure byte-forwarder: it validates nothing about the message body, it only forwards bytes to a configured downstream URL. That makes Cinc Collector responsible for defining and owning the actual message contract — there is no existing Go type anywhere to reuse for this. This document records where that contract came from and the design decisions around accepting it.

## Goals

- Accept exactly what real chef-client and InSpec's `automate` reporter plugin actually send, not an approximation reconstructed from prose documentation.
- Handle the fact that compliance reports and chef-client messages don't share a consistent identity/scoping model.

## Non-goals

- Full fidelity with every field Chef Automate itself might store (deprecations, full node document, full resource diff). Those are kept opaquely as `json`/`JSON` blobs (`RawMessage`/`RawReport` columns) rather than modeled field-by-field, so nothing is lost even though not everything is indexed.

## Where the schema came from

The exact field lists for `run_start`, `run_converge`, and the InSpec `automate` reporter's compliance report payload (`internal/automate/messages.go`) were read directly from source, not reconstructed from memory or from Chef's own (partially outdated) RFC/design-doc prose:

- `chef/chef`'s `lib/chef/data_collector/run_start_message.rb`, `run_end_message.rb`, and `message_helpers.rb`.
- `inspec/inspec`'s `lib/inspec/reporters/automate.rb`, `json.rb`, and `json_automate.rb`.

Notable findings from reading the actual source rather than guessing:

- `run_converge`'s `"duration"` and `"delta"` fields on each resource are **strings**, not numbers (e.g. `"12"`, milliseconds).
- `run_start`'s `entity_uuid` prefers `node["uuid"]`, while `run_converge`'s unconditionally uses `Chef::Config[:chef_guid]` — a real, if minor, inconsistency in the upstream source, not a bug introduced here.
- **InSpec's compliance report payload carries no organization field at all.** Chef Automate's own data model is not org-scoped the way Chef Infra Server is; `node_name`/`environment` are optional passthrough-config fields, not something always present. This has a direct schema consequence — see below.

## Design decision: org scoping via the authenticated token, not the message body

Every table in this project's schema is scoped by `(org, ...)`, matching a multi-tenant Chef Infra Server setup where Cinc Console may be managing multiple orgs via `cinc-server-ng`. `run_start`/`run_converge` messages carry `organization_name` directly, so this is straightforward for chef-client traffic. Compliance reports don't carry anything equivalent.

Two paths were considered:

1. **Path-based org routing** (`POST /data-collector/v0/{org}`) — rejected. `cinc-server-ng`'s reverse proxy discards the incoming request path/query entirely and always forwards to one fixed configured `--data-collector-url` for its whole process, regardless of org. A per-org URL path would never actually be reached for chef-client traffic relayed through it.
2. **A token's home org** (chosen) — each bearer token (`tokens.org`, nullable) carries an optional home org. `internal/auth.Middleware` resolves it and stashes it in the request context (`auth.TokenOrg`); `internal/api/ingest.go`'s compliance-report handler uses it as the only source of org scoping, and the chef-client handlers prefer the message's own `organization_name` but fall back to it. This means an InSpec CI pipeline (or `cinc-server-ng`'s own relay token) is configured with its org once, at token-creation time, rather than needing the org threaded through every single request.

## Status derivation for compliance reports

InSpec's report format has no single top-level "status" field for the whole report. `internal/automate.ComplianceReport.Counts()` rolls control-level outcomes up the same way InSpec's own CLI summary does: a control counts as failed if any of its results failed, skipped if every result was skipped, passed otherwise. The report's overall `status` column is then: `failed` if any control failed, `skipped` if every control was skipped and none passed, `passed` otherwise.

## Search text

The native inverted index (see the datastore design doc) must be declared over real text columns, not JSON paths. `compliance_reports.search_text` is built at ingest time, in Go (`ComplianceReport.SearchText()`), from profile/control titles, descriptions, and failure messages — not computed in SQL — because that keeps the flattening logic in one place, in a language with real test coverage, rather than duplicated across a SQL expression and application code.
