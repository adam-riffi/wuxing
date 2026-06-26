-- The lineage spine: one row per causal chain (sequence), per triggered
-- execution (run), and per unit of work (session). Each row is opened on start
-- and closed once on completion; causal order is carried by the *_order fields.

CREATE TABLE wuxing_ft_sequence (
    sequence_id       TEXT PRIMARY KEY,
    origin_trigger_id TEXT,
    opened_at         TEXT NOT NULL,
    closed_at         TEXT,
    outcome_id        INTEGER,
    FOREIGN KEY (origin_trigger_id) REFERENCES wuxing_dt_trigger (trigger_id),
    FOREIGN KEY (outcome_id) REFERENCES wuxing_dt_outcome (outcome_id)
);

CREATE TABLE wuxing_ft_run (
    run_id         TEXT PRIMARY KEY,
    sequence_id    TEXT NOT NULL,
    sequence_order INTEGER NOT NULL,
    service_id     TEXT,
    trigger_id     TEXT,
    opened_at      TEXT NOT NULL,
    closed_at      TEXT,
    outcome_id     INTEGER,
    FOREIGN KEY (sequence_id) REFERENCES wuxing_ft_sequence (sequence_id),
    FOREIGN KEY (service_id) REFERENCES wuxing_dt_service (service_id),
    FOREIGN KEY (trigger_id) REFERENCES wuxing_dt_trigger (trigger_id),
    FOREIGN KEY (outcome_id) REFERENCES wuxing_dt_outcome (outcome_id)
);

CREATE INDEX idx_ft_run_sequence ON wuxing_ft_run (sequence_id, sequence_order);

CREATE TABLE wuxing_ft_session (
    session_id  TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL,
    sequence_id TEXT NOT NULL,
    run_order   INTEGER NOT NULL,
    opened_at   TEXT NOT NULL,
    closed_at   TEXT,
    outcome_id  INTEGER,
    FOREIGN KEY (run_id) REFERENCES wuxing_ft_run (run_id),
    FOREIGN KEY (sequence_id) REFERENCES wuxing_ft_sequence (sequence_id),
    FOREIGN KEY (outcome_id) REFERENCES wuxing_dt_outcome (outcome_id)
);

CREATE INDEX idx_ft_session_run ON wuxing_ft_session (run_id, run_order);

-- Append-only enforcement: a spine row may be closed exactly once and never
-- deleted. You don't rewrite history — you file a correcting fact.
CREATE TRIGGER wuxing_ft_sequence_close_once BEFORE UPDATE ON wuxing_ft_sequence
WHEN OLD.closed_at IS NOT NULL
BEGIN SELECT RAISE(ABORT, 'wuxing_ft_sequence row already closed'); END;
CREATE TRIGGER wuxing_ft_sequence_no_delete BEFORE DELETE ON wuxing_ft_sequence
BEGIN SELECT RAISE(ABORT, 'wuxing_ft_sequence is append-only'); END;

CREATE TRIGGER wuxing_ft_run_close_once BEFORE UPDATE ON wuxing_ft_run
WHEN OLD.closed_at IS NOT NULL
BEGIN SELECT RAISE(ABORT, 'wuxing_ft_run row already closed'); END;
CREATE TRIGGER wuxing_ft_run_no_delete BEFORE DELETE ON wuxing_ft_run
BEGIN SELECT RAISE(ABORT, 'wuxing_ft_run is append-only'); END;

CREATE TRIGGER wuxing_ft_session_close_once BEFORE UPDATE ON wuxing_ft_session
WHEN OLD.closed_at IS NOT NULL
BEGIN SELECT RAISE(ABORT, 'wuxing_ft_session row already closed'); END;
CREATE TRIGGER wuxing_ft_session_no_delete BEFORE DELETE ON wuxing_ft_session
BEGIN SELECT RAISE(ABORT, 'wuxing_ft_session is append-only'); END;
