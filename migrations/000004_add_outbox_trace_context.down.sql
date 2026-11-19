ALTER TABLE todoapp.outbox
    DROP COLUMN tracestate,
    DROP COLUMN traceparent;
