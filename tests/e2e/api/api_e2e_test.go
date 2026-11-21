package e2e_api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Reigor1/20h-project/internal/testutil"
)

// TestApiE2E starts the application and exercises the public Users, Tasks, and Statistics API flows.
func TestApiE2E(t *testing.T) {
	baseURL := testutil.StartApp(t)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	t.Run("Users", func(t *testing.T) {
		testUsersAPI(t, client, baseURL)
	})

	t.Run("Tasks", func(t *testing.T) {
		testTasksAPI(t, client, baseURL)
	})

	t.Run("Statistics", func(t *testing.T) {
		testStatisticsAPI(t, client, baseURL)
	})
}
