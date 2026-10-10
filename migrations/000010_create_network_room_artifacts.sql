CREATE TABLE network_room_artifacts (
    id bigserial PRIMARY KEY,
    room_id bigint NOT NULL REFERENCES network_rooms(id) ON DELETE CASCADE,
    case_id bigint NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    customer_id bigint NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    artifact_kind text NOT NULL,
    source_class text NOT NULL,
    coverage_policy text NOT NULL,
    content text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (room_id, case_id, artifact_kind, source_class),
    CHECK (artifact_kind IN ('room_context', 'assessment_summary')),
    CHECK (coverage_policy IN ('room_summary', 'source_scope')),
    CHECK (btrim(source_class) <> '')
);

CREATE INDEX network_room_artifacts_room_id_idx ON network_room_artifacts(room_id);
CREATE INDEX network_room_artifacts_case_id_idx ON network_room_artifacts(case_id);
CREATE INDEX network_room_artifacts_coverage_idx ON network_room_artifacts(coverage_policy);
