package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/npc1607/uu/internal/config"
	"github.com/npc1607/uu/internal/router"
)

func TestShowSteamDeckBindingQRCode(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	application := New(config.Options{
		Router:     router.SteamDeck,
		Model:      router.DefaultModel,
		BaseDir:    dir,
		InstallDir: dir,
		LogDir:     t.TempDir(),
	}, WithOutput(&stdout, &stdout))
	if err := application.initParams(); err != nil {
		t.Fatal(err)
	}

	qrFile := filepath.Join(application.steamDeckRuntimeDir(), steamDeckQRCodeName)
	if err := os.MkdirAll(filepath.Dir(qrFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(qrFile, []byte("QR-CONTENT\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	application.showSteamDeckBindingQRCode(func(time.Duration) {
		if err := os.Remove(qrFile); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	})

	output := stdout.String()
	for _, want := range []string{"插件安装成功", "扫码并完成绑定", "QR-CONTENT"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output does not contain %q:\n%s", want, output)
		}
	}
}

func TestShowSteamDeckBindingFallsBackToLAN(t *testing.T) {
	dir := t.TempDir()
	var stdout bytes.Buffer
	application := New(config.Options{
		Router:     router.SteamDeck,
		Model:      router.DefaultModel,
		BaseDir:    dir,
		InstallDir: dir,
		LogDir:     t.TempDir(),
	}, WithOutput(&stdout, &stdout))
	if err := application.initParams(); err != nil {
		t.Fatal(err)
	}

	sleeps := 0
	application.showSteamDeckBindingQRCode(func(time.Duration) { sleeps++ })
	if sleeps != steamDeckQRCodeRetries {
		t.Fatalf("sleep count = %d, want %d", sleeps, steamDeckQRCodeRetries)
	}
	if !strings.Contains(stdout.String(), "局域网绑定") {
		t.Fatalf("output does not contain LAN binding fallback:\n%s", stdout.String())
	}
}
