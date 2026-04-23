package management

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	upgradeWorkspaceDir = "/workspace"
	upgradeScriptPath   = "/workspace/scripts/update-main-and-docker.sh"
	upgradeLogPath      = "/workspace/logs/oneclick-upgrade.log"
	triggerUpgradeJob   = defaultTriggerUpgradeJob
	checkUpgradeReady   = defaultCheckUpgradeReady
)

type containerMount struct {
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

func (h *Handler) PostUpgrade(c *gin.Context) {
	if h == nil {
		c.JSON(500, gin.H{"error": "handler_unavailable"})
		return
	}

	h.upgradeMu.Lock()
	if h.upgradeInProgress {
		h.upgradeMu.Unlock()
		c.JSON(409, gin.H{
			"error":   "upgrade_in_progress",
			"message": "升级任务已经在进行中，请稍后刷新页面查看结果",
		})
		return
	}
	h.upgradeInProgress = true
	h.upgradeMu.Unlock()

	if err := checkUpgradeReady(); err != nil {
		h.upgradeMu.Lock()
		h.upgradeInProgress = false
		h.upgradeMu.Unlock()
		c.JSON(409, gin.H{
			"error":   "upgrade_precheck_failed",
			"message": err.Error(),
		})
		return
	}

	jobID, err := triggerUpgradeJob()
	if err != nil {
		h.upgradeMu.Lock()
		h.upgradeInProgress = false
		h.upgradeMu.Unlock()
		c.JSON(500, gin.H{
			"error":   "upgrade_start_failed",
			"message": err.Error(),
		})
		return
	}

	c.JSON(202, gin.H{
		"status":   "accepted",
		"job-id":   jobID,
		"log-path": upgradeLogPath,
		"message":  "已触发一键升级，服务将短暂重启，请稍后刷新页面",
	})
}

func defaultTriggerUpgradeJob() (string, error) {
	if _, err := os.Stat(upgradeScriptPath); err != nil {
		return "", fmt.Errorf("upgrade script not found: %w", err)
	}

	mounts, err := resolveHostMounts()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(upgradeLogPath), 0o755); err != nil {
		return "", fmt.Errorf("prepare upgrade log directory: %w", err)
	}

	runCommand := fmt.Sprintf(
		"mkdir -p %s && chmod +x %s && ALLOW_DIRTY_WORKTREE=true %s >> %s 2>&1",
		shellEscape(filepath.Dir(upgradeLogPath)),
		shellEscape(upgradeScriptPath),
		shellEscape(upgradeScriptPath),
		shellEscape(upgradeLogPath),
	)

	cmd := exec.Command(
		"docker", "compose", "run",
		"-d", "--rm",
		"-e", "ALLOW_DIRTY_WORKTREE=true",
		"-e", "HOST_WORKSPACE_PATH="+mounts.workspace,
		"-e", "CLI_PROXY_CONFIG_PATH="+mounts.configFile,
		"-e", "CLI_PROXY_AUTH_PATH="+mounts.authDir,
		"-e", "CLI_PROXY_LOG_PATH="+mounts.logsDir,
		"cli-proxy-updater",
		"sh", "-lc", runCommand,
	)
	cmd.Dir = upgradeWorkspaceDir
	cmd.Env = append(
		os.Environ(),
		"HOST_WORKSPACE_PATH="+mounts.workspace,
		"CLI_PROXY_CONFIG_PATH="+mounts.configFile,
		"CLI_PROXY_AUTH_PATH="+mounts.authDir,
		"CLI_PROXY_LOG_PATH="+mounts.logsDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("start detached upgrade container: %w: %s", err, strings.TrimSpace(string(output)))
	}

	jobID := strings.TrimSpace(string(output))
	if jobID == "" {
		return "", fmt.Errorf("upgrade container started without a container id")
	}

	return jobID, nil
}

type hostMounts struct {
	workspace  string
	configFile string
	authDir    string
	logsDir    string
}

func resolveHostMounts() (*hostMounts, error) {
	containerID, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("resolve current container id: %w", err)
	}

	cmd := exec.Command("docker", "inspect", "--format", "{{json .Mounts}}", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect current container mounts: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var mounts []containerMount
	if err := json.Unmarshal(output, &mounts); err != nil {
		return nil, fmt.Errorf("decode current container mounts: %w", err)
	}

	result := &hostMounts{}
	for _, mount := range mounts {
		switch mount.Destination {
		case "/workspace":
			result.workspace = mount.Source
		case "/CLIProxyAPI/config.yaml":
			result.configFile = mount.Source
		case "/root/.cli-proxy-api":
			result.authDir = mount.Source
		case "/CLIProxyAPI/logs":
			result.logsDir = mount.Source
		}
	}

	if strings.TrimSpace(result.workspace) == "" {
		return nil, fmt.Errorf("unable to resolve host workspace path from current container mounts")
	}
	if strings.TrimSpace(result.configFile) == "" {
		result.configFile = filepath.Join(result.workspace, "config.yaml")
	}
	if strings.TrimSpace(result.authDir) == "" {
		result.authDir = filepath.Join(result.workspace, "auths")
	}
	if strings.TrimSpace(result.logsDir) == "" {
		result.logsDir = filepath.Join(result.workspace, "logs")
	}

	return result, nil
}

func defaultCheckUpgradeReady() error {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=no")
	cmd.Dir = upgradeWorkspaceDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("检查仓库状态失败: %w: %s", err, strings.TrimSpace(string(output)))
	}

	if strings.TrimSpace(string(output)) != "" {
		return fmt.Errorf("检测到已跟踪的本地改动，无法安全执行一键升级。请先提交、暂存或清理这些改动后再试")
	}

	return nil
}

func shellEscape(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
