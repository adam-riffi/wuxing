-- The processors tool: derived rollups of the raw facts — one row per run and
-- per sequence, in the processors_ domain. Unlike the append-only source facts,
-- these are DERIVED projections (recomputable as late facts arrive), so they are
-- upserted by run_id / sequence_id and carry no close-once triggers.

CREATE TABLE processors_ft_run (
    run_id         TEXT PRIMARY KEY,
    sequence_id    TEXT,
    service_id     TEXT,
    outcome_id     INTEGER,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    session_count  INTEGER NOT NULL DEFAULT 0,
    ai_calls       INTEGER NOT NULL DEFAULT 0,
    ai_tokens_in   INTEGER NOT NULL DEFAULT 0,
    ai_tokens_out  INTEGER NOT NULL DEFAULT 0,
    ai_cost        DOUBLE PRECISION NOT NULL DEFAULT 0,
    crossings      INTEGER NOT NULL DEFAULT 0,
    rows_crossed   INTEGER NOT NULL DEFAULT 0,
    bytes_crossed  INTEGER NOT NULL DEFAULT 0,
    mutations      INTEGER NOT NULL DEFAULT 0,
    rows_written   INTEGER NOT NULL DEFAULT 0,
    derived_at     TEXT NOT NULL
);

CREATE TABLE processors_ft_sequence (
    sequence_id    TEXT PRIMARY KEY,
    run_count      INTEGER NOT NULL DEFAULT 0,
    outcome_id     INTEGER,
    ai_calls       INTEGER NOT NULL DEFAULT 0,
    ai_tokens_in   INTEGER NOT NULL DEFAULT 0,
    ai_tokens_out  INTEGER NOT NULL DEFAULT 0,
    ai_cost        DOUBLE PRECISION NOT NULL DEFAULT 0,
    crossings      INTEGER NOT NULL DEFAULT 0,
    mutations      INTEGER NOT NULL DEFAULT 0,
    derived_at     TEXT NOT NULL
);
