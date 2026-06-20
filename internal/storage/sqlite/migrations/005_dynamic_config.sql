CREATE TABLE IF NOT EXISTS integrations (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    config_ciphertext BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS destinations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    config_ciphertext BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_integrations_source ON integrations(source);
CREATE INDEX IF NOT EXISTS idx_destinations_type ON destinations(type);
