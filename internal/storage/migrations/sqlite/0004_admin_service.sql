-- Admin table: the service index — declared state (what wuxing IS), distinct
-- from the event facts (what it DID). Mutable via register (insert) / deregister
-- (delete); the canonical cfg is hard-saved as JSON.

CREATE TABLE wuxing_admin_service (
    service_id    TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    version       TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    cfg_json      TEXT NOT NULL,
    registered_at TEXT NOT NULL
);
