CREATE TABLE todoapp.task_activity (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    event_id UUID NOT NULL,
    task_id BIGINT NOT NULL,
    task_version BIGINT NOT NULL,

    event_type VARCHAR(100) NOT NULL,
    schema_version INTEGER NOT NULL,

    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,

    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT task_activity_event_id_unique
        UNIQUE (event_id),

    CONSTRAINT task_activity_task_id_positive
        CHECK (task_id > 0),

    CONSTRAINT task_activity_task_version_positive
        CHECK (task_version > 0),

    CONSTRAINT task_activity_event_type_not_empty
        CHECK (event_type <> ''),

    CONSTRAINT task_activity_schema_version_positive
        CHECK (schema_version > 0),

    CONSTRAINT task_activity_payload_object
        CHECK (JSONB_TYPEOF(payload) = 'object')
);

CREATE INDEX task_activity_task_history_idx
    ON todoapp.task_activity (task_id, occurred_at DESC, id DESC);