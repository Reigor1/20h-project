package core_postgres_pool

import "errors"

// Sentinel errors provide stable classifications for common PostgreSQL adapter failures.
var (
	ErrNoRows             = errors.New("no rows")
	ErrViolatesForeignKey = errors.New("violates foreign key")
	ErrUnknown            = errors.New("unknown error")
)
