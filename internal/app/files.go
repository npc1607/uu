package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/npc1607/uu/internal/plugin"
)

func (i *App) createUninstall() error {
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
