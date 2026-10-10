CREATE TABLE network_rooms (
    id bigserial PRIMARY KEY,
    room_key text NOT NULL,
    entity_hint text NOT NULL,
    typology_hint text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'OPEN',
    public_summary text NOT NULL DEFAULT '',
    public_risk_tags text[] NOT NULL DEFAULT '{}'::text[],
    created_by_user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (room_key),
    CHECK (btrim(room_key) <> ''),
    CHECK (btrim(entity_hint) <> ''),
    CHECK (status IN ('OPEN', 'MONITORING', 'CLOSED'))
);

CREATE INDEX network_rooms_entity_hint_idx ON network_rooms(entity_hint);
CREATE INDEX network_rooms_typology_hint_idx ON network_rooms(typology_hint) WHERE typology_hint <> '';
CREATE INDEX network_rooms_status_idx ON network_rooms(status);
CREATE INDEX network_rooms_created_by_user_id_idx ON network_rooms(created_by_user_id);

CREATE TABLE network_room_cases (
    id bigserial PRIMARY KEY,
    room_id bigint NOT NULL REFERENCES network_rooms(id) ON DELETE CASCADE,
    case_id bigint NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    relation_type text NOT NULL DEFAULT 'related_case',
    added_by_user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (room_id, case_id),
    CHECK (relation_type IN ('root_case', 'related_case', 'typology_match', 'entity_match'))
);

CREATE INDEX network_room_cases_room_id_idx ON network_room_cases(room_id);
CREATE INDEX network_room_cases_case_id_idx ON network_room_cases(case_id);
CREATE INDEX network_room_cases_relation_type_idx ON network_room_cases(relation_type);

CREATE TABLE network_room_participants (
    id bigserial PRIMARY KEY,
    room_id bigint NOT NULL REFERENCES network_rooms(id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    participant_role text NOT NULL DEFAULT 'analyst',
    joined_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (room_id, user_id),
    CHECK (participant_role IN ('analyst', 'observer'))
);

CREATE INDEX network_room_participants_room_id_idx ON network_room_participants(room_id);
CREATE INDEX network_room_participants_user_id_idx ON network_room_participants(user_id);
