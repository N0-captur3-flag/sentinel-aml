CREATE TABLE edd_records (
    id bigserial PRIMARY KEY,
    customer_id bigint NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    analyst_user_id bigint NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    source_of_funds text NOT NULL,
    risk_rationale text NOT NULL,
    confidential_comment text NOT NULL,
    review_status text NOT NULL DEFAULT 'DRAFT',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (customer_id),
    CHECK (review_status IN ('DRAFT', 'IN_REVIEW', 'APPROVED', 'REJECTED'))
);

CREATE INDEX edd_records_customer_id_idx ON edd_records(customer_id);
CREATE INDEX edd_records_analyst_user_id_idx ON edd_records(analyst_user_id);
CREATE INDEX edd_records_review_status_idx ON edd_records(review_status);
CREATE INDEX edd_records_updated_at_idx ON edd_records(updated_at);
