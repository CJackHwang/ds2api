package webui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ds2api/internal/config"
)

const (
	defaultBuildTimeout = 5 * time.Minute
)

func EnsureBuiltOnStartup() {
	if !shouldAutoBuild() {
		return
	}
	staticDir := resolveStaticAdminDir(config.StaticAdminDir())
	if hasBuiltUI(staticDir) {
		return
	}
	if err := buildWebUI(staticDir); err != nil {
		config.Logger.Warn("[webui] auto build failed", "error", err)
		return
	}
	if hasBuiltUI(staticDir) {
		config.Logger.Info("[webui] auto build completed", "dir", staticDir)
		return
	}
	config.Logger.Warn("[webui] auto build finished but output missing", "dir", staticDir)
}

func shouldAutoBuild() bool {
	raw := strings.TrimSpace(os.Getenv("DS2API_AUTO_BUILD_WEBUI"))
	if raw == "" {
		return !config.IsVercel()
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return !config.IsVercel()
	}
}

func hasBuiltUI(staticDir string) bool {
	if strings.TrimSpace(staticDir) == "" {
		return false
	}
	indexPath := filepath.Join(staticDir, "index.html")
	st, err := os.Stat(indexPath)
	return err == nil && !st.IsDir()
}

func buildWebUI(staticDir string) error {
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm not found in PATH: %w", err)
	}
	if strings.TrimSpace(staticDir) == "" {
		return errors.New("static admin dir is empty")
	}

	config.Logger.Info("[webui] static files missing, running npm build")
	ctx, cancel := context.WithTimeout(context.Background(), defaultBuildTimeout)
	defer cancel()

	if _, err := os.Stat(filepath.Join("webui", "node_modules")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		installCmd := exec.CommandContext(ctx, "npm", "ci", "--include=optional", "--prefix", "webui")
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("webui npm ci timed out after %s", defaultBuildTimeout)
			}
			return err
		}
	}

	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		return err
	}
	if err := ensureFrontendNativeBinding(ctx); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "npm", "run", "build", "--prefix", "webui", "--", "--outDir", staticDir, "--emptyOutDir")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("webui build timed out after %s", defaultBuildTimeout)
		}
		return err
	}
	return nil
}

func ensureFrontendNativeBinding(ctx context.Context) error {
	bindingPkg := rolldownBindingPackage()
	if bindingPkg == "" {
		return nil
	}
	bindingPath := filepath.Join(append([]string{"webui", "node_modules"}, strings.Split(bindingPkg, "/")...)...)
	if _, err := os.Stat(bindingPath); err == nil {
		return nil
	}
	config.Logger.Warn("[webui] native binding missing; repairing optional deps", "package", bindingPkg)
	repairCmd := exec.CommandContext(ctx, "npm", "install", "--include=optional", "--prefix", "webui")
	repairCmd.Stdout = os.Stdout
	repairCmd.Stderr = os.Stderr
	if err := repairCmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("webui optional dependency repair timed out after %s", defaultBuildTimeout)
		}
		return fmt.Errorf("repair optional deps for %s: %w", bindingPkg, err)
	}
	if _, err := os.Stat(bindingPath); err != nil {
		return fmt.Errorf("required frontend native binding still missing after repair: %s", bindingPkg)
	}
	return nil
}

func rolldownBindingPackage() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return "@rolldown/binding-win32-x64-msvc"
	case "windows/arm64":
		return "@rolldown/binding-win32-arm64-msvc"
	case "linux/amd64":
		return "@rolldown/binding-linux-x64-gnu"
	case "linux/arm64":
		return "@rolldown/binding-linux-arm64-gnu"
	case "darwin/amd64":
		return "@rolldown/binding-darwin-x64"
	case "darwin/arm64":
		return "@rolldown/binding-darwin-arm64"
	default:
		return ""
	}
}
