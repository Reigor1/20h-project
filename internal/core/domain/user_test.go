package domain

import (
	"errors"
	"testing"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

// TestUserValidate verifies full-name and optional phone-number constraints.
func TestUserValidate(t *testing.T) {
	validPhoneNumber := "+1234567890"
	phoneNumberWithoutPrefix := "1234567890"
	tooShortFullName := "Al"

	tests := []struct {
		name    string
		user    User
		wantErr bool
	}{
		{
			name: "valid user without phone",
			user: User{
				FullName: "Alex Smith",
			},
		},
		{
			name: "valid user with phone",
			user: User{
				FullName:    "Alex Smith",
				PhoneNumber: &validPhoneNumber,
			},
		},
		{
			name: "name shorter than minimum",
			user: User{
				FullName: tooShortFullName,
			},
			wantErr: true,
		},
		{
			name: "phone without plus prefix",
			user: User{
				FullName:    "Alex Smith",
				PhoneNumber: &phoneNumberWithoutPrefix,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.user.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, want error: %t", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

// TestUserPatchValidate checks required-field and nullable-field rules for user patches.
func TestUserPatchValidate(t *testing.T) {
	updatedFullName := "Updated name"
	validPhoneNumber := "+1234567890"

	tests := []struct {
		name    string
		patch   UserPatch
		wantErr bool
	}{
		{
			name: "valid field updates",
			patch: UserPatch{
				FullName: Nullable[string]{
					Value: &updatedFullName,
					Set:   true,
				},
				PhoneNumber: Nullable[string]{
					Value: &validPhoneNumber,
					Set:   true,
				},
			},
		},
		{
			name: "phone number can be cleared",
			patch: UserPatch{
				PhoneNumber: Nullable[string]{Set: true},
			},
		},
		{
			name: "full name cannot be cleared",
			patch: UserPatch{
				FullName: Nullable[string]{Set: true},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.patch.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, want error: %t", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

// TestUserApplyPatch verifies that only supplied fields change and invalid patches are atomic.
func TestUserApplyPatch(t *testing.T) {
	originalPhoneNumber := "+1234567890"
	updatedFullName := "Updated name"
	emptyFullName := ""

	tests := []struct {
		name      string
		patch     UserPatch
		wantName  string
		wantPhone *string
		wantErr   bool
	}{
		{
			name: "updates name and clears phone",
			patch: UserPatch{
				FullName: Nullable[string]{
					Value: &updatedFullName,
					Set:   true,
				},
				PhoneNumber: Nullable[string]{Set: true},
			},
			wantName: updatedFullName,
		},
		{
			name: "invalid name leaves user unchanged",
			patch: UserPatch{
				FullName: Nullable[string]{
					Value: &emptyFullName,
					Set:   true,
				},
			},
			wantName:  "Original name",
			wantPhone: &originalPhoneNumber,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := User{
				FullName:    "Original name",
				PhoneNumber: &originalPhoneNumber,
			}
			err := user.ApplyPatch(tt.patch)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ApplyPatch() error = %v, want error: %t", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("ApplyPatch() error = %v, want ErrInvalidArgument", err)
			}
			if user.FullName != tt.wantName || !sameStringPointerValue(user.PhoneNumber, tt.wantPhone) {
				t.Fatalf(
					"ApplyPatch() user = {FullName: %q, PhoneNumber: %v}, want {FullName: %q, PhoneNumber: %v}",
					user.FullName,
					user.PhoneNumber,
					tt.wantName,
					tt.wantPhone,
				)
			}
		})
	}
}

// TestNewUserUninitialized checks the default identity and version of a new user.
func TestNewUserUninitialized(t *testing.T) {
	phoneNumber := "+1234567890"
	user := NewUserUninitialized("Alex Smith", &phoneNumber)

	if user.ID != UninitializedID || user.Version != UninitializedVersion {
		t.Fatalf(
			"NewUserUninitialized() ID/version = %d/%d, want %d/%d",
			user.ID,
			user.Version,
			UninitializedID,
			UninitializedVersion,
		)
	}
	if user.FullName != "Alex Smith" || !sameStringPointerValue(user.PhoneNumber, &phoneNumber) {
		t.Fatalf("NewUserUninitialized() fields = %+v, want input name and phone number", user)
	}
}

// sameStringPointerValue compares optional string values for test assertions.
func sameStringPointerValue(got, want *string) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}
