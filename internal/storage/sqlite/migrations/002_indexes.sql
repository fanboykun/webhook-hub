CREATE UNIQUE INDEX IF NOT EXISTS idx_receipt_source_delivery
    ON receipt_models(source, integration_id, source_delivery_id);

CREATE INDEX IF NOT EXISTS idx_events_receipt_id
    ON event_models(receipt_id);

CREATE INDEX IF NOT EXISTS idx_events_source
    ON event_models(source);

CREATE INDEX IF NOT EXISTS idx_events_type
    ON event_models(type);

CREATE INDEX IF NOT EXISTS idx_events_service_env_occurred
    ON event_models(service, environment, occurred_at);

CREATE INDEX IF NOT EXISTS idx_events_fingerprint
    ON event_models(fingerprint);

CREATE UNIQUE INDEX IF NOT EXISTS idx_delivery_event_destination
    ON delivery_models(event_id, destination_id);

CREATE INDEX IF NOT EXISTS idx_deliveries_status_next_attempt
    ON delivery_models(status, next_attempt_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_delivery_attempt_sequence
    ON delivery_attempt_models(delivery_id, attempt_number);
