-- Per-tool detail fact tables: the typed measures behind the tool-call envelope,
-- keyed by call_id and carrying the lineage stamp so they join the spine. These
-- are append-only facts (insert-only; never closed or rewritten). Reserved words
-- are avoided in column names (row_count, recorded_at) for cross-dialect safety.

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
