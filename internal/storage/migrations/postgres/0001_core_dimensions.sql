-- Core dimension tables (Postgres). Identical shape to the sqlite set; the
-- INTEGER PRIMARY KEY holds explicit ids (no auto-increment needed).

CREATE TABLE wuxing_dt_outcome (
    outcome_id INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE
);

CREATE TABLE wuxing_dt_trigger (
    trigger_id TEXT PRIMARY KEY,
    kind       TEXT NOT NULL CHECK (kind IN ('cron', 'event', 'manual')),
    spec       TEXT NOT NULL DEFAULT ''
);

CREATE TABLE wuxing_dt_service (
    service_id TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    version    TEXT NOT NULL DEFAULT ''
);

INSERT INTO wuxing_dt_outcome (outcome_id, name) VALUES
    (1, 'success'),
    (2, 'failure'),
    (3, 'cancelled');
