CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS receipt_models (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    integration_id TEXT NOT NULL,
    source_delivery_id TEXT NOT NULL,
    source_event_type TEXT NOT NULL,
    payload_sha256 TEXT NOT NULL,
    raw_payload BLOB,
    headers_json BLOB,
    received_at TIMESTAMP NOT NULL,
    status TEXT NOT NULL,
    ignore_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS event_models (
    id TEXT PRIMARY KEY,
    receipt_id TEXT NOT NULL,
    source TEXT NOT NULL,
    integration_id TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    type TEXT NOT NULL,
    action TEXT NOT NULL,
    lifecycle TEXT NOT NULL,
    severity TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    service TEXT NOT NULL,
    environment TEXT NOT NULL,
    release TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    actor TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    group_key TEXT NOT NULL,
    url TEXT NOT NULL,
    occurred_at TIMESTAMP NOT NULL,
    labels_json BLOB,
    fields_json BLOB,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY(receipt_id) REFERENCES receipt_models(id)
);

CREATE TABLE IF NOT EXISTS delivery_models (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    destination_type TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL,
    max_attempts INTEGER NOT NULL,
    next_attempt_at TIMESTAMP NOT NULL,
    locked_by TEXT NOT NULL DEFAULT '',
    locked_until TIMESTAMP NULL,
    provider_message_id TEXT NOT NULL DEFAULT '',
    last_error_code TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    FOREIGN KEY(event_id) REFERENCES event_models(id)
);

CREATE TABLE IF NOT EXISTS delivery_attempt_models (
    id TEXT PRIMARY KEY,
    delivery_id TEXT NOT NULL,
    attempt_number INTEGER NOT NULL,
    worker_id TEXT NOT NULL,
    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP NOT NULL,
    outcome TEXT NOT NULL,
    response_code INTEGER NOT NULL,
    error_code TEXT NOT NULL,
    error_message TEXT NOT NULL,
    duration_ms INTEGER NOT NULL,
    FOREIGN KEY(delivery_id) REFERENCES delivery_models(id)
);
