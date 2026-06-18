CREATE TABLE IF NOT EXISTS route_models (
    id TEXT PRIMARY KEY,
    match_json BLOB NOT NULL,
    destinations_json BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
