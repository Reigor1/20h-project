package e2e_api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

type apiResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type userRequest struct {
	FullName    string  `json:"full_name"`
	PhoneNumber *string `json:"phone_number,omitempty"`
}

type userResponse struct {
	ID          int     `json:"id"`
	Version     int     `json:"version"`
	FullName    string  `json:"full_name"`
	PhoneNumber *string `json:"phone_number,omitempty"`
}

type expectedErrorCase struct {
	name   string
	method string
	path   string
	body   any
	status int
}
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func testUsersAPI(t *testing.T, client *http.Client, baseURL string) {
	const usersPath = "/api/v1/users"

	request := func(method, path string, body any) (apiResult, error) {
		var encodedBody io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return apiResult{}, fmt.Errorf("encode %s %s body: %w", method, path, err)
			}
			encodedBody = bytes.NewReader(encoded)
		}

		req, err := http.NewRequest(method, baseURL+path, encodedBody)
		if err != nil {
			return apiResult{}, fmt.Errorf("create %s %s request: %w", method, path, err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := client.Do(req)
		if err != nil {
			return apiResult{}, fmt.Errorf("send %s %s request: %w", method, path, err)
		}
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return apiResult{}, fmt.Errorf("read %s %s response: %w", method, path, err)
		}
		return apiResult{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: responseBody}, nil
	}

	assertStatus := func(t *testing.T, result apiResult, method, path string, want int) {
		t.Helper()
		if result.StatusCode != want {
			t.Fatalf("%s %s status = %d, want %d; body: %s", method, path, result.StatusCode, want, result.Body)
		}
	}
	decodeError := func(t *testing.T, result apiResult) errorResponse {
		t.Helper()
		var got errorResponse
		if err := json.Unmarshal(result.Body, &got); err != nil {
			t.Fatalf("decode error response: %v; body: %s", err, result.Body)
		}
		if strings.TrimSpace(got.Error) == "" ||
			strings.TrimSpace(got.Message) == "" {
			t.Fatalf("error response = %+v, want nonempty error and message", got)
		}
		return got
	}
	runExpectedErrors := func(t *testing.T, cases []expectedErrorCase) {
		t.Helper()
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				result, err := request(tc.method, tc.path, tc.body)
				if err != nil {
					t.Fatalf("request %s %s: %v", tc.method, tc.path, err)
				}
				assertStatus(t, result, tc.method, tc.path, tc.status)
				decodeError(t, result)
			})
		}
	}
	registerUserCleanup := func(t *testing.T, userID int) {
		t.Helper()

		t.Cleanup(func() {
			path := fmt.Sprintf("%s/%d", usersPath, userID)
			result, err := request(http.MethodDelete, path, nil)
			if err != nil {
				t.Errorf("cleanup DELETE %s: %v", path, err)
				return
			}

			if result.StatusCode != http.StatusNoContent &&
				result.StatusCode != http.StatusNotFound {
				t.Errorf(
					"cleanup DELETE %s status = %d, want %d or %d; body: %s",
					path,
					result.StatusCode,
					http.StatusNoContent,
					http.StatusNotFound,
					result.Body,
				)
			}
		})
	}
	createUser := func(t *testing.T, name string, phone *string) userResponse {
		t.Helper()

		result, err := request(
			http.MethodPost,
			usersPath,
			userRequest{FullName: name, PhoneNumber: phone},
		)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}

		assertStatus(t, result, http.MethodPost, usersPath, http.StatusCreated)

		var user userResponse
		if err := json.Unmarshal(result.Body, &user); err != nil {
			t.Fatalf("decode created user: %v; body: %s", err, result.Body)
		}

		if user.ID <= 0 {
			t.Fatalf("created user ID = %d, want positive ID", user.ID)
		}

		registerUserCleanup(t, user.ID)

		return user
	}

	t.Run("CreateUser", func(t *testing.T) {
		t.Run("creates user with phone number", func(t *testing.T) {
			input := userRequest{
				FullName:    "E2E Create",
				PhoneNumber: stringPointer("+1234567890"),
			}

			result, err := request(http.MethodPost, usersPath, input)
			if err != nil {
				t.Fatal(err)
			}

			assertStatus(t, result, http.MethodPost, usersPath, http.StatusCreated)

			var got userResponse
			if err := json.Unmarshal(result.Body, &got); err != nil {
				t.Fatalf("decode created user: %v; body: %s", err, result.Body)
			}

			if got.ID <= 0 {
				t.Fatalf("created user ID = %d, want positive ID", got.ID)
			}
			registerUserCleanup(t, got.ID)

			if got.FullName != input.FullName ||
				!reflect.DeepEqual(got.PhoneNumber, input.PhoneNumber) {
				t.Fatalf("created user = %+v, want full name %q and phone number %q",
					got, input.FullName, *input.PhoneNumber)
			}
		})

		t.Run("creates user without phone number", func(t *testing.T) {
			input := userRequest{
				FullName: "E2E Create",
			}

			result, err := request(http.MethodPost, usersPath, input)
			if err != nil {
				t.Fatal(err)
			}

			assertStatus(t, result, http.MethodPost, usersPath, http.StatusCreated)

			var got userResponse
			if err := json.Unmarshal(result.Body, &got); err != nil {
				t.Fatalf("decode created user: %v; body: %s", err, result.Body)
			}

			if got.ID <= 0 {
				t.Fatalf("created user ID = %d, want positive ID", got.ID)
			}
			registerUserCleanup(t, got.ID)

			if got.FullName != input.FullName || got.PhoneNumber != nil {
				t.Fatalf("created user = %+v, want full name %q and nil phone number",
					got, input.FullName)
			}
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{
				{
					name:   "rejects full name shorter than minimum",
					method: http.MethodPost,
					path:   usersPath, body: userRequest{FullName: "AB"},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects missing full name",
					method: http.MethodPost,
					path:   usersPath,
					body: map[string]any{
						"phone_number": "+1234567890",
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects empty full name",
					method: http.MethodPost,
					path:   usersPath,
					body:   userRequest{FullName: ""},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects full name exceeding maximum length",
					method: http.MethodPost,
					path:   usersPath,
					body:   userRequest{FullName: strings.Repeat("A", 101)},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects invalid phone number",
					method: http.MethodPost,
					path:   usersPath,
					body: userRequest{
						FullName:    "E2E Invalid Phone",
						PhoneNumber: stringPointer("not-a-phone"),
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects phone number with wrong JSON type",
					method: http.MethodPost,
					path:   usersPath,
					body: map[string]any{
						"full_name":    "E2E Invalid Phone Type",
						"phone_number": 1234567890,
					},
					status: http.StatusBadRequest,
				},
			})
		})
	})

	t.Run("GetUsers", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			t.Run("returns test users across ordered pages", func(t *testing.T) {
				created := []userResponse{
					createUser(t, "E2E Page A", nil),
					createUser(t, "E2E Page B", nil),
					createUser(t, "E2E Page C", nil),
				}
				if !(created[0].ID < created[1].ID && created[1].ID < created[2].ID) {
					t.Fatalf("created IDs = %d, %d, %d; want ascending IDs", created[0].ID, created[1].ID, created[2].ID)
				}
				for page, expected := range [][]userResponse{{created[0], created[1]}, {created[2]}} {
					query := url.Values{"limit": {"2"}, "offset": {fmt.Sprint(page * 2)}}
					path := usersPath + "?" + query.Encode()
					result, err := request(http.MethodGet, path, nil)
					if err != nil {
						t.Fatal(err)
					}
					assertStatus(t, result, http.MethodGet, path, http.StatusOK)
					var got []userResponse
					if err := json.Unmarshal(result.Body, &got); err != nil {
						t.Fatalf("decode users: %v", err)
					}
					if len(got) != len(expected) {
						t.Fatalf("page %d returned %d users, want %d: %+v", page, len(got), len(expected), got)
					}
					for index := range expected {
						if got[index].ID != expected[index].ID || got[index].FullName != expected[index].FullName {
							t.Fatalf("page %d user[%d] = %+v, want %+v", page, index, got[index], expected[index])
						}
					}
				}
			})
		})
		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{
				{
					name:   "rejects noninteger limit",
					method: http.MethodGet,
					path:   usersPath + "?limit=abc",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects noninteger offset",
					method: http.MethodGet,
					path:   usersPath + "?offset=abc",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects decimal limit",
					method: http.MethodGet,
					path:   usersPath + "?limit=1.5",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects decimal offset",
					method: http.MethodGet,
					path:   usersPath + "?offset=1.5",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects overflowing limit",
					method: http.MethodGet,
					path:   usersPath + "?limit=99999999999999999999999999999999",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects overflowing offset",
					method: http.MethodGet,
					path:   usersPath + "?offset=99999999999999999999999999999999",
					status: http.StatusBadRequest,
				},
			})
		})
	})

	t.Run("GetUser", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			t.Run("retrieves created user", func(t *testing.T) {
				created := createUser(t, "E2E Get", stringPointer("+1234567890"))
				path := fmt.Sprintf("%s/%d", usersPath, created.ID)
				result, err := request(http.MethodGet, path, nil)
				if err != nil {
					t.Fatal(err)
				}
				assertStatus(t, result, http.MethodGet, path, http.StatusOK)
				var got userResponse
				if err := json.Unmarshal(result.Body, &got); err != nil {
					t.Fatalf("decode user: %v", err)
				}
				if got.ID != created.ID || got.FullName != created.FullName || !reflect.DeepEqual(got.PhoneNumber, created.PhoneNumber) {
					t.Fatalf("retrieved user = %+v, want %+v", got, created)
				}
			})
		})
		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{{
				name: "returns not found for missing user", method: http.MethodGet,
				path: usersPath + "/2147483647", status: http.StatusNotFound,
			}})
		})
	})

	t.Run("DeleteUser", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			t.Run("deletes user and makes it unavailable", func(t *testing.T) {
				user := createUser(t, "E2E Delete", nil)
				path := fmt.Sprintf("%s/%d", usersPath, user.ID)
				result, err := request(http.MethodDelete, path, nil)
				if err != nil {
					t.Fatal(err)
				}
				assertStatus(t, result, http.MethodDelete, path, http.StatusNoContent)
				getResult, err := request(http.MethodGet, path, nil)
				if err != nil {
					t.Fatal(err)
				}
				assertStatus(t, getResult, http.MethodGet, path, http.StatusNotFound)
				decodeError(t, getResult)
			})
		})
		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{{
				name: "returns not found when deleting missing user", method: http.MethodDelete,
				path: usersPath + "/2147483647", status: http.StatusNotFound,
			}})
		})
	})

	t.Run("PatchUser", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			t.Run("updates name and preserves omitted phone", func(t *testing.T) {
				user := createUser(t, "E2E Original", stringPointer("+1234567890"))
				path := fmt.Sprintf("%s/%d", usersPath, user.ID)
				result, err := request(http.MethodPatch, path, map[string]string{"full_name": "E2E Patched"})
				if err != nil {
					t.Fatal(err)
				}
				assertStatus(t, result, http.MethodPatch, path, http.StatusOK)
				var patched userResponse
				if err := json.Unmarshal(result.Body, &patched); err != nil {
					t.Fatalf("decode patched user: %v", err)
				}
				if patched.FullName != "E2E Patched" || !reflect.DeepEqual(patched.PhoneNumber, user.PhoneNumber) {
					t.Fatalf("patched user = %+v", patched)
				}
				getResult, err := request(http.MethodGet, path, nil)
				if err != nil {
					t.Fatal(err)
				}
				assertStatus(t, getResult, http.MethodGet, path, http.StatusOK)
				var persisted userResponse
				if err := json.Unmarshal(getResult.Body, &persisted); err != nil {
					t.Fatalf("decode persisted user: %v", err)
				}
				if persisted.FullName != patched.FullName || !reflect.DeepEqual(persisted.PhoneNumber, user.PhoneNumber) {
					t.Fatalf("persisted user = %+v", persisted)
				}
			})
		})
		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{{
				name: "returns not found when patching missing user", method: http.MethodPatch,
				path: usersPath + "/2147483647", body: map[string]string{"full_name": "E2E Missing"},
				status: http.StatusNotFound,
			}})
		})
	})
}

func stringPointer(value string) *string { return &value }
