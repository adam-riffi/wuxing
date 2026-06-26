-- Core dimension tables from the data-model spine: small, stable lookups joined
-- to the fact tables. Naming follows ‹database›.‹domain›_‹kind›_‹entity›; here the
-- database (wuxing) is implicit in the file, the domain is the core (wuxing), and
-- the kind is dt (dimension).

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

-- The fixed outcome vocabulary services classify their domain errors into and
-- the kernel stamps onto every event.
INSERT INTO wuxing_dt_outcome (outcome_id, name) VALUES
    (1, 'success'),
    (2, 'failure'),
    (3, 'cancelled');
