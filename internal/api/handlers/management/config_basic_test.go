package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestGetLatestVersionFallsBackToReleasePage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer apiServer.Close()

	pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v6.9.31", http.StatusFound)
		case "/releases/tag/v6.9.31":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer pageServer.Close()

	originalAPIURL := latestReleaseURL
	originalPageURL := latestReleasePageURL
	t.Cleanup(func() {
		latestReleaseURL = originalAPIURL
		latestReleasePageURL = originalPageURL
	})

	latestReleaseURL = apiServer.URL
	latestReleasePageURL = pageServer.URL + "/releases/latest"

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/latest-version", nil)

	handler := &Handler{cfg: &config.Config{}}
	handler.GetLatestVersion(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := response["latest-version"]; got != "v6.9.31" {
		t.Fatalf("expected fallback latest-version v6.9.31, got %q", got)
	}
}

func TestExtractReleaseVersionFromURL(t *testing.T) {
	t.Parallel()

	testCases := map[string]string{
		"https://github.com/router-for-me/CLIProxyAPI/releases/tag/v6.9.30": "v6.9.30",
		"https://github.com/router-for-me/CLIProxyAPI/releases/latest":      "",
		"https://github.com/router-for-me/CLIProxyAPI/tag/v6.9.30":          "v6.9.30",
	}

	for rawURL, want := range testCases {
		rawURL := rawURL
		want := want
		t.Run(rawURL, func(t *testing.T) {
			t.Parallel()

			u, err := http.NewRequest(http.MethodGet, rawURL, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}

			if got := extractReleaseVersionFromURL(u.URL); got != want {
				t.Fatalf("expected %q, got %q", want, got)
			}
		})
	}
}
