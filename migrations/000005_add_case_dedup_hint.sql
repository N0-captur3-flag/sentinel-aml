ALTER TABLE cases
    ADD COLUMN dedup_hint text NOT NULL DEFAULT '';

ALTER TABLE cases
    ADD CONSTRAINT cases_dedup_hint_not_blank_when_set
    CHECK (dedup_hint = '' OR btrim(dedup_hint) <> '');

CREATE INDEX cases_dedup_hint_idx ON cases(dedup_hint) WHERE dedup_hint <> '';
CREATE INDEX cases_owner_dedup_hint_idx ON cases(owner_user_id, dedup_hint) WHERE dedup_hint <> '';
