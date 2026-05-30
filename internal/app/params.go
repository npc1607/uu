package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/npc1607/uu/internal/plugin"
	"github.com/npc1607/uu/internal/router"
)

type runtimeParams struct {
	router               string
	model                string
	installDir           string
	uninstallFile        string
	monitorFile          string
	monitorConfig        string
	uninstallDownloadURL string
	monitorDownloadURL   string
	webListen            string
	namespaceName        string
	namespaceParent      string
	namespaceLink        string
	namespaceAddress     string
	namespaceGateway     string
	namespaceDNS         string
	namespaceMode        string
}

func (i *App) initParams() error {
	routerName := strings.TrimSpace(i.opts.Router)
	if !router.Supported(routerName) {
		return fmt.Errorf("unsupported router %q", routerName)
	}

	installDir := i.opts.InstallDir
	scheme := httpsScheme
	switch routerName {
	case router.ASUSWRTMerlin:
		installDir = firstNonEmpty(installDir, "/jffs/uu")
	case router.Xiaomi:
		scheme = httpScheme
		installDir = firstNonEmpty(installDir, "/data/uu")
	case router.HiWiFi:
		installDir = firstNonEmpty(installDir, "/plugins/uu")
	case router.OpenWRT:
		scheme = httpScheme
		installDir = firstNonEmpty(installDir, "/usr/sbin/uu/")
	case router.SteamDeck:
		installDir = firstNonEmpty(installDir, i.getSteamDeckInstallDir())
	}

	installDir = ensureTrailingSeparator(installDir)
	i.params = runtimeParams{
		router:               routerName,
		model:                i.opts.Model,
		installDir:           installDir,
		uninstallFile:        filepath.Join(i.opts.LogDir, fmt.Sprintf("uu_uninstall_%d.sh", time.Now().UnixNano())),
		monitorFile:          filepath.Join(installDir, plugin.MonitorFilename),
		monitorConfig:        filepath.Join(installDir, plugin.MonitorConfigName),
		uninstallDownloadURL: scheme + uninstallEndpoint + routerName,
		monitorDownloadURL:   scheme + monitorEndpoint + routerName,
		webListen:            firstNonEmpty(i.opts.WebListen, "127.0.0.1:8088"),
		namespaceName:        firstNonEmpty(i.opts.NamespaceName, "uu-ns"),
		namespaceParent:      strings.TrimSpace(i.opts.NamespaceParent),
		namespaceLink:        firstNonEmpty(i.opts.NamespaceLink, "uu-macvlan0"),
		namespaceAddress:     strings.TrimSpace(i.opts.NamespaceAddress),
		namespaceGateway:     strings.TrimSpace(i.opts.NamespaceGateway),
		namespaceDNS:         firstNonEmpty(i.opts.NamespaceDNS, "223.5.5.5,119.29.29.29"),
		namespaceMode:        firstNonEmpty(i.opts.NamespaceMode, "macvlan"),
	}
	i.initialized = true
	return nil
}

func (i *App) getSteamDeckInstallDir() string {
	baseDir, err := filepath.Abs(i.opts.BaseDir)
	if err != nil {
		return ensureTrailingSeparator(i.opts.BaseDir)
	}
	return ensureTrailingSeparator(baseDir)
}
