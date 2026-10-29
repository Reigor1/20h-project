package e2e_api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

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

func testUsersAPI(t *testing.T, client *http.Client, baseURL string) {
	api := apiClient{httpClient: client, baseURL: baseURL}
	const usersPath = "/api/v1/users"

	t.Run("CreateUser", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			runExpectedSuccesses(t, api, []expectedSuccessCase{
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
						registerUserCleanup(t, &api, got.ID)

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
						registerUserCleanup(t, &api, got.ID)

						if got.FullName != "E2E Create" || got.PhoneNumber != nil {
							t.Fatalf("created user = %+v", got)
						}
					},
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
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
				createUser(t, &api, "E2E Page A", nil),
				createUser(t, &api, "E2E Page B", nil),
				createUser(t, &api, "E2E Page C", nil),
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

			runExpectedSuccesses(t, api, cases)
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
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
			created := createUser(t, &api, "E2E Get", stringPointer("+1234567890"))
			path := fmt.Sprintf("%s/%d", usersPath, created.ID)

			runExpectedSuccesses(t, api, []expectedSuccessCase{
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
			runExpectedErrors(t, api, []expectedErrorCase{
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
			user := createUser(t, &api, "E2E Delete", nil)
			path := fmt.Sprintf("%s/%d", usersPath, user.ID)

			runExpectedSuccesses(t, api, []expectedSuccessCase{
				{
					name:   "deletes user and makes it unavailable",
					method: http.MethodDelete,
					path:   path,
					status: http.StatusNoContent,
					check: func(t *testing.T, _ apiResult) {
						t.Helper()

						result, err := api.request(http.MethodGet, path, nil)
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
						assertErrorResponse(t, result)
					},
				},
			})
		})

		t.Run("ExpectedErrors", func(t *testing.T) {
			runExpectedErrors(t, api, []expectedErrorCase{
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
			user := createUser(t, &api, "E2E Original", stringPointer("+1234567890"))
			path := fmt.Sprintf("%s/%d", usersPath, user.ID)

			runExpectedSuccesses(t, api, []expectedSuccessCase{
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

						getResult, err := api.request(http.MethodGet, path, nil)
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
			runExpectedErrors(t, api, []expectedErrorCase{
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
