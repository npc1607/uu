package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/npc1607/uu/internal/plugin"
	"github.com/npc1607/uu/internal/router"
)

func (i *App) createUninstall() error {
	if i.params.router == router.SteamDeck {
		return i.createSteamDeckUninstall()
	}

	dst := filepath.Join(i.params.installDir, plugin.UninstallFilename)
	if !fileExists(i.params.uninstallFile) {
		fmt.Fprintf(i.stdout, "uninstall file:%s not exist\n", i.params.uninstallFile)
		return nil
	}
	if err := copyFile(i.params.uninstallFile, dst); err != nil {
		return err
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		return err
	}
	updated := strings.ReplaceAll(
		string(content),
		"ROUTER=${1:-asuswrt-merlin}",
		"ROUTER=${1:-steam-deck-plugin}",
	)
	return os.WriteFile(dst, []byte(updated), fileModeOrDefault(dst, 0o755))
}

func (i *App) createSteamDeckUninstall() error {
	dst := filepath.Join(i.params.installDir, plugin.UninstallFilename)
	content := fmt.Sprintf(`#!/bin/sh
set -u

INSTALL_DIR="$(cd "$(dirname "$0")"; pwd -P)"
systemctl disable uuplugin >/dev/null 2>&1 || true
systemctl stop uuplugin >/dev/null 2>&1 || true
rm -f /etc/systemd/system/uuplugin.service
systemctl daemon-reload >/dev/null 2>&1 || true
rm -rf /tmp/uu
rm -f "${INSTALL_DIR}/%s" "${INSTALL_DIR}/%s" "${INSTALL_DIR}/%s"
`, plugin.MonitorFilename, plugin.MonitorConfigName, plugin.UninstallFilename)
	return os.WriteFile(dst, []byte(content), 0o755)
}

func (i *App) cleanUp() error {
	if i.params.router == router.SteamDeck {
		return i.cleanUpSteamDeck()
	}
	if !fileExists(i.params.uninstallFile) {
		return fmt.Errorf("%s does not exist", i.params.uninstallFile)
	}
	if err := chmodAdd(i.params.uninstallFile, 0o100); err != nil {
		return err
	}
	return i.runSilent("/bin/sh", i.params.uninstallFile, i.params.router, i.params.model)
}

func (i *App) cleanUpSteamDeck() error {
	serviceFile := "/etc/systemd/system/uuplugin.service"
	uninstallFile := filepath.Join(i.params.installDir, plugin.UninstallFilename)
	i.logf("cleanup targets: monitor=%s config=%s uninstall=%s runtime=/tmp/uu service=%s", i.params.monitorFile, i.params.monitorConfig, uninstallFile, serviceFile)

	if err := i.persistSteamDeckRuntimeIdentity(); err != nil {
		i.logf("persist identity before cleanup failed: %v", err)
	}
	if err := plugin.StopProcessesByPattern(plugin.MonitorFilename); err != nil {
		i.logf("stop monitor during cleanup failed: %v", err)
	}
	if err := plugin.Stop(); err != nil {
		i.logf("stop uuplugin during cleanup failed: %v", err)
	}
	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = i.runSilent("systemctl", "disable", "uuplugin")
		_ = i.runSilent("systemctl", "stop", "uuplugin")
	}

	for _, path := range []string{i.params.monitorFile, i.params.monitorConfig, uninstallFile} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			i.logf("remove %s failed: %v", path, err)
		} else {
			i.logf("removed %s", path)
		}
	}
	if err := os.RemoveAll("/tmp/uu"); err != nil {
		i.logf("remove /tmp/uu failed: %v", err)
	}
	if err := os.Remove(serviceFile); err != nil && !os.IsNotExist(err) {
		i.logf("remove %s failed: %v", serviceFile, err)
	}
	return nil
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileModeOrDefault(path string, fallback os.FileMode) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return fallback
	}
	return info.Mode()
}

func chmodAdd(path string, bits os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.Chmod(path, info.Mode()|bits)
}

func fileContains(path string, needle string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(content), needle), nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func ensureTrailingSeparator(path string) string {
	if path == "" {
		return path
	}
	if strings.HasSuffix(path, string(os.PathSeparator)) {
		return path
	}
	return path + string(os.PathSeparator)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
