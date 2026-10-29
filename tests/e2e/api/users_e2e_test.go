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

type expectedSuccessCase struct {
	name   string
	method string
	path   string
	body   any
	status int
	check  func(t *testing.T, result apiResult)
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

	runExpectedSuccesses := func(t *testing.T, cases []expectedSuccessCase) {
		t.Helper()

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				result, err := request(tc.method, tc.path, tc.body)
				if err != nil {
					t.Fatalf("request %s %s: %v", tc.method, tc.path, err)
				}

				assertStatus(t, result, tc.method, tc.path, tc.status)

				if tc.check != nil {
					tc.check(t, result)
				}
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
		t.Run("Success", func(t *testing.T) {
			runExpectedSuccesses(t, []expectedSuccessCase{
				{
					name:   "creates user with phone number",
					method: http.MethodPost,
					path:   usersPath,
					body: userRequest{
						FullName:    "E2E Create",
						PhoneNumber: stringPointer("+1234567890"),
					},
					status: http.StatusCreated,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var got userResponse
						if err := json.Unmarshal(result.Body, &got); err != nil {
							t.Fatalf("decode created user: %v; body: %s", err, result.Body)
						}

						if got.ID <= 0 {
							t.Fatalf("created user ID = %d, want positive ID", got.ID)
						}
						registerUserCleanup(t, got.ID)

						if got.FullName != "E2E Create" ||
							got.PhoneNumber == nil ||
							*got.PhoneNumber != "+1234567890" {
							t.Fatalf("created user = %+v", got)
						}
					},
				},
				{
					name:   "creates user without phone number",
					method: http.MethodPost,
					path:   usersPath,
					body:   userRequest{FullName: "E2E Create"},
					status: http.StatusCreated,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var got userResponse
						if err := json.Unmarshal(result.Body, &got); err != nil {
							t.Fatalf("decode created user: %v; body: %s", err, result.Body)
						}

						if got.ID <= 0 {
							t.Fatalf("created user ID = %d, want positive ID", got.ID)
						}
						registerUserCleanup(t, got.ID)

						if got.FullName != "E2E Create" || got.PhoneNumber != nil {
							t.Fatalf("created user = %+v", got)
						}
					},
				},
			})
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
			created := []userResponse{
				createUser(t, "E2E Page A", nil),
				createUser(t, "E2E Page B", nil),
				createUser(t, "E2E Page C", nil),
			}

			if !(created[0].ID < created[1].ID && created[1].ID < created[2].ID) {
				t.Fatalf(
					"created IDs = %d, %d, %d; want ascending IDs",
					created[0].ID,
					created[1].ID,
					created[2].ID,
				)
			}

			pages := []struct {
				name     string
				offset   int
				expected []userResponse
			}{
				{
					name:     "returns first page in ID order",
					offset:   0,
					expected: created[:2],
				},
				{
					name:     "returns second page in ID order",
					offset:   2,
					expected: created[2:],
				},
			}

			cases := make([]expectedSuccessCase, 0, len(pages))

			for _, page := range pages {
				page := page

				query := url.Values{
					"limit":  {"2"},
					"offset": {fmt.Sprint(page.offset)},
				}
				path := usersPath + "?" + query.Encode()

				cases = append(cases, expectedSuccessCase{
					name:   page.name,
					method: http.MethodGet,
					path:   path,
					status: http.StatusOK,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var got []userResponse
						if err := json.Unmarshal(result.Body, &got); err != nil {
							t.Fatalf("decode users: %v; body: %s", err, result.Body)
						}

						if len(got) != len(page.expected) {
							t.Fatalf(
								"page returned %d users, want %d: %+v",
								len(got),
								len(page.expected),
								got,
							)
						}

						for i, want := range page.expected {
							if got[i].ID != want.ID ||
								got[i].FullName != want.FullName {
								t.Errorf(
									"user[%d] = %+v, want ID=%d and FullName=%q",
									i,
									got[i],
									want.ID,
									want.FullName,
								)
							}
						}
					},
				})
			}

			runExpectedSuccesses(t, cases)
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
			created := createUser(t, "E2E Get", stringPointer("+1234567890"))
			path := fmt.Sprintf("%s/%d", usersPath, created.ID)

			runExpectedSuccesses(t, []expectedSuccessCase{
				{
					name:   "retrieves created user",
					method: http.MethodGet,
					path:   path,
					status: http.StatusOK,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var got userResponse
						if err := json.Unmarshal(result.Body, &got); err != nil {
							t.Fatalf("decode user: %v; body: %s", err, result.Body)
						}

						if got.ID != created.ID ||
							got.FullName != created.FullName ||
							!reflect.DeepEqual(got.PhoneNumber, created.PhoneNumber) {
							t.Fatalf("retrieved user = %+v, want %+v", got, created)
						}
					},
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{
				{
					name:   "returns not found for missing user",
					method: http.MethodGet,
					path:   usersPath + "/2147483647",
					status: http.StatusNotFound,
				},
				{
					name:   "rejects noninteger user ID",
					method: http.MethodGet,
					path:   usersPath + "/abc",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects decimal user ID",
					method: http.MethodGet,
					path:   usersPath + "/1.5",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects overflowing user ID",
					method: http.MethodGet,
					path:   usersPath + "/99999999999999999999999999999999",
					status: http.StatusBadRequest,
				},
				{
					name:   "returns not found for zero user ID",
					method: http.MethodGet,
					path:   usersPath + "/0",
					status: http.StatusNotFound,
				},
				{
					name:   "returns not found for negative user ID",
					method: http.MethodGet,
					path:   usersPath + "/-1",
					status: http.StatusNotFound,
				},
			})
		})
	})

	t.Run("DeleteUser", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			user := createUser(t, "E2E Delete", nil)
			path := fmt.Sprintf("%s/%d", usersPath, user.ID)

			runExpectedSuccesses(t, []expectedSuccessCase{
				{
					name:   "deletes user and makes it unavailable",
					method: http.MethodDelete,
					path:   path,
					status: http.StatusNoContent,
					check: func(t *testing.T, _ apiResult) {
						t.Helper()

						result, err := request(http.MethodGet, path, nil)
						if err != nil {
							t.Fatalf("GET deleted user: %v", err)
						}

						assertStatus(
							t,
							result,
							http.MethodGet,
							path,
							http.StatusNotFound,
						)
						decodeError(t, result)
					},
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{
				{
					name:   "returns not found when deleting missing user",
					method: http.MethodDelete,
					path:   usersPath + "/2147483647",
					status: http.StatusNotFound,
				},
				{
					name:   "rejects noninteger user ID",
					method: http.MethodDelete,
					path:   usersPath + "/abc",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects decimal user ID",
					method: http.MethodDelete,
					path:   usersPath + "/1.5",
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects overflowing user ID",
					method: http.MethodDelete,
					path:   usersPath + "/99999999999999999999999999999999",
					status: http.StatusBadRequest,
				},
				{
					name:   "returns not found when deleting user with zero ID",
					method: http.MethodDelete,
					path:   usersPath + "/0",
					status: http.StatusNotFound,
				},
				{
					name:   "returns not found when deleting user with negative ID",
					method: http.MethodDelete,
					path:   usersPath + "/-1",
					status: http.StatusNotFound,
				},
			})
		})
	})

	t.Run("PatchUser", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			user := createUser(t, "E2E Original", stringPointer("+1234567890"))
			path := fmt.Sprintf("%s/%d", usersPath, user.ID)

			runExpectedSuccesses(t, []expectedSuccessCase{
				{
					name:   "updates name and preserves omitted phone",
					method: http.MethodPatch,
					path:   path,
					body:   map[string]string{"full_name": "E2E Patched"},
					status: http.StatusOK,
					check: func(t *testing.T, result apiResult) {
						t.Helper()

						var patched userResponse
						if err := json.Unmarshal(result.Body, &patched); err != nil {
							t.Fatalf("decode patched user: %v; body: %s", err, result.Body)
						}

						if patched.ID != user.ID ||
							patched.FullName != "E2E Patched" ||
							!reflect.DeepEqual(patched.PhoneNumber, user.PhoneNumber) {
							t.Fatalf("patched user = %+v", patched)
						}

						getResult, err := request(http.MethodGet, path, nil)
						if err != nil {
							t.Fatalf("GET patched user: %v", err)
						}

						assertStatus(t, getResult, http.MethodGet, path, http.StatusOK)

						var persisted userResponse
						if err := json.Unmarshal(getResult.Body, &persisted); err != nil {
							t.Fatalf("decode persisted user: %v; body: %s", err, getResult.Body)
						}

						if persisted.ID != user.ID ||
							persisted.FullName != "E2E Patched" ||
							!reflect.DeepEqual(persisted.PhoneNumber, user.PhoneNumber) {
							t.Fatalf("persisted user = %+v", persisted)
						}
					},
				},
			})
		})
		
		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, []expectedErrorCase{
				{
					name:   "returns not found when patching missing user",
					method: http.MethodPatch,
					path:   usersPath + "/2147483647",
					body:   map[string]string{"full_name": "E2E Missing"},
					status: http.StatusNotFound,
				},
				{
					name:   "rejects noninteger user ID",
					method: http.MethodPatch,
					path:   usersPath + "/abc",
					body:   map[string]string{"full_name": "E2E Patched"},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects decimal user ID",
					method: http.MethodPatch,
					path:   usersPath + "/1.5",
					body:   map[string]string{"full_name": "E2E Patched"},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects overflowing user ID",
					method: http.MethodPatch,
					path:   usersPath + "/99999999999999999999999999999999",
					body:   map[string]string{"full_name": "E2E Patched"},
					status: http.StatusBadRequest,
				},
				{
					name:   "returns not found for zero user ID",
					method: http.MethodPatch,
					path:   usersPath + "/0",
					body:   map[string]string{"full_name": "E2E Patched"},
					status: http.StatusNotFound,
				},
				{
					name:   "returns not found for negative user ID",
					method: http.MethodPatch,
					path:   usersPath + "/-1",
					body:   map[string]string{"full_name": "E2E Patched"},
					status: http.StatusNotFound,
				},
				{
					name:   "rejects full name shorter than minimum",
					method: http.MethodPatch,
					path:   usersPath + "/2147483647",
					body:   map[string]string{"full_name": "AB"},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects empty full name",
					method: http.MethodPatch,
					path:   usersPath + "/2147483647",
					body:   map[string]string{"full_name": ""},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects full name exceeding maximum length",
					method: http.MethodPatch,
					path:   usersPath + "/2147483647",
					body:   map[string]string{"full_name": strings.Repeat("A", 101)},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects invalid phone number",
					method: http.MethodPatch,
					path:   usersPath + "/2147483647",
					body: map[string]string{
						"phone_number": "not-a-phone",
					},
					status: http.StatusBadRequest,
				},
				{
					name:   "rejects phone number with wrong JSON type",
					method: http.MethodPatch,
					path:   usersPath + "/2147483647",
					body: map[string]any{
						"full_name":    "E2E Patched",
						"phone_number": 1234567890,
					},
					status: http.StatusBadRequest,
				},
			})
		})
	})
}

func stringPointer(value string) *string { return &value }
