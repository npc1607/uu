package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/npc1607/uu/internal/router"
)

const (
	steamDeckIdentityPatchMarker  = "# uu-go: persist Steam Deck identity in install dir"
	steamDeckPluginLogPatchMarker = "# uu-go: capture uuplugin stdout/stderr"
	steamDeckLegacyRuntimeDir     = "/tmp/uu"
	steamDeckRuntimeDirName       = "runtime"
	steamDeckPluginLogName        = "uuplugin.log"
)

var steamDeckIdentityFiles = []string{".uuplugin_uuid", ".uid"}

func (i *App) patchSteamDeckMonitorIdentity() error {
	if i.params.router != router.SteamDeck {
		return nil
	}
	if err := patchMonitorRuntimeDir(i.params.monitorFile); err != nil {
		return err
	}
	if err := patchMonitorIdentityPersistence(i.params.monitorFile); err != nil {
		return err
	}
	return patchMonitorPluginLogging(i.params.monitorFile)
}

func (i *App) steamDeckRuntimeDir() string {
	return filepath.Join(i.params.installDir, steamDeckRuntimeDirName)
}

func (i *App) steamDeckPluginLogFile() string {
	return filepath.Join(i.steamDeckRuntimeDir(), steamDeckPluginLogName)
}

func patchMonitorRuntimeDir(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	const oldLine = `RUNNING_DIR="/tmp/uu"`
	const newLine = `RUNNING_DIR="${BASEDIR}/runtime"`
	script := string(content)
	if strings.Contains(script, newLine) {
		return nil
	}
	if !strings.Contains(script, oldLine) {
		return fmt.Errorf("monitor script does not contain runtime dir anchor")
	}
	script = strings.Replace(script, oldLine, newLine, 1)
	return os.WriteFile(path, []byte(script), fileModeOrDefault(path, 0o755))
}

func patchMonitorIdentityPersistence(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	script := string(content)
	if strings.Contains(script, steamDeckIdentityPatchMarker) {
		return nil
	}

	const functionAnchor = "check_dir() {\n"
	const postCheckDirAnchor = "check_dir\n[ \"$?\" != \"0\" ] && exit 1\n\ncheck_plugin_upgrade"
	const loopAnchor = "    check_backtar_file\n    check_plugin_file\n    check_acc\n    sleep 1\n    check_running"

	if !strings.Contains(script, functionAnchor) {
		return fmt.Errorf("monitor script does not contain check_dir anchor")
	}
	if !strings.Contains(script, postCheckDirAnchor) {
		return fmt.Errorf("monitor script does not contain startup check_dir anchor")
	}
	if !strings.Contains(script, loopAnchor) {
		return fmt.Errorf("monitor script does not contain main loop anchor")
	}

	identityFunction := `persist_steam_deck_identity() {
    ` + steamDeckIdentityPatchMarker + `
    [ "${ROUTER}" != "${STEAM_DECK_PLUGIN}" ] && return 0
    [ -z "${PLUGIN_DIR}" ] && return 0
    [ ! -d "${RUNNING_DIR}" ] && return 0
    [ ! -d "${PLUGIN_DIR}" ] && return 0

    for identity_file in .uuplugin_uuid .uid
    do
        local persistent_file="${PLUGIN_DIR}/${identity_file}"
        local runtime_file="${RUNNING_DIR}/${identity_file}"

        if [ -f "${persistent_file}" ] && [ ! -f "${runtime_file}" ]; then
            cp "${persistent_file}" "${runtime_file}" >/dev/null 2>&1 || true
        fi

        if [ -f "${runtime_file}" ]; then
            cp "${runtime_file}" "${persistent_file}" >/dev/null 2>&1 || true
            chmod a+rw "${persistent_file}" >/dev/null 2>&1 || true
        fi
    done
    return 0
}

`

	script = strings.Replace(script, functionAnchor, identityFunction+functionAnchor, 1)
	script = strings.Replace(script, postCheckDirAnchor, "check_dir\n[ \"$?\" != \"0\" ] && exit 1\npersist_steam_deck_identity\n\ncheck_plugin_upgrade", 1)
	script = strings.Replace(script, loopAnchor, "    check_backtar_file\n    check_plugin_file\n    persist_steam_deck_identity\n    check_acc\n    sleep 1\n    persist_steam_deck_identity\n    check_running", 1)

	return os.WriteFile(path, []byte(script), fileModeOrDefault(path, 0o755))
}

func patchMonitorPluginLogging(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	script := string(content)
	if strings.Contains(script, steamDeckPluginLogPatchMarker) {
		return nil
	}

	replacements := map[string]string{
		`${exefile} "${confile}" >/dev/null 2>&1 &`:                  `${exefile} "${confile}" >>"${RUNNING_DIR}/uuplugin.log" 2>&1 &`,
		`${exefile} "${RUNNING_DIR}/$PLUGIN_CONF" >/dev/null 2>&1 &`: `${exefile} "${RUNNING_DIR}/$PLUGIN_CONF" >>"${RUNNING_DIR}/uuplugin.log" 2>&1 &`,
	}
	updated := script
	for old, replacement := range replacements {
		updated = strings.ReplaceAll(updated, old, replacement)
	}
	if updated == script {
		return fmt.Errorf("monitor script does not contain uuplugin start command")
	}

	const startAnchor = "start_acc() {\n"
	logSetup := `start_acc() {
    ` + steamDeckPluginLogPatchMarker + `
    touch "${RUNNING_DIR}/uuplugin.log" >/dev/null 2>&1 || true
    if [ -f "${RUNNING_DIR}/uuplugin.log" ]; then
        local plugin_log_size=$(wc -c < "${RUNNING_DIR}/uuplugin.log" 2>/dev/null || echo 0)
        [ "${plugin_log_size}" -gt 1048576 ] && : > "${RUNNING_DIR}/uuplugin.log"
    fi
`
	if !strings.Contains(updated, startAnchor) {
		return fmt.Errorf("monitor script does not contain start_acc anchor")
	}
	updated = strings.Replace(updated, startAnchor, logSetup, 1)

	return os.WriteFile(path, []byte(updated), fileModeOrDefault(path, 0o755))
}

func (i *App) persistSteamDeckRuntimeIdentity() error {
	if i.params.router != router.SteamDeck {
		return nil
	}
	return copyIdentityFiles(i.steamDeckRuntimeDir(), i.params.installDir)
}

func copyIdentityFiles(srcDir string, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	for _, name := range steamDeckIdentityFiles {
		src := filepath.Join(srcDir, name)
		content, err := os.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		mode := fs.FileMode(0o666)
		if info, err := os.Stat(src); err == nil {
			mode = info.Mode()
		}
		dst := filepath.Join(dstDir, name)
		if err := os.WriteFile(dst, content, mode); err != nil {
			return err
		}
	}
	return nil
}
