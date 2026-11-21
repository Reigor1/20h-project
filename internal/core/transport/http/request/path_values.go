package core_http_request

import (
	"fmt"
	"net/http"
	"strconv"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

// GetIntPathValue reads a named URL path value and parses it as an integer.
// Missing and malformed values are wrapped with ErrInvalidArgument.
func GetIntPathValue(r *http.Request, key string) (int, error) {
	pathValue := r.PathValue(key)
	if pathValue == "" {
		return 0, fmt.Errorf(
			"no key='%s' in path values: %w",
			key,
			core_errors.ErrInvalidArgument,
		)
	}

	val, err := strconv.Atoi(pathValue)
	if err != nil {
		return 0, fmt.Errorf(
			"path value='%s' by key='%s' not a valid integer: %v: %w",
			pathValue,
			key,
			err,
			core_errors.ErrInvalidArgument,
		)
	}

	return val, nil
}
