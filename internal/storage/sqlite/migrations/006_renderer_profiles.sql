CREATE TABLE IF NOT EXISTS renderer_profiles (
    id TEXT PRIMARY KEY,
    profile_json BLOB NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
