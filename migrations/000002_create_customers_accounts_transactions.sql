CREATE TABLE customers (
    id bigserial PRIMARY KEY,
    owner_user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    display_name text NOT NULL,
    customer_type text NOT NULL,
    country text NOT NULL,
    registration_number text NOT NULL,
    risk_level text NOT NULL DEFAULT 'medium',
    entity_hint text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    status text NOT NULL DEFAULT 'active',
    UNIQUE (owner_user_id, registration_number),
    CHECK (customer_type IN ('individual', 'organization')),
    CHECK (risk_level IN ('low', 'medium', 'high')),
    CHECK (status IN ('active', 'inactive'))
);

CREATE INDEX customers_owner_user_id_idx ON customers(owner_user_id);
CREATE INDEX customers_owner_status_idx ON customers(owner_user_id, status);
CREATE INDEX customers_risk_level_idx ON customers(risk_level);

CREATE TABLE accounts (
    id bigserial PRIMARY KEY,
    customer_id bigint NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    iban_like text NOT NULL,
    currency text NOT NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (customer_id, iban_like),
    CHECK (currency ~ '^[A-Z]{3}$'),
    CHECK (status IN ('active', 'closed'))
);

CREATE INDEX accounts_customer_id_idx ON accounts(customer_id);
CREATE INDEX accounts_status_idx ON accounts(status);

CREATE TABLE transactions (
    id bigserial PRIMARY KEY,
    account_id bigint NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    counterparty_name text NOT NULL,
    counterparty_country text NOT NULL,
    counterparty_fingerprint text NOT NULL,
    amount numeric(18,2) NOT NULL,
    currency text NOT NULL,
    direction text NOT NULL,
    purpose text NOT NULL,
    trace_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    risk_score integer NOT NULL DEFAULT 0,
    UNIQUE (account_id, trace_id),
    CHECK (amount > 0),
    CHECK (currency ~ '^[A-Z]{3}$'),
    CHECK (direction IN ('inbound', 'outbound')),
    CHECK (risk_score >= 0 AND risk_score <= 100)
);

CREATE INDEX transactions_account_id_idx ON transactions(account_id);
CREATE INDEX transactions_trace_id_idx ON transactions(trace_id);
CREATE INDEX transactions_counterparty_fingerprint_idx ON transactions(counterparty_fingerprint);
CREATE INDEX transactions_created_at_idx ON transactions(created_at);
CREATE INDEX transactions_risk_score_idx ON transactions(risk_score);
