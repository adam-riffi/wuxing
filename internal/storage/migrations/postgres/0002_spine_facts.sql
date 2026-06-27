-- The lineage spine (Postgres). Same tables/FKs as the sqlite set; append-only
-- enforcement uses a shared plpgsql guard instead of sqlite's RAISE(ABORT)
-- triggers.
--
-- NOTE: this migration is multi-statement and uses dollar-quoted plpgsql, so it
-- is executed under pgx's simple query protocol (configured in storage.Open for
-- the postgres dialect). Verified against a real Postgres (testcontainer/Supabase).

CREATE TABLE wuxing_ft_sequence (
    sequence_id       TEXT PRIMARY KEY,
    origin_trigger_id TEXT REFERENCES wuxing_dt_trigger (trigger_id),
    opened_at         TEXT NOT NULL,
    closed_at         TEXT,
    outcome_id        INTEGER REFERENCES wuxing_dt_outcome (outcome_id)
);

CREATE TABLE wuxing_ft_run (
    run_id         TEXT PRIMARY KEY,
    sequence_id    TEXT NOT NULL REFERENCES wuxing_ft_sequence (sequence_id),
    sequence_order INTEGER NOT NULL,
    service_id     TEXT REFERENCES wuxing_dt_service (service_id),
    trigger_id     TEXT REFERENCES wuxing_dt_trigger (trigger_id),
    opened_at      TEXT NOT NULL,
    closed_at      TEXT,
    outcome_id     INTEGER REFERENCES wuxing_dt_outcome (outcome_id)
);

CREATE INDEX idx_ft_run_sequence ON wuxing_ft_run (sequence_id, sequence_order);

CREATE TABLE wuxing_ft_session (
    session_id  TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES wuxing_ft_run (run_id),
    sequence_id TEXT NOT NULL REFERENCES wuxing_ft_sequence (sequence_id),
    run_order   INTEGER NOT NULL,
    opened_at   TEXT NOT NULL,
    closed_at   TEXT,
    outcome_id  INTEGER REFERENCES wuxing_dt_outcome (outcome_id)
);

CREATE INDEX idx_ft_session_run ON wuxing_ft_session (run_id, run_order);

-- A spine row may be closed exactly once and never deleted.
CREATE FUNCTION wuxing_spine_guard() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
    END IF;
    IF OLD.closed_at IS NOT NULL THEN
        RAISE EXCEPTION '% row already closed', TG_TABLE_NAME;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER wuxing_ft_sequence_guard BEFORE UPDATE OR DELETE ON wuxing_ft_sequence
    FOR EACH ROW EXECUTE FUNCTION wuxing_spine_guard();
CREATE TRIGGER wuxing_ft_run_guard BEFORE UPDATE OR DELETE ON wuxing_ft_run
    FOR EACH ROW EXECUTE FUNCTION wuxing_spine_guard();
CREATE TRIGGER wuxing_ft_session_guard BEFORE UPDATE OR DELETE ON wuxing_ft_session
    FOR EACH ROW EXECUTE FUNCTION wuxing_spine_guard();
