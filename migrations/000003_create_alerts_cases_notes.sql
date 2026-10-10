CREATE TABLE alerts (
    id bigserial PRIMARY KEY,
    transaction_id bigint NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    customer_id bigint NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    rule_code text NOT NULL,
    risk_score integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'NEW',
    assigned_user_id bigint REFERENCES users(id) ON DELETE SET NULL,
    typology_hint text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (transaction_id, rule_code),
    CHECK (risk_score >= 0 AND risk_score <= 100),
    CHECK (status IN ('NEW', 'TRIAGED', 'CASE_OPENED', 'ESCALATED', 'CLOSED'))
);

CREATE INDEX alerts_transaction_id_idx ON alerts(transaction_id);
CREATE INDEX alerts_customer_id_idx ON alerts(customer_id);
CREATE INDEX alerts_customer_status_idx ON alerts(customer_id, status);
CREATE INDEX alerts_assigned_user_id_idx ON alerts(assigned_user_id);
CREATE INDEX alerts_status_idx ON alerts(status);
CREATE INDEX alerts_updated_at_idx ON alerts(updated_at);

CREATE TABLE cases (
    id bigserial PRIMARY KEY,
    customer_id bigint NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    alert_id bigint NOT NULL REFERENCES alerts(id) ON DELETE RESTRICT,
    owner_user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'OPEN',
    public_summary text NOT NULL DEFAULT '',
    risk_score integer NOT NULL DEFAULT 0,
    typology_hint text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (alert_id),
    CHECK (risk_score >= 0 AND risk_score <= 100),
    CHECK (status IN ('OPEN', 'UNDER_REVIEW', 'ESCALATED', 'REPORTED', 'CLOSED'))
);

CREATE INDEX cases_customer_id_idx ON cases(customer_id);
CREATE INDEX cases_alert_id_idx ON cases(alert_id);
CREATE INDEX cases_owner_user_id_idx ON cases(owner_user_id);
CREATE INDEX cases_owner_status_idx ON cases(owner_user_id, status);
CREATE INDEX cases_updated_at_idx ON cases(updated_at);

CREATE TABLE case_notes (
    id bigserial PRIMARY KEY,
    case_id bigint NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    author_user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    visibility text NOT NULL DEFAULT 'private',
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (visibility IN ('private', 'internal'))
);

CREATE INDEX case_notes_case_id_idx ON case_notes(case_id);
CREATE INDEX case_notes_author_user_id_idx ON case_notes(author_user_id);
CREATE INDEX case_notes_visibility_idx ON case_notes(visibility);
CREATE INDEX case_notes_updated_at_idx ON case_notes(updated_at);
