CREATE TABLE disputes (
    id bigserial PRIMARY KEY,
    owner_user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    case_id bigint NOT NULL REFERENCES cases(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'OPEN',
    reason text NOT NULL,
    resolution_summary text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_user_id, case_id, status),
    CHECK (btrim(reason) <> ''),
    CHECK (status IN ('OPEN', 'UNDER_REVIEW', 'RESOLVED', 'REJECTED'))
);

CREATE INDEX disputes_owner_status_idx ON disputes(owner_user_id, status);
CREATE INDEX disputes_case_id_idx ON disputes(case_id);
CREATE INDEX disputes_created_at_idx ON disputes(created_at);

CREATE TABLE exports (
    id bigserial PRIMARY KEY,
    owner_user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    root_case_id bigint NOT NULL REFERENCES cases(id) ON DELETE RESTRICT,
    dispute_id bigint REFERENCES disputes(id) ON DELETE SET NULL,
    purpose text NOT NULL,
    status text NOT NULL DEFAULT 'PENDING',
    bundle_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    redaction_policy text NOT NULL DEFAULT 'public_redacted',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '7 days'),
    CHECK (btrim(purpose) <> ''),
    CHECK (status IN ('PENDING', 'GENERATED', 'FAILED', 'EXPIRED')),
    CHECK (redaction_policy IN ('public_redacted')),
    CHECK (jsonb_typeof(bundle_json) = 'object')
);

CREATE INDEX exports_owner_status_idx ON exports(owner_user_id, status);
CREATE INDEX exports_root_case_id_idx ON exports(root_case_id);
CREATE INDEX exports_dispute_id_idx ON exports(dispute_id);
CREATE INDEX exports_expires_at_idx ON exports(expires_at);

CREATE TABLE export_items (
    id bigserial PRIMARY KEY,
    export_id bigint NOT NULL REFERENCES exports(id) ON DELETE CASCADE,
    item_type text NOT NULL,
    object_type text NOT NULL,
    object_id bigint NOT NULL,
    relation text NOT NULL,
    redaction_policy text NOT NULL DEFAULT 'public_redacted',
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (export_id, item_type, object_type, object_id, relation),
    CHECK (object_id > 0),
    CHECK (item_type IN ('root_case', 'linked_case', 'audit_summary', 'edd_summary', 'metadata')),
    CHECK (object_type IN ('case', 'audit_event', 'edd_record', 'export')),
    CHECK (relation IN ('root', 'linked', 'audit', 'edd', 'metadata')),
    CHECK (redaction_policy IN ('public_redacted')),
    CHECK (jsonb_typeof(payload_json) = 'object')
);

CREATE INDEX export_items_export_id_idx ON export_items(export_id);
CREATE INDEX export_items_object_idx ON export_items(object_type, object_id);
CREATE INDEX export_items_item_type_idx ON export_items(item_type);
