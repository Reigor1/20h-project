CREATE TABLE todoapp.users (
    id SERIAL PRIMARY KEY,
    version BIGINT NOT NULL DEFAULT 1,
    full_name VARCHAR(100) NOT NULL CHECK (
        char_length(full_name) BETWEEN 3 AND 100
    ),
    phone_number VARCHAR(15) CHECK (
        phone_number ~ '^\+[0-9]{9,14}$'
    )
);

CREATE TABLE todoapp.tasks (
    id SERIAL PRIMARY KEY,
    version BIGINT NOT NULL DEFAULT 1,
    title VARCHAR(100) NOT NULL CHECK (
        char_length(title) BETWEEN 1 AND 100
    ),
    description VARCHAR(1000) CHECK (
        char_length(description) BETWEEN 1 AND 1000
    ),
    completed BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    CHECK (
        (
            completed = FALSE
            AND completed_at IS NULL
        )
        OR (
            completed = TRUE
            AND completed_at IS NOT NULL
            AND completed_at >= created_at
        )
    ),
    author_user_id INTEGER NOT NULL REFERENCES todoapp.users (id)
);

CREATE TABLE todoapp.outbox (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    schema_version INTEGER NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    aggregate_version BIGINT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    CONSTRAINT outbox_event_id_unique UNIQUE (event_id),
    CONSTRAINT outbox_event_type_not_empty CHECK (event_type <> ''),
    CONSTRAINT outbox_schema_version_positive CHECK (schema_version > 0),
    CONSTRAINT outbox_aggregate_type_not_empty CHECK (aggregate_type <> ''),
    CONSTRAINT outbox_aggregate_id_positive CHECK (aggregate_id > 0),
    CONSTRAINT outbox_aggregate_version_positive CHECK (aggregate_version > 0),
    CONSTRAINT outbox_payload_object CHECK (
        JSONB_TYPEOF(payload) = 'object'
    ),
    CONSTRAINT outbox_attempts_non_negative CHECK (attempts >= 0)
);

CREATE INDEX outbox_pending_idx ON todoapp.outbox (created_at, id)
WHERE
    published_at IS NULL;

CREATE INDEX outbox_aggregate_version_idx ON todoapp.outbox (
    aggregate_type,
    aggregate_id,
    aggregate_version
);