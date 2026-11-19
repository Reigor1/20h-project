ALTER TABLE todoapp.outbox
    ADD COLUMN traceparent TEXT NOT NULL DEFAULT '',
    ADD COLUMN tracestate TEXT NOT NULL DEFAULT '';
