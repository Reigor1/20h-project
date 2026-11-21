package core_errors

import "errors"

// Shared sentinel errors let services and transport adapters classify common
// domain failures with errors.Is without depending on their message text.
var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrConflict        = errors.New("conflict")
)
