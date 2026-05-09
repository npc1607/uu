package app

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/npc1607/uu/internal/plugin"
	"github.com/npc1607/uu/internal/router"
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
	content := fmt.Sprintf(`[Unit]
Description=UU Plugin
Wants=network-online.target
After=network.target network-online.target

[Service]
ExecStart=/bin/sh %s

[Install]
WantedBy=default.target
`, filepath.Join(i.params.installDir, plugin.MonitorFilename))

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
