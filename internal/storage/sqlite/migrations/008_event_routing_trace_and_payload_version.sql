ALTER TABLE event_models ADD COLUMN route_trace_json BLOB;
ALTER TABLE event_models ADD COLUMN payload_version INTEGER NOT NULL DEFAULT 1;
