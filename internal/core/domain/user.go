package domain

import (
	"fmt"
	"regexp"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

// User is the domain model for an application user. Version supports optimistic
// concurrency checks when the user is updated.
type User struct {
	ID      int
	Version int

	FullName    string
	PhoneNumber *string
}

// NewUser constructs a User with the supplied persisted or explicitly assigned state.
func NewUser(
	id int,
	version int,
	fullName string,
	phoneNumber *string,
) User {
	return User{
		ID:          id,
		Version:     version,
		FullName:    fullName,
		PhoneNumber: phoneNumber,
	}
}

// NewUserUninitialized creates a user that has not yet received a database ID or version.
func NewUserUninitialized(
	fullName string,
	phoneNumber *string,
) User {
	return NewUser(
		UninitializedID,
		UninitializedVersion,
		fullName,
		phoneNumber,
	)
}

// Validate checks the user's full name and optional international phone number.
// Text lengths are measured in Unicode code points; phone numbers must start with '+'.
func (u *User) Validate() error {
	fullNameLen := len([]rune(u.FullName))
	if fullNameLen < 3 || fullNameLen > 100 {
		return fmt.Errorf(
			"invalid `FullName` len: %d: %w",
			fullNameLen,
			core_errors.ErrInvalidArgument,
		)
	}

	if u.PhoneNumber != nil {
		phoneNumberLen := len([]rune(*u.PhoneNumber))
		if phoneNumberLen < 10 || phoneNumberLen > 15 {
			return fmt.Errorf(
				"invalid `PhoneNumber` len: %d: %w",
				phoneNumberLen,
				core_errors.ErrInvalidArgument,
			)
		}

		re := regexp.MustCompile(`^\+[0-9]+$`)

		if !re.MatchString(*u.PhoneNumber) {
			return fmt.Errorf(
				"invalid `PhoneNumber` format:%w",
				core_errors.ErrInvalidArgument,
			)
		}
	}

	return nil
}

// UserPatch represents fields supplied for a partial user update. Nullable.Set
// distinguishes an omitted field from an explicit value or NULL.
type UserPatch struct {
	FullName    Nullable[string]
	PhoneNumber Nullable[string]
}

// NewUserPatch constructs a partial user update without applying it to a User.
func NewUserPatch(
	fullName Nullable[string],
	phoneNumber Nullable[string],
) UserPatch {
	return UserPatch{
		FullName:    fullName,
		PhoneNumber: phoneNumber,
	}
}

// Validate rejects attempts to set the required FullName field to NULL.
func (p *UserPatch) Validate() error {
	if p.FullName.Set && p.FullName.Value == nil {
		return fmt.Errorf(
			"'FullName' can't be patched to NULL: %w",
			core_errors.ErrInvalidArgument,
		)
	}

	return nil
}

// ApplyPatch applies the supplied fields to a copy, validates the resulting user,
// and updates the receiver only when the complete patch is valid.
func (u *User) ApplyPatch(path UserPatch) error {
	if err := path.Validate(); err != nil {
		return fmt.Errorf("validate user patch: %w", err)
	}

	tmp := *u

	if path.FullName.Set {
		tmp.FullName = *path.FullName.Value
	}

	if path.PhoneNumber.Set {
		tmp.PhoneNumber = path.PhoneNumber.Value
	}

	if err := tmp.Validate(); err != nil {
		return fmt.Errorf("validate patched user: %w", err)
	}

	*u = tmp

	return nil
}
