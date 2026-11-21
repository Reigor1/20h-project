package core_http_request

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
)

// TestDecodeAndValidateRequest covers JSON decoding, DTO validation, and custom Validate methods.
func TestDecodeAndValidateRequest(t *testing.T) {
	type validatedRequest struct {
		Name string `json:"name" validate:"required"`
	}
	tests := []struct {
		name                string
		body                string
		dest                any
		wantErr             bool
		wantInvalidArgument bool
		wantName            string
	}{
		{
			name:     "valid JSON",
			body:     `{"name":"Ada"}`,
			dest:     &validatedRequest{},
			wantName: "Ada",
		},
		{
			name:                "malformed JSON",
			body:                `{"name":`,
			dest:                &validatedRequest{},
			wantErr:             true,
			wantInvalidArgument: true,
		},
		{
			name:                "empty body",
			body:                "",
			dest:                &validatedRequest{},
			wantErr:             true,
			wantInvalidArgument: true,
		},
		{
			name:                "validate tag rejects missing name",
			body:                `{}`,
			dest:                &validatedRequest{},
			wantErr:             true,
			wantInvalidArgument: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(
				"POST",
				"/",
				strings.NewReader(tc.body),
			)
			err := DecodeAndValidateRequest(request, tc.dest)
			if (err != nil) != tc.wantErr {
				t.Fatalf("DecodeAndValidateRequest() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantInvalidArgument && !errors.Is(err, core_errors.ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
			if err == nil && tc.wantName != "" {
				gotName := tc.dest.(*validatedRequest).Name
				if gotName != tc.wantName {
					t.Fatalf("decoded name = %q, want %q", gotName, tc.wantName)
				}
			}
		})
	}

	t.Run("valid validate tag is accepted", func(t *testing.T) {
		var dest validatedRequest
		request := httptest.NewRequest(
			"POST",
			"/",
			strings.NewReader(`{"name":"Ada"}`),
		)
		if err := DecodeAndValidateRequest(request, &dest); err != nil {
			t.Fatalf("DecodeAndValidateRequest() error = %v", err)
		}
	})
	t.Run("custom validation error is wrapped", func(t *testing.T) {
		called := false
		dest := &customValidatable{called: &called}
		request := httptest.NewRequest(
			"POST",
			"/",
			strings.NewReader(`{"name":"Ada"}`),
		)
		err := DecodeAndValidateRequest(request, dest)
		if !called {
			t.Fatal("custom Validate() was not called")
		}
		if !errors.Is(err, core_errors.ErrInvalidArgument) {
			t.Fatalf("error = %v, want ErrInvalidArgument", err)
		}
	})
	t.Run("trailing second JSON value is currently accepted", func(t *testing.T) {
		var dest validatedRequest
		request := httptest.NewRequest(
			"POST",
			"/",
			strings.NewReader(`{"name":"Ada"} {"name":"Grace"}`),
		)
		err := DecodeAndValidateRequest(request, &dest)
		if err != nil || dest.Name != "Ada" {
			t.Fatalf("current single-decode behavior: name=%q error=%v", dest.Name, err)
		}
	})
}

// customValidatable is a test DTO whose validation behavior is controlled by its fields.
type customValidatable struct {
	Name   string `json:"name"`
	called *bool
}

// Validate returns a test error when the custom DTO is configured as invalid.
func (v *customValidatable) Validate() error {
	*v.called = true
	return errors.New("invalid custom request")
}

// TestGetIntPathValue checks parsing and error classification of integer path values.
func TestGetIntPathValue(t *testing.T) {
	tooLarge := "999999999999999999999999999999999999999"
	tests := []struct {
		name    string
		value   string
		set     bool
		want    int
		wantErr bool
	}{
		{
			name:  "positive integer",
			value: "42",
			set:   true,
			want:  42,
		},
		{
			name:  "negative integer",
			value: "-9",
			set:   true,
			want:  -9,
		},
		{
			name:    "missing parameter",
			wantErr: true,
		},
		{
			name:    "empty value",
			value:   "",
			set:     true,
			wantErr: true,
		},
		{
			name:    "invalid string",
			value:   "4x",
			set:     true,
			wantErr: true,
		},
		{
			name:    "integer overflow",
			value:   tooLarge,
			set:     true,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tc.set {
				r.SetPathValue("id", tc.value)
			}
			got, err := GetIntPathValue(r, "id")
			if (err != nil) != tc.wantErr {
				t.Fatalf(
					"GetIntPathValue() = %d, %v; wantErr %v",
					got,
					err,
					tc.wantErr,
				)
			}
			if tc.wantErr {
				if !errors.Is(err, core_errors.ErrInvalidArgument) {
					t.Fatalf("error = %v, want ErrInvalidArgument", err)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("value = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestGetIntQueryParam checks optional integer query parameter parsing.
func TestGetIntQueryParam(t *testing.T) {
	overflow := "999999999999999999999999999999999999999"
	tests := []struct {
		name    string
		query   string
		want    *int
		wantErr bool
	}{
		{
			name:  "missing",
			query: "",
		},
		{
			name:  "empty",
			query: "?id=",
		},
		{
			name:  "valid integer",
			query: "?id=17",
			want:  intPointer(17),
		},
		{
			name:  "negative integer",
			query: "?id=-3",
			want:  intPointer(-3),
		},
		{
			name:    "invalid integer",
			query:   "?id=nope",
			wantErr: true,
		},
		{
			name:    "overflow",
			query:   "?id=" + overflow,
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/"+tc.query, nil)
			got, err := GetIntQueryParam(request, "id")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if !errors.Is(err, core_errors.ErrInvalidArgument) {
					t.Fatalf("error = %v, want ErrInvalidArgument", err)
				}
				return
			}
			if tc.want == nil && got != nil {
				t.Fatalf("value = %v, want nil", *got)
			}
			if tc.want != nil && (got == nil || *got != *tc.want) {
				t.Fatalf("value = %v, want %d", got, *tc.want)
			}
		})
	}
}

// TestGetDateQueryParam checks optional YYYY-MM-DD query parameter parsing.
func TestGetDateQueryParam(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    *time.Time
		wantErr bool
	}{
		{
			name:  "missing",
			query: "",
		},
		{
			name:  "valid date",
			query: "?date=2026-10-24",
			want: datePointer(
				time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC),
			),
		},
		{
			name:  "leap day",
			query: "?date=2024-02-29",
			want: datePointer(
				time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			),
		},
		{
			name:    "malformed date",
			query:   "?date=24-10-2026",
			wantErr: true,
		},
		{
			name:    "impossible calendar date",
			query:   "?date=2026-02-30",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/"+tc.query, nil)
			got, err := GetDateQueryParam(request, "date")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if !errors.Is(err, core_errors.ErrInvalidArgument) {
					t.Fatalf("error = %v, want ErrInvalidArgument", err)
				}
				return
			}
			if tc.want == nil && got != nil {
				t.Fatalf("date = %v, want nil", *got)
			}
			if tc.want != nil && (got == nil || !got.Equal(*tc.want)) {
				t.Fatalf("date = %v, want %v", got, tc.want)
			}
		})
	}
}

// intPointer creates an integer pointer for query parsing test cases.
func intPointer(value int) *int { return &value }

// datePointer creates a time pointer for query parsing test cases.
func datePointer(value time.Time) *time.Time { return &value }
