-- +goose Up
CREATE TABLE compliance_reports (
    report_id         UUID PRIMARY KEY,
    org               TEXT NOT NULL,
    node_name         TEXT NOT NULL,
    environment       TEXT,
    status            TEXT NOT NULL,
    controls_passed   INT,
    controls_failed   INT,
    controls_skipped  INT,
    profiles          JSON,
    raw_report        JSON NOT NULL,
    -- Flattened profile/control titles and error text, built at write time
    -- in Go from `profiles`/`raw_report`. The inverted index below is
    -- declared over real columns, not JSON paths, so this column exists
    -- purely to make that text searchable.
    search_text       TEXT NOT NULL DEFAULT '',
    start_time        TIMESTAMPTZ NOT NULL,
    end_time          TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX compliance_reports_node_idx ON compliance_reports (org, node_name, end_time DESC);

-- SereneDB has no built-in "english"/"simple" text search dictionary: the
-- tokenizer pipeline is defined explicitly. `frequency, position` are
-- required for BM25 ranking (BM25() needs frequency stats).
CREATE TEXT SEARCH DICTIONARY basic_dict
    AS split_text(case := 'lower') | normalize_tokens('en_US.UTF-8', accent := false)
    WITH (frequency, position);

-- Query this via `SELECT ... FROM compliance_reports_search WHERE col @@ '...'`
-- (selecting FROM the index name, not the base table) -- see docs/specs for
-- why. The index-as-relation still exposes every column of
-- compliance_reports, including ones not listed here, so ordinary filters
-- (status, date range) combine with the @@ predicate in the same query.
CREATE INDEX compliance_reports_search ON compliance_reports
    USING inverted (node_name basic_dict, search_text basic_dict);

-- +goose Down
DROP INDEX compliance_reports_search;
DROP TEXT SEARCH DICTIONARY basic_dict;
DROP TABLE compliance_reports;
