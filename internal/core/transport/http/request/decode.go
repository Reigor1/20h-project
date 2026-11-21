package core_http_request

import (
	"encoding/json"
	"fmt"
	"net/http"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	"github.com/go-playground/validator/v10"
)

var requestValidator = validator.New()

// validatable is implemented by request DTOs that perform domain-specific validation.
type validatable interface {
	Validate() error
}

// DecodeAndValidateRequest decodes the request body into dest and validates it.
// DTOs implementing Validate use that method; other values use the shared struct validator.
func DecodeAndValidateRequest(r *http.Request, dest any) error {
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode json: %v: %w",
			err,
			core_errors.ErrInvalidArgument,
		)
	}

	var err error

	v, ok := dest.(validatable)
	if ok {
		err = v.Validate()
	} else {
		err = requestValidator.Struct(dest)
	}

	if err != nil {
		return fmt.Errorf(
			"request validation: %v: %w",
			err,
			core_errors.ErrInvalidArgument,
		)
	}

	return nil
}
