package app

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/npc1607/uu/internal/plugin"
	"github.com/npc1607/uu/internal/router"
)

const (
	namespaceServiceName = "uuplugin-ns"
	namespaceServiceFile = "/etc/systemd/system/uuplugin-ns.service"
)

func (i *App) configRouter() error {
	switch i.params.router {
	case router.ASUSWRTMerlin:
		_ = i.runCommand("nvram", "set", "jffs2_enable=1")
		_ = i.runCommand("nvram", "set", "jffs2_scripts=1")
		_ = i.startDetached("nvram", "commit")
		return nil
	case router.Xiaomi, router.HiWiFi, router.OpenWRT, router.SteamDeck:
		return nil
	default:
		return fmt.Errorf("unsupported router %q", i.params.router)
	}
}

func (i *App) configBootup() error {
	switch i.params.router {
	case router.ASUSWRTMerlin:
		if i.checkMerlin() {
			return i.configServicesStart()
		}
		return i.configExecStart()
	case router.Xiaomi, router.HiWiFi, router.OpenWRT:
		return i.configBootupImplementation()
	case router.SteamDeck:
		return i.configSteamDeckSystemd()
	default:
		return fmt.Errorf("unsupported router %q", i.params.router)
	}
}

func (i *App) checkMerlin() bool {
	ip := ipv4ForInterface("br0")
	if ip == "" {
		return false
	}
	resp, err := i.client.Get("http://" + ip + "/images/merlin-logo.png")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (i *App) configExecStart() error {
	bootupScript := filepath.Join(i.params.installDir, "uuplugin_bootup.sh")
	content := fmt.Sprintf("#!/bin/sh\nnohup /bin/sh %s &\n", i.params.monitorFile)
	if err := os.WriteFile(bootupScript, []byte(content), 0o755); err != nil {
		return err
	}
	if err := chmodAdd(bootupScript, 0o100); err != nil {
		return err
	}
	_ = i.runCommand("nvram", "set", "jffs2_exec="+bootupScript)
	_ = i.startDetached("nvram", "commit")
	return nil
}

func (i *App) configServicesStart() error {
	servicesStartFile := "/jffs/scripts/services-start"
	if !pathExists(servicesStartFile) {
		if err := os.MkdirAll(filepath.Dir(servicesStartFile), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(servicesStartFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o755)
		if err != nil {
			return err
		}
		if _, err := file.WriteString("#!/bin/sh\n\n\n"); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}

	if err := chmodAdd(servicesStartFile, 0o100); err != nil {
		return err
	}
	contains, err := fileContains(servicesStartFile, i.params.monitorFile)
	if err != nil {
		return err
	}
	if contains {
		return nil
	}
	file, err := os.OpenFile(servicesStartFile, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintf(file, "/bin/sh %s &\n", i.params.monitorFile)
	return err
}

func (i *App) configSteamDeckSystemd() error {
	service := "/etc/systemd/system/uuplugin.service"
	if os.Geteuid() != 0 {
		return fmt.Errorf("root privileges required to write %s; rerun with sudo", service)
	}
	content := steamDeckSystemdUnit(filepath.Join(i.params.installDir, plugin.MonitorFilename))

	if err := os.WriteFile(service, []byte(content), 0o644); err != nil {
		return err
	}
	if err := i.runCommand("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := i.runCommand("systemctl", "enable", "uuplugin"); err != nil {
		return err
	}
	return i.runCommand("systemctl", "start", "uuplugin")
}

func steamDeckSystemdUnit(monitorPath string) string {
	return fmt.Sprintf(`[Unit]
Description=UU Plugin
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStart=/bin/sh %s
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
`, monitorPath)
}

func (i *App) NamespaceInstall() int {
	i.openLog(true)
	defer i.closeLog()
	if i.logPath != "" {
		fmt.Fprintf(i.stdout, "Install log: %s\n", i.logPath)
	}
	if err := i.initParams(); err != nil {
		return i.failNamespaceInstall(9, "init params failed: %v", err)
	}
	if os.Geteuid() != 0 {
		return i.failNamespaceInstall(1, "root privileges required to write %s; rerun with sudo", namespaceServiceFile)
	}
	if err := validateLocalListen(i.params.webListen); err != nil {
		return i.failNamespaceInstall(1, "validate web listen failed: %v", err)
	}

	cfg, err := i.resolveNamespaceConfig()
	if err != nil {
		return i.failNamespaceInstall(1, "resolve namespace config failed: %v", err)
	}
	if err := validateNamespaceConfig(cfg); err != nil {
		return i.failNamespaceInstall(1, "validate namespace config failed: %v", err)
	}
	if err := i.ensureNamespacePrereqs(); err != nil {
		return i.failNamespaceInstall(1, "namespace prerequisites failed: %v", err)
	}
	if err := i.prepareNamespaceMonitor(); err != nil {
		return i.failNamespaceInstall(5, "prepare namespace monitor failed: %v", err)
	}
	if err := i.configNamespaceSystemd(cfg); err != nil {
		return i.failNamespaceInstall(6, "configure namespace service failed: %v", err)
	}
	if err := i.waitNamespacePlugin(cfg.Name, namespaceActionTimeout); err != nil {
		return i.failNamespaceInstall(7, "uuplugin did not start in namespace: %v", err)
	}

	fmt.Fprintf(i.stdout, "Namespace installation succeeded! service=%s web=http://%s/\n", namespaceServiceName, i.params.webListen)
	return i.finishSuccessfulInstall()
}

func (i *App) NamespaceUninstall() int {
	i.openLog(true)
	defer i.closeLog()
	if i.logPath != "" {
		fmt.Fprintf(i.stdout, "Uninstall log: %s\n", i.logPath)
	}
	if err := i.initParams(); err != nil {
		return i.failNamespaceUninstall(9, "init params failed: %v", err)
	}
	if os.Geteuid() != 0 {
		return i.failNamespaceUninstall(1, "root privileges required to remove %s; rerun with sudo", namespaceServiceFile)
	}
	if err := i.namespaceUninstall(); err != nil {
		return i.failNamespaceUninstall(1, "%v", err)
	}
	fmt.Fprintf(i.stdout, "Namespace uninstall succeeded! service=%s removed\n", namespaceServiceName)
	return 0
}

func (i *App) failNamespaceInstall(code int, format string, args ...any) int {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(i.stderr, "Namespace installation failed: %s\n", msg)
	if i.logPath != "" {
		fmt.Fprintf(i.stderr, "Install log: %s\n", i.logPath)
	}
	i.logf("namespace installation failed: %s", msg)
	return code
}

func (i *App) failNamespaceUninstall(code int, format string, args ...any) int {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(i.stderr, "Namespace uninstall failed: %s\n", msg)
	if i.logPath != "" {
		fmt.Fprintf(i.stderr, "Uninstall log: %s\n", i.logPath)
	}
	i.logf("namespace uninstall failed: %s", msg)
	return code
}

func (i *App) namespaceUninstall() error {
	if _, err := exec.LookPath("systemctl"); err == nil {
		state, stateErr := systemdState(namespaceServiceName)
		if stateErr != nil {
			i.logf("inspect %s state warning: %v", namespaceServiceName, stateErr)
			fmt.Fprintf(i.stdout, "service=%s state=unknown\n", namespaceServiceName)
		} else {
			fmt.Fprintf(i.stdout, "service=%s state=%s\n", namespaceServiceName, state)
			if namespaceServiceRunningState(state) {
				fmt.Fprintf(i.stdout, "stopping service=%s\n", namespaceServiceName)
				if err := i.runCommand("systemctl", "stop", namespaceServiceName); err != nil {
					return fmt.Errorf("stop %s failed: %w", namespaceServiceName, err)
				}
			} else {
				fmt.Fprintf(i.stdout, "service=%s running=false\n", namespaceServiceName)
			}
		}
		if err := i.runSilent("systemctl", "disable", namespaceServiceName); err != nil {
			i.logf("disable %s warning: %v", namespaceServiceName, err)
		}
	} else {
		i.logf("systemctl not found; skipping %s service stop/disable", namespaceServiceName)
	}

	cfg := namespaceConfig{
		Name: firstNonEmpty(i.params.namespaceName, "uu-ns"),
		Link: firstNonEmpty(i.params.namespaceLink, "uu-macvlan0"),
	}
	if err := validateNamespaceName(cfg.Name); err != nil {
		return err
	}
	if err := validateInterfaceName(cfg.Link); err != nil {
		return err
	}
	if err := i.namespaceStopConfig(cfg); err != nil {
		return err
	}
	i.cleanupStalePluginPIDFile()

	for _, path := range i.namespaceInstallArtifacts() {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s failed: %w", path, err)
		}
		i.logf("removed %s", path)
	}

	if _, err := exec.LookPath("systemctl"); err == nil {
		if err := i.runCommand("systemctl", "daemon-reload"); err != nil {
			return fmt.Errorf("systemctl daemon-reload failed: %w", err)
		}
		_ = i.runSilent("systemctl", "reset-failed", namespaceServiceName)
	}
	return nil
}

func (i *App) namespaceInstallArtifacts() []string {
	return []string{
		namespaceServiceFile,
		i.params.monitorFile,
		i.params.monitorConfig,
		filepath.Join(i.params.installDir, plugin.UninstallFilename),
		i.steamDeckRuntimeDir(),
		steamDeckLegacyRuntimeDir,
	}
}

func namespaceServiceRunningState(state string) bool {
	switch state {
	case "active", "activating", "deactivating", "reloading":
		return true
	default:
		return false
	}
}

func (i *App) configNamespaceSystemd(cfg namespaceConfig) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}

	content := namespaceSystemdUnit(
		exe,
		i.namespaceServiceArgs("ns-serve", cfg),
		i.namespaceServiceArgs("ns-stop", cfg),
	)
	if err := os.WriteFile(namespaceServiceFile, []byte(content), 0o644); err != nil {
		return err
	}

	_ = i.runSilent("systemctl", "disable", "uuplugin")
	_ = i.runSilent("systemctl", "stop", "uuplugin")
	if err := i.runCommand("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := i.runCommand("systemctl", "enable", namespaceServiceName); err != nil {
		return err
	}
	return i.runCommand("systemctl", "restart", namespaceServiceName)
}

