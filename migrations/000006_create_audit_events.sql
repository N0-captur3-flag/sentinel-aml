CREATE TABLE audit_events (
    id bigserial PRIMARY KEY,
    actor_user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    object_type text NOT NULL,
    object_id bigint NOT NULL,
    event_type text NOT NULL,
    public_message text NOT NULL DEFAULT '',
    diff_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    visibility_scope text NOT NULL DEFAULT 'owner',
    CHECK (object_id > 0),
    CHECK (object_type IN ('customer', 'account', 'transaction', 'alert', 'case', 'case_note', 'edd_record')),
    CHECK (event_type IN ('created', 'updated', 'status_changed', 'claimed', 'triaged', 'reviewed')),
    CHECK (visibility_scope IN ('owner', 'internal', 'system')),
    CHECK (jsonb_typeof(diff_json) = 'array')
);

CREATE INDEX audit_events_actor_user_id_idx ON audit_events(actor_user_id);
CREATE INDEX audit_events_object_idx ON audit_events(object_type, object_id);
CREATE INDEX audit_events_event_type_idx ON audit_events(event_type);
CREATE INDEX audit_events_visibility_created_idx ON audit_events(visibility_scope, created_at);
CREATE INDEX audit_events_created_at_idx ON audit_events(created_at);
