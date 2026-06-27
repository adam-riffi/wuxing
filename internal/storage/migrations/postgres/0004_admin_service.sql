-- Admin table: the service index (Postgres) — identical shape to the sqlite set.
-- Declared state, mutable via register (insert) / deregister (delete).

CREATE TABLE wuxing_admin_service (
    service_id    TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    version       TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    cfg_json      TEXT NOT NULL,
    registered_at TEXT NOT NULL
);
