package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/npc1607/uu/internal/router"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uu.yaml")
	content := []byte(`
router: openwrt
model: "x86_64"
install-dir: '/opt/uu'
log_dir: /tmp/uu-logs # trailing comment
follow_logs: true
follow_log_file: /tmp/monitor.log
follow_log_lines: 50
follow_log_timeout: 30s
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Router != router.OpenWRT {
		t.Fatalf("Router = %q, want %q", cfg.Router, router.OpenWRT)
	}
	if cfg.Model != "x86_64" {
		t.Fatalf("Model = %q", cfg.Model)
	}
	if cfg.InstallDir != "/opt/uu" {
		t.Fatalf("InstallDir = %q", cfg.InstallDir)
	}
	if cfg.LogDir != "/tmp/uu-logs" {
		t.Fatalf("LogDir = %q", cfg.LogDir)
	}
	if cfg.FollowLogs == nil || !*cfg.FollowLogs {
		t.Fatalf("FollowLogs = %v, want true", cfg.FollowLogs)
	}
	if cfg.FollowLogFile != "/tmp/monitor.log" {
		t.Fatalf("FollowLogFile = %q", cfg.FollowLogFile)
	}
	if cfg.FollowLogLines == nil || *cfg.FollowLogLines != 50 {
		t.Fatalf("FollowLogLines = %v, want 50", cfg.FollowLogLines)
	}
	if cfg.FollowLogTimeout == nil || *cfg.FollowLogTimeout != 30*time.Second {
		t.Fatalf("FollowLogTimeout = %v, want 30s", cfg.FollowLogTimeout)
	}
}

func TestFileApplyKeepsDefaultsForEmptyValues(t *testing.T) {
	opts := DefaultOptions("/tmp/base")
	File{Router: router.HiWiFi}.Apply(&opts)

	if opts.Router != router.HiWiFi {
		t.Fatalf("Router = %q, want %q", opts.Router, router.HiWiFi)
	}
	if opts.Model != router.DefaultModel {
		t.Fatalf("Model = %q, want %q", opts.Model, router.DefaultModel)
	}
}
