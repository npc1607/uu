package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/npc1607/uu/internal/config"
	"github.com/npc1607/uu/internal/router"
)

func TestCheckSteamDeckIPTablesReportsKernelMismatch(t *testing.T) {
	binDir := t.TempDir()
	iptables := filepath.Join(binDir, "iptables")
	if err := os.WriteFile(iptables, []byte("#!/bin/sh\necho 'iptables: Failed to initialize nft: Protocol not supported' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	var output bytes.Buffer
	application := New(config.Options{
		Router:  router.SteamDeck,
		Model:   router.DefaultModel,
		BaseDir: t.TempDir(),
		LogDir:  t.TempDir(),
	}, WithOutput(&output, &output))
	application.logWriter = &output

	err := application.checkSteamDeckIPTables()
	if err == nil {
		t.Fatal("checkSteamDeckIPTables succeeded with a broken iptables")
	}
	for _, want := range []string{"Protocol not supported", "reboot first"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error does not contain %q: %v", want, err)
		}
	}
}