func (i *App) namespaceServiceArgs(command string, cfg namespaceConfig) []string {
	args := []string{
		command,
		"--router", i.params.router,
		"--model", i.params.model,
		"--install-dir", i.params.installDir,
		"--log-dir", i.opts.LogDir,
		"--listen", i.params.webListen,
		"--namespace-name", cfg.Name,
		"--namespace-parent", cfg.Parent,
		"--namespace-link", cfg.Link,
		"--namespace-address", cfg.Address,
		"--namespace-gateway", cfg.Gateway,
		"--namespace-dns", strings.Join(cfg.DNS, ","),
		"--namespace-mode", cfg.Mode,
		"--follow-log-file", i.opts.FollowLogFile,
		"--follow-log-lines", strconv.Itoa(i.opts.FollowLogLines),
		"--follow-log-timeout", i.opts.FollowLogTimeout.String(),
	}
	if i.opts.FollowLogs {
		args = append(args, "--follow-logs")
	}
	return args
}

func namespaceSystemdUnit(exePath string, startArgs []string, stopArgs []string) string {
	return fmt.Sprintf(`[Unit]
Description=UU Plugin Namespace
Wants=network-online.target
After=network-online.target
Conflicts=uuplugin.service

[Service]
Type=simple
ExecStart=%s
ExecStop=%s
Restart=always
RestartSec=5s
TimeoutStopSec=30s

[Install]
WantedBy=multi-user.target
`, systemdExecLine(exePath, startArgs), systemdExecLine(exePath, stopArgs))
}

