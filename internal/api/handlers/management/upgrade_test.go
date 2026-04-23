package management

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestPostUpgradeStartsDetachedJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalTrigger := triggerUpgradeJob
	originalCheck := checkUpgradeReady
	t.Cleanup(func() {
		triggerUpgradeJob = originalTrigger
		checkUpgradeReady = originalCheck
	})

	checkUpgradeReady = func() error { return nil }
	triggerUpgradeJob = func() (string, error) {
		return "job-123", nil
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/upgrade", nil)

	handler := &Handler{cfg: &config.Config{}}
	handler.PostUpgrade(ctx)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPostUpgradeRejectsDuplicateRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/upgrade", nil)

	handler := &Handler{cfg: &config.Config{}, upgradeInProgress: true}
	handler.PostUpgrade(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPostUpgradeFailsPrecheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalTrigger := triggerUpgradeJob
	originalCheck := checkUpgradeReady
	t.Cleanup(func() {
		triggerUpgradeJob = originalTrigger
		checkUpgradeReady = originalCheck
	})

	checkUpgradeReady = func() error {
		return fmt.Errorf("repo dirty")
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/upgrade", nil)

	handler := &Handler{cfg: &config.Config{}}
	handler.PostUpgrade(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
}
