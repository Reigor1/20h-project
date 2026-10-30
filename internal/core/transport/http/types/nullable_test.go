package core_http_types

import (
	"encoding/json"
	"testing"

	"github.com/Reigor1/20h-project/internal/core/domain"
)

func TestNullableUnmarshalJSON(t *testing.T) {
	type request struct {
		Name Nullable[string]
		Age  Nullable[int]
	}
	tests := []struct {
		name          string
		json          string
		wantNameSet   bool
		wantName      *string
		wantAgeSet    bool
		wantAge       *int
		wantDecodeErr bool
	}{
		{
			name:        "ordinary values",
			json:        `{"name":"Ada","age":37}`,
			wantNameSet: true,
			wantName:    stringPointer("Ada"),
			wantAgeSet:  true,
			wantAge:     intValuePointer(37),
		},
		{
			name:        "explicit null values",
			json:        `{"name":null,"age":null}`,
			wantNameSet: true,
			wantAgeSet:  true,
		},
		{
			name: "omitted fields",
			json: `{}`,
		},
		{
			name:          "incompatible JSON type",
			json:          `{"name":12}`,
			wantNameSet:   true,
			wantDecodeErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var decoded request
			err := json.Unmarshal([]byte(testCase.json), &decoded)
			if (err != nil) != testCase.wantDecodeErr {
				t.Fatalf("json.Unmarshal() error = %v, wantError %v", err, testCase.wantDecodeErr)
			}
			if decoded.Name.Set != testCase.wantNameSet {
				t.Errorf("Name.Set = %v, want %v", decoded.Name.Set, testCase.wantNameSet)
			}
			assertStringPointer(t, "Name.Value", decoded.Name.Value, testCase.wantName)
			if decoded.Age.Set != testCase.wantAgeSet {
				t.Errorf("Age.Set = %v, want %v", decoded.Age.Set, testCase.wantAgeSet)
			}
			assertIntPointer(t, "Age.Value", decoded.Age.Value, testCase.wantAge)
		})
	}
}

func TestNullableUnmarshalJSONSetsStateBeforeDecodeError(t *testing.T) {
	value := "previous"
	nullable := Nullable[string]{
		Nullable: domain.Nullable[string]{
			Value: &value,
			Set:   true,
		},
	}

	err := json.Unmarshal([]byte(`12`), &nullable)
	if err == nil {
		t.Fatal("json.Unmarshal() error = nil, want incompatible-type error")
	}
	if !nullable.Set || nullable.Value == nil || *nullable.Value != value {
		t.Fatalf(
			"state after failed decode = {Value: %v, Set: %v}, want previous value retained and Set true",
			nullable.Value,
			nullable.Set,
		)
	}
}

func TestNullableToDomain(t *testing.T) {
	value := "updated"
	tests := []struct {
		name      string
		input     Nullable[string]
		wantSet   bool
		wantValue *string
	}{
		{
			name: "unset value",
		},
		{
			name: "explicit null",
			input: Nullable[string]{
				Nullable: domain.Nullable[string]{Set: true},
			},
			wantSet: true,
		},
		{
			name: "ordinary value",
			input: Nullable[string]{
				Nullable: domain.Nullable[string]{
					Value: &value,
					Set:   true,
				},
			},
			wantSet:   true,
			wantValue: &value,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := testCase.input.ToDomain()
			if got.Set != testCase.wantSet {
				t.Errorf("ToDomain().Set = %v, want %v", got.Set, testCase.wantSet)
			}
			assertStringPointer(t, "ToDomain().Value", got.Value, testCase.wantValue)
			if got.Value != testCase.input.Value {
				t.Errorf(
					"ToDomain() changed value pointer: got %p, want %p",
					got.Value,
					testCase.input.Value,
				)
			}
		})
	}
}

func assertStringPointer(t *testing.T, name string, got, want *string) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s = %q, want nil", name, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Errorf("%s = %v, want %q", name, got, *want)
	}
}

func assertIntPointer(t *testing.T, name string, got, want *int) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s = %d, want nil", name, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Errorf("%s = %v, want %d", name, got, *want)
	}
}

func stringPointer(value string) *string {
	return &value
}

func intValuePointer(value int) *int {
	return &value
}
