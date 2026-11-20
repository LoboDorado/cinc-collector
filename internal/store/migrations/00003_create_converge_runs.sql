-- +goose Up
CREATE TABLE converge_runs (
    run_id                  UUID PRIMARY KEY,
    org                     TEXT NOT NULL,
    node_name               TEXT NOT NULL,
    status                  TEXT NOT NULL,
    start_time              TIMESTAMPTZ NOT NULL,
    end_time                TIMESTAMPTZ,
    total_resource_count    INT,
    updated_resource_count  INT,
    run_list                JSON,
    resources               JSON,
    error                   JSON,
    raw_message             JSON NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX converge_runs_node_idx ON converge_runs (org, node_name, start_time DESC);

-- +goose Down
DROP TABLE converge_runs;
