package repository_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/core/domain"
	core_errors "github.com/Reigor1/20h-project/internal/core/errors"
	core_postgres_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool"
	core_pgx_pool "github.com/Reigor1/20h-project/internal/core/repository/postgres/pool/pgx"
	users_postgres_repository "github.com/Reigor1/20h-project/internal/features/users/repository/postgres"
	"github.com/Reigor1/20h-project/internal/testutil"
)

func TestUsersRepositoryIntegration(t *testing.T) {
	config := testutil.PostgresTestConfig(t)
	if !strings.HasSuffix(strings.ToLower(config.Database), "_test") {
		t.Fatalf("refusing to modify database %q: test database name must end in _test", config.Database)
	}

	connectionContext, cancelConnection := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelConnection()
	pool, err := core_pgx_pool.NewPool(connectionContext, config, nil)
	if err != nil {
		t.Fatalf("connect to configured PostgreSQL test database: %v", err)
	}
	t.Cleanup(pool.Close)

	ctx, cancelOperations := context.WithTimeout(context.Background(), time.Minute)
	defer cancelOperations()

	var connectedDatabase string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&connectedDatabase); err != nil {
		t.Fatalf("read connected database name: %v", err)
	}
	if connectedDatabase != config.Database {
		t.Fatalf(
			"connected database = %q, want configured test database %q",
			connectedDatabase,
			config.Database,
		)
	}

	testutil.PreparePostgresSchema(t, ctx, pool, "testdata/users_schema.sql")

	repository := users_postgres_repository.NewUsersRepository(pool)

	t.Run("CreateUser", func(t *testing.T) {
		phoneNumber := "+1234567890"
		tests := []struct {
			name        string
			fullName    string
			phoneNumber *string
		}{
			{
				name:        "creates a user with a phone number",
				fullName:    "Create with phone",
				phoneNumber: &phoneNumber,
			},
			{
				name:     "creates a user without a phone number",
				fullName: "Create without phone",
			},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				inputUser := domain.NewUserUninitialized(testCase.fullName, testCase.phoneNumber)

				createdUser, err := repository.CreateUser(ctx, inputUser)
				if err != nil {
					t.Fatalf("CreateUser() error = %v, want nil", err)
				}
				registerUserCleanup(t, ctx, pool, createdUser.ID)
				if createdUser.ID <= 0 || createdUser.ID == domain.UninitializedID {
					t.Fatalf("CreateUser() ID = %d, want database-generated ID", createdUser.ID)
				}
				if createdUser.Version != 1 {
					t.Fatalf("CreateUser() version = %d, want database default 1", createdUser.Version)
				}

				wantUser := domain.NewUser(
					createdUser.ID,
					1,
					testCase.fullName,
					testCase.phoneNumber,
				)
				if !reflect.DeepEqual(createdUser, wantUser) {
					t.Fatalf("CreateUser() = %+v, want %+v", createdUser, wantUser)
				}

				storedUser := readStoredUser(t, ctx, pool, createdUser.ID)
				if !reflect.DeepEqual(storedUser, createdUser) {
					t.Fatalf("stored user = %+v, want returned user %+v", storedUser, createdUser)
				}
			})
		}

		t.Run("database constraints reject invalid input", func(t *testing.T) {
			tests := []struct {
				name        string
				fullName    string
				phoneNumber *string
			}{
				{
					name:     "name shorter than the database constraint",
					fullName: "Al",
				},
				{
					name:        "phone does not match the database format",
					fullName:    "Invalid phone format",
					phoneNumber: stringPointer("1234567890"),
				},
			}

			for _, testCase := range tests {
				t.Run(testCase.name, func(t *testing.T) {
					inputUser := domain.NewUserUninitialized(testCase.fullName, testCase.phoneNumber)
					usersBeforeInsert := countUsers(t, ctx, pool)

					_, err := repository.CreateUser(ctx, inputUser)

					if err == nil {
						t.Fatal("CreateUser() error = nil, want database constraint error")
					}
					if !errors.Is(err, core_postgres_pool.ErrUnknown) {
						t.Fatalf("CreateUser() error = %v, want ErrUnknown from PostgreSQL adapter", err)
					}
					if !strings.Contains(err.Error(), "SQLSTATE 23514") {
						t.Fatalf("CreateUser() error = %v, want PostgreSQL CHECK violation SQLSTATE 23514", err)
					}
					if usersAfterInsert := countUsers(t, ctx, pool); usersAfterInsert != usersBeforeInsert {
						t.Fatalf(
							"user count after rejected insert = %d, want unchanged count %d",
							usersAfterInsert,
							usersBeforeInsert,
						)
					}
				})
			}
		})
	})

	t.Run("GetUser", func(t *testing.T) {
		phoneNumber := "+1234567890"
		for _, testCase := range []struct {
			name        string
			fullName    string
			phoneNumber *string
		}{
			{
				name:        "retrieves a user with a phone number",
				fullName:    "Get with phone",
				phoneNumber: &phoneNumber,
			},
			{
				name:     "retrieves a user with a NULL phone number",
				fullName: "Get without phone",
			},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				storedUser := insertUser(t, ctx, pool, testCase.fullName, testCase.phoneNumber)

				got, err := repository.GetUser(ctx, storedUser.ID)
				if err != nil {
					t.Fatalf("GetUser() error = %v, want nil", err)
				}
				if !reflect.DeepEqual(got, storedUser) {
					t.Fatalf("GetUser() = %+v, want %+v", got, storedUser)
				}
			})
		}

		t.Run("returns not found for a missing user", func(t *testing.T) {
			missingUser := insertUser(t, ctx, pool, "Get missing user", nil)
			deleteUserDirectly(t, ctx, pool, missingUser.ID)

			_, err := repository.GetUser(ctx, missingUser.ID)

			if !errors.Is(err, core_errors.ErrNotFound) {
				t.Fatalf("GetUser() error = %v, want ErrNotFound", err)
			}
		})
	})

	t.Run("GetUsers", func(t *testing.T) {
		t.Run("returns users ordered by ID with pagination and nullable fields", func(t *testing.T) {
			initialCount := countUsers(t, ctx, pool)
			phoneNumber := "+19876543210"

			firstUser := insertUser(t, ctx, pool, "List first user", &phoneNumber)
			secondUser := insertUser(t, ctx, pool, "List second user", nil)
			thirdUser := insertUser(t, ctx, pool, "List third user", nil)

			limit := 2
			offset := initialCount + 1

			got, err := repository.GetUsers(ctx, &limit, &offset)
			if err != nil {
				t.Fatalf("GetUsers() error = %v, want nil", err)
			}

			wantUsers := []domain.User{secondUser, thirdUser}
			if !reflect.DeepEqual(got, wantUsers) {
				t.Fatalf(
					"GetUsers() = %+v, want %+v; first inserted user was %+v",
					got,
					wantUsers,
					firstUser,
				)
			}
		})

		t.Run("returns an empty page after the final row", func(t *testing.T) {
			offset := countUsers(t, ctx, pool)

			got, err := repository.GetUsers(ctx, nil, &offset)
			if err != nil {
				t.Fatalf("GetUsers() error = %v, want nil", err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("GetUsers() = %#v, want a non-nil empty slice", got)
			}
		})
	})

	t.Run("PatchUser", func(t *testing.T) {
		t.Run("updates fields and increments version", func(t *testing.T) {
			originalPhoneNumber := "+1234567890"
			updatedPhoneNumber := "+19876543210"
			storedUser := insertUser(t, ctx, pool, "Patch original user", &originalPhoneNumber)
			updatedUser := domain.NewUser(
				storedUser.ID,
				storedUser.Version,
				"Patch updated user",
				&updatedPhoneNumber,
			)

			got, err := repository.PatchUser(ctx, storedUser.ID, updatedUser)
			if err != nil {
				t.Fatalf("PatchUser() error = %v, want nil", err)
			}
			wantUser := domain.NewUser(
				storedUser.ID,
				storedUser.Version+1,
				updatedUser.FullName,
				updatedUser.PhoneNumber,
			)
			assertPatchedUser(t, ctx, pool, got, wantUser)
		})

		t.Run("clears the phone number with NULL", func(t *testing.T) {
			phoneNumber := "+1234567890"
			storedUser := insertUser(t, ctx, pool, "Patch clear phone", &phoneNumber)
			updatedUser := domain.NewUser(
				storedUser.ID,
				storedUser.Version,
				storedUser.FullName,
				nil,
			)

			got, err := repository.PatchUser(ctx, storedUser.ID, updatedUser)
			if err != nil {
				t.Fatalf("PatchUser() error = %v, want nil", err)
			}
			wantUser := domain.NewUser(
				storedUser.ID,
				storedUser.Version+1,
				storedUser.FullName,
				nil,
			)
			assertPatchedUser(t, ctx, pool, got, wantUser)
		})

		t.Run("returns conflict for a stale version", func(t *testing.T) {
			storedUser := insertUser(t, ctx, pool, "Patch stale version", nil)
			staleUser := domain.NewUser(
				storedUser.ID,
				storedUser.Version-1,
				"Patch stale update",
				nil,
			)

			_, err := repository.PatchUser(ctx, storedUser.ID, staleUser)

			if !errors.Is(err, core_errors.ErrConflict) {
				t.Fatalf("PatchUser() error = %v, want ErrConflict", err)
			}
			if got := readStoredUser(t, ctx, pool, storedUser.ID); !reflect.DeepEqual(got, storedUser) {
				t.Fatalf("user after stale update = %+v, want unchanged %+v", got, storedUser)
			}
		})

		t.Run("returns conflict for a missing user", func(t *testing.T) {
			storedUser := insertUser(t, ctx, pool, "Patch missing user", nil)
			deleteUserDirectly(t, ctx, pool, storedUser.ID)

			_, err := repository.PatchUser(ctx, storedUser.ID, storedUser)

			if !errors.Is(err, core_errors.ErrConflict) {
				t.Fatalf("PatchUser() error = %v, want ErrConflict", err)
			}
		})
	})

	t.Run("DeleteUser", func(t *testing.T) {
		t.Run("deletes an existing user and reports a missing user", func(t *testing.T) {
			storedUser := insertUser(t, ctx, pool, "Delete integration user", nil)

			if err := repository.DeleteUser(ctx, storedUser.ID); err != nil {
				t.Fatalf("DeleteUser() error = %v, want nil", err)
			}
			if userExists(t, ctx, pool, storedUser.ID) {
				t.Fatalf("user %d still exists after DeleteUser()", storedUser.ID)
			}

			err := repository.DeleteUser(ctx, storedUser.ID)
			if !errors.Is(err, core_errors.ErrNotFound) {
				t.Fatalf("DeleteUser() for missing user error = %v, want ErrNotFound", err)
			}
		})
	})
}

