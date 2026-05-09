package app

import (
	"path/filepath"
	"testing"

	"github.com/npc1607/uu/internal/config"
	"github.com/npc1607/uu/internal/router"
)

func TestSteamDeckDefaultInstallDirUsesBinaryDir(t *testing.T) {
	baseDir := t.TempDir()
	logDir := t.TempDir()
	app := New(config.Options{
		Router:  router.SteamDeck,
		Model:   router.DefaultModel,
		BaseDir: baseDir,
		LogDir:  logDir,
	})

	if err := app.initParams(); err != nil {
		t.Fatal(err)
	}

	want := ensureTrailingSeparator(baseDir)
	if app.params.installDir != want {
		t.Fatalf("installDir = %q, want %q", app.params.installDir, want)
	}
	if filepath.Dir(app.params.uninstallFile) != logDir {
		t.Fatalf("uninstallFile = %q, want temp file under %q", app.params.uninstallFile, logDir)
	}
}
