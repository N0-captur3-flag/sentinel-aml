CREATE TABLE network_room_dirty_candidates (
    id bigserial PRIMARY KEY,
    entity_hint_norm text NOT NULL,
    typology_hint_norm text NOT NULL,
    reason text NOT NULL DEFAULT 'case_changed',
    status text NOT NULL DEFAULT 'pending',
    attempts integer NOT NULL DEFAULT 0,
    locked_until timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_hint_norm, typology_hint_norm),
    CHECK (btrim(entity_hint_norm) <> ''),
    CHECK (btrim(typology_hint_norm) <> ''),
    CHECK (status IN ('pending', 'processing', 'done'))
);

CREATE INDEX IF NOT EXISTS network_room_dirty_candidates_pending_idx
ON network_room_dirty_candidates (updated_at, id)
WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS network_room_dirty_candidates_expired_processing_idx
ON network_room_dirty_candidates (locked_until, id)
WHERE status = 'processing';

CREATE INDEX IF NOT EXISTS customers_entity_hint_norm_idx
ON customers ((replace(lower(btrim(entity_hint)), ' ', '_')))
WHERE btrim(entity_hint) <> '';

CREATE INDEX IF NOT EXISTS cases_typology_hint_norm_idx
ON cases ((replace(lower(btrim(typology_hint)), ' ', '_')))
WHERE btrim(typology_hint) <> '';

CREATE INDEX IF NOT EXISTS cases_customer_typology_idx
ON cases(customer_id, typology_hint);

CREATE INDEX IF NOT EXISTS network_room_cases_room_idx
ON network_room_cases(room_id);

CREATE INDEX IF NOT EXISTS network_room_cases_case_idx
ON network_room_cases(case_id);

CREATE INDEX IF NOT EXISTS audit_events_object_created_idx
ON audit_events(object_type, object_id, created_at);
