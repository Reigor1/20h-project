package domain

// UninitializedID and UninitializedVersion mark domain entities that have not yet
// been assigned their database identity and persisted version.
var (
	UninitializedID      = -1
	UninitializedVersion = -1
)