func systemdExecLine(exePath string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, systemdQuoteArg(exePath))
	for _, arg := range args {
		parts = append(parts, systemdQuoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func systemdQuoteArg(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(value)
	if escaped == "" || strings.ContainsAny(escaped, " \t\n\r\"\\") {
		return `"` + escaped + `"`
	}
	return escaped
}

func (i *App) configBootupImplementation() error {
	initScript := filepath.Join(i.params.installDir, "S99uuplugin")
	linkScript := "/etc/rc.d/S99uuplugin"
	content := fmt.Sprintf(`#!/bin/sh /etc/rc.common


START=99
start() {
    /bin/sh %s &
}
`, i.params.monitorFile)

	if err := os.WriteFile(initScript, []byte(content), 0o755); err != nil {
		return err
	}
	if !fileExists(initScript) {
		return fmt.Errorf("%s does not exist after write", initScript)
	}
	if err := chmodAdd(initScript, 0o100); err != nil {
		return err
	}

	_ = os.Remove(linkScript)
	if err := os.Symlink(initScript, linkScript); err != nil {
		_ = os.Remove(initScript)
		return err
	}
	return nil
}

func (i *App) printSN() error {
	var iface string
	switch i.params.router {
	case router.ASUSWRTMerlin:
		iface = "br0"
	case router.Xiaomi, router.HiWiFi, router.OpenWRT:
		iface = "br-lan"
	default:
		return fmt.Errorf("router %q has no sn interface", i.params.router)
	}

	mac, err := macForInterface(iface)
	if err != nil {
		return err
	}
	fmt.Fprintf(i.stdout, "sn=%s\n", mac)
	return nil
}

func ipv4ForInterface(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

func macForInterface(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}
	mac := strings.TrimSpace(iface.HardwareAddr.String())
	if mac == "" {
		return "", fmt.Errorf("interface %s has no mac address", name)
	}
	return mac, nil
}