func insertUser(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	fullName string,
	phoneNumber *string,
) domain.User {
	t.Helper()

	var user domain.User
	err := pool.QueryRow(
		ctx,
		`INSERT INTO todoapp.users (full_name, phone_number)
		 VALUES ($1, $2)
		 RETURNING id, version, full_name, phone_number`,
		fullName,
		phoneNumber,
	).Scan(
		&user.ID,
		&user.Version,
		&user.FullName,
		&user.PhoneNumber,
	)
	if err != nil {
		t.Fatalf("insert user fixture: %v", err)
	}

	registerUserCleanup(t, ctx, pool, user.ID)
	return user
}

func registerUserCleanup(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, id int) {
	t.Helper()

	t.Cleanup(func() {
		deleteUserDirectly(t, ctx, pool, id)
	})
}

func deleteUserDirectly(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, id int) {
	t.Helper()

	if _, err := pool.Exec(ctx, "DELETE FROM todoapp.users WHERE id = $1", id); err != nil {
		t.Errorf("delete user fixture %d: %v", id, err)
	}
}

func readStoredUser(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, id int) domain.User {
	t.Helper()

	var user domain.User
	err := pool.QueryRow(
		ctx,
		`SELECT id, version, full_name, phone_number
		 FROM todoapp.users
		 WHERE id = $1`,
		id,
	).Scan(
		&user.ID,
		&user.Version,
		&user.FullName,
		&user.PhoneNumber,
	)
	if err != nil {
		t.Fatalf("read stored user %d: %v", id, err)
	}
	return user
}

func countUsers(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM todoapp.users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	return count
}

func userExists(t *testing.T, ctx context.Context, pool *core_pgx_pool.Pool, id int) bool {
	t.Helper()

	var exists bool
	if err := pool.QueryRow(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM todoapp.users WHERE id = $1)",
		id,
	).Scan(&exists); err != nil {
		t.Fatalf("check whether user %d exists: %v", id, err)
	}
	return exists
}

func assertPatchedUser(
	t *testing.T,
	ctx context.Context,
	pool *core_pgx_pool.Pool,
	got domain.User,
	want domain.User,
) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PatchUser() = %+v, want %+v", got, want)
	}
	storedUser := readStoredUser(t, ctx, pool, want.ID)
	if !reflect.DeepEqual(storedUser, want) {
		t.Fatalf("stored patched user = %+v, want %+v", storedUser, want)
	}
}

func stringPointer(value string) *string {
	return &value
}
