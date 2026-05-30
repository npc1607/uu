package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchMonitorIdentityPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uuplugin_monitor.sh")
	script := `#!/bin/sh
ROUTER=""
STEAM_DECK_PLUGIN="steam-deck-plugin"
PLUGIN_DIR=""
RUNNING_DIR="/tmp/uu"

check_dir() {
    return 0
}

check_dir
[ "$?" != "0" ] && exit 1

check_plugin_upgrade

while :
do
    check_backtar_file
    check_plugin_file
    check_acc
    sleep 1
    check_running
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := patchMonitorIdentityPersistence(path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	patched := string(first)
	for _, want := range []string{
		"persist_steam_deck_identity()",
		steamDeckIdentityPatchMarker,
		"for identity_file in .uuplugin_uuid .uid",
		"persist_steam_deck_identity\n\ncheck_plugin_upgrade",
		"    persist_steam_deck_identity\n    check_acc",
		"    sleep 1\n    persist_steam_deck_identity\n    check_running",
	} {
		if !strings.Contains(patched, want) {
			t.Fatalf("patched script does not contain %q:\n%s", want, patched)
		}
	}

	if err := patchMonitorIdentityPersistence(path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != patched {
		t.Fatal("patch is not idempotent")
	}
}

func TestPatchMonitorPluginLogging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "uuplugin_monitor.sh")
	script := `#!/bin/sh
RUNNING_DIR="/tmp/uu"
PLUGIN_EXE="uuplugin"
PLUGIN_CONF="uu.conf"

start_acc() {
    local exefile="${RUNNING_DIR}/${PLUGIN_EXE}"
    local confile="${RUNNING_DIR}/${PLUGIN_CONF}"
    ${exefile} "${confile}" >/dev/null 2>&1 &
    ${exefile} "${RUNNING_DIR}/$PLUGIN_CONF" >/dev/null 2>&1 &
}
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := patchMonitorPluginLogging(path); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	patched := string(content)
	for _, want := range []string{
		steamDeckPluginLogPatchMarker,
		`>>"${RUNNING_DIR}/uuplugin.log" 2>&1 &`,
		`touch "${RUNNING_DIR}/uuplugin.log"`,
	} {
		if !strings.Contains(patched, want) {
			t.Fatalf("patched script does not contain %q:\n%s", want, patched)
		}
	}

	if err := patchMonitorPluginLogging(path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != patched {
		t.Fatal("plugin log patch is not idempotent")
	}
}

func TestCopyIdentityFiles(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, ".uuplugin_uuid"), []byte("uuid-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, ".uid"), []byte("uid-1"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyIdentityFiles(srcDir, dstDir); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		want string
	}{
		{name: ".uuplugin_uuid", want: "uuid-1"},
		{name: ".uid", want: "uid-1"},
	} {
		got, err := os.ReadFile(filepath.Join(dstDir, tc.name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.want {
			t.Fatalf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
}
