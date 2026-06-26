-- Per-tool detail fact tables (Postgres) — identical shape to the sqlite set;
-- keyed by call_id and carrying the lineage stamp so they join the spine.
-- Append-only (insert-only). Reserved words avoided in column names.

CREATE TABLE connector_ft_crossing (
    call_id     TEXT NOT NULL,
    sequence_id TEXT,
    run_id      TEXT,
    session_id  TEXT,
    backend     TEXT,
    target      TEXT,
    direction   TEXT,
    row_count   INTEGER,
    byte_count  INTEGER,
    query_ms    INTEGER,
    recorded_at TEXT NOT NULL
);

CREATE TABLE wuxing_ft_data_mutation (
    call_id       TEXT NOT NULL,
    sequence_id   TEXT,
    run_id        TEXT,
    session_id    TEXT,
    target        TEXT,
    rows_inserted INTEGER,
    rows_updated  INTEGER,
    rows_deleted  INTEGER,
    recorded_at   TEXT NOT NULL
);

CREATE TABLE ai_ft_call (
    call_id     TEXT NOT NULL,
    sequence_id TEXT,
    run_id      TEXT,
    session_id  TEXT,
    model       TEXT,
    mode        TEXT,
    tokens_in   INTEGER,
    tokens_out  INTEGER,
    cost        DOUBLE PRECISION,
    ttft_ms     INTEGER,
    turn_count  INTEGER,
    recorded_at TEXT NOT NULL
);

CREATE INDEX idx_crossing_sequence ON connector_ft_crossing (sequence_id);
CREATE INDEX idx_mutation_sequence ON wuxing_ft_data_mutation (sequence_id);
CREATE INDEX idx_ai_call_sequence ON ai_ft_call (sequence_id);
