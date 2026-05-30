package app

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/npc1607/uu/internal/plugin"
	"github.com/npc1607/uu/internal/router"
)

const namespaceActionTimeout = 90 * time.Second

type namespaceConfig struct {
	Name    string
	Parent  string
	Link    string
	Address string
	Gateway string
	DNS     []string
	Mode    string
}

type namespaceState struct {
	Name             string              `json:"name"`
	Exists           bool                `json:"exists"`
	Parent           string              `json:"parent,omitempty"`
	Link             string              `json:"link,omitempty"`
	Address          string              `json:"address,omitempty"`
	Gateway          string              `json:"gateway,omitempty"`
	DNS              []string            `json:"dns,omitempty"`
	Mode             string              `json:"mode,omitempty"`
	WebListen        string              `json:"web_listen,omitempty"`
	MonitorPIDs      []int               `json:"monitor_pids,omitempty"`
	PluginPIDs       []int               `json:"plugin_pids,omitempty"`
	NamespacePIDs    []int               `json:"namespace_pids,omitempty"`
	Neighbors        []namespaceNeighbor `json:"neighbors,omitempty"`
	Sockets          []namespaceSocket   `json:"sockets,omitempty"`
	ObservedAt       string              `json:"observed_at,omitempty"`
	ObservationError string              `json:"observation_error,omitempty"`
	InstallDir       string              `json:"install_dir,omitempty"`
	RuntimeDir       string              `json:"runtime_dir,omitempty"`
	PluginLogFile    string              `json:"plugin_log_file,omitempty"`
	MonitorFile      string              `json:"monitor_file,omitempty"`
	MonitorConfig    string              `json:"monitor_config,omitempty"`
	LogFile          string              `json:"log_file,omitempty"`
	Error            string              `json:"error,omitempty"`
}

type namespaceSocket struct {
	Protocol     string `json:"protocol"`
	State        string `json:"state"`
	LocalAddress string `json:"local_address"`
	LocalPort    string `json:"local_port"`
	PeerAddress  string `json:"peer_address"`
	PeerPort     string `json:"peer_port"`
	Process      string `json:"process,omitempty"`
	LANPeer      bool   `json:"lan_peer,omitempty"`
}

type namespaceNeighbor struct {
	Address string `json:"address"`
	MAC     string `json:"mac,omitempty"`
	State   string `json:"state,omitempty"`
}

func (i *App) NamespaceStart() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "namespace start failed: %v\n", err)
		return 1
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if err := i.namespaceStartLocked(); err != nil {
		fmt.Fprintf(i.stderr, "namespace start failed: %v\n", err)
		return 1
	}
	state, err := i.namespaceStateLocked()
	if err != nil {
		fmt.Fprintf(i.stdout, "namespace=%s state=started\n", i.params.namespaceName)
		return 0
	}
	i.printNamespaceState(state)
	return 0
}

func (i *App) NamespaceStop() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "namespace stop failed: %v\n", err)
		return 1
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if err := i.namespaceStopLocked(); err != nil {
		fmt.Fprintf(i.stderr, "namespace stop failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(i.stdout, "namespace=%s state=stopped\n", i.params.namespaceName)
	return 0
}

func (i *App) NamespaceStatus() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "namespace status failed: %v\n", err)
		return 1
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	state, err := i.namespaceStateLocked()
	if err != nil {
		fmt.Fprintf(i.stderr, "namespace status failed: %v\n", err)
		return 1
	}
	i.printNamespaceState(state)
	if state.Exists && len(state.PluginPIDs) > 0 {
		return 0
	}
	return 1
}

func (i *App) namespaceStartLocked() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("root privileges required for namespace mode; rerun with sudo")
	}

	cfg, err := i.resolveNamespaceConfig()
	if err != nil {
		return err
	}
	if err := validateNamespaceConfig(cfg); err != nil {
		return err
	}
	if err := i.ensureNamespacePrereqs(); err != nil {
		return err
	}
	if err := i.prepareNamespaceMonitor(); err != nil {
		return err
	}
	if err := i.namespaceStopConfig(cfg); err != nil {
		return err
	}
	if err := i.stopLegacyHostPlugin(); err != nil {
		return err
	}
	if err := i.createNamespace(cfg); err != nil {
		return err
	}
	if err := i.startNamespaceMonitor(cfg); err != nil {
		_ = i.namespaceStopConfig(cfg)
		return err
	}
	if err := i.waitNamespacePlugin(cfg.Name, namespaceActionTimeout); err != nil {
		return err
	}
	return nil
}

func (i *App) namespaceStopLocked() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("root privileges required for namespace mode; rerun with sudo")
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
	return i.namespaceStopConfig(cfg)
}

func (i *App) namespaceStopConfig(cfg namespaceConfig) error {
	exists, err := i.namespaceExists(cfg.Name)
	if err != nil {
		return err
	}

	var pids []int
	if exists {
		pids, err = i.namespacePIDs(cfg.Name)
		if err != nil {
			return err
		}
		if len(pids) > 0 {
			if err := terminatePIDList(pids); err != nil {
				return fmt.Errorf("terminate namespace pids: %w", err)
			}
		}
		if err := i.runSilent("ip", "netns", "delete", cfg.Name); err != nil {
			return fmt.Errorf("delete netns %s: %w", cfg.Name, err)
		}
	}
	if err := i.runSilent("ip", "link", "delete", cfg.Link); err != nil {
		i.logf("delete link %s failed: %v", cfg.Link, err)
	}
	if err := os.RemoveAll(filepath.Join("/etc/netns", cfg.Name)); err != nil {
		i.logf("remove /etc/netns/%s failed: %v", cfg.Name, err)
	}
	if !anyPIDAlive(pids) {
		i.cleanupLegacyPIDFile(pids)
	}
	return nil
}

func (i *App) namespaceStateLocked() (namespaceState, error) {
	cfg, err := i.resolveNamespaceConfig()
	if err != nil {
		return namespaceState{Error: err.Error()}, err
	}
	exists, err := i.namespaceExists(cfg.Name)
	if err != nil {
		return namespaceState{Error: err.Error()}, err
	}

	state := namespaceState{
		Name:          cfg.Name,
		Exists:        exists,
		Parent:        cfg.Parent,
		Link:          cfg.Link,
		Address:       cfg.Address,
		Gateway:       cfg.Gateway,
		DNS:           append([]string(nil), cfg.DNS...),
		Mode:          cfg.Mode,
		WebListen:     i.params.webListen,
		InstallDir:    i.params.installDir,
		RuntimeDir:    i.steamDeckRuntimeDir(),
		PluginLogFile: i.steamDeckPluginLogFile(),
		MonitorFile:   i.params.monitorFile,
		MonitorConfig: i.params.monitorConfig,
		LogFile:       i.opts.FollowLogFile,
	}
	if !exists {
		return state, nil
	}
	if pids, err := i.namespacePIDs(cfg.Name); err == nil {
		state.NamespacePIDs = append([]int(nil), pids...)
		state.MonitorPIDs = filterPIDs(pids, plugin.MonitorFilename)
		state.PluginPIDs = namespacePluginPIDs(pids)
	}
	state.Sockets, state.Neighbors, err = i.namespaceNetworkState(cfg)
	state.ObservedAt = time.Now().Format(time.RFC3339)
	if err != nil {
		state.ObservationError = err.Error()
	}
	return state, nil
}

func (i *App) printNamespaceState(state namespaceState) {
	fmt.Fprintf(i.stdout, "namespace=%s state=%s\n", state.Name, boolState(state.Exists))
	fmt.Fprintf(i.stdout, "namespace_parent=%s\n", state.Parent)
	fmt.Fprintf(i.stdout, "namespace_link=%s\n", state.Link)
	fmt.Fprintf(i.stdout, "namespace_address=%s\n", state.Address)
	fmt.Fprintf(i.stdout, "namespace_gateway=%s\n", state.Gateway)
	fmt.Fprintf(i.stdout, "namespace_dns=%s\n", strings.Join(state.DNS, ","))
	fmt.Fprintf(i.stdout, "namespace_mode=%s\n", state.Mode)
	fmt.Fprintf(i.stdout, "web_listen=%s\n", state.WebListen)
	fmt.Fprintf(i.stdout, "namespace_pids=%s\n", joinPIDs(state.NamespacePIDs))
	fmt.Fprintf(i.stdout, "monitor_pids=%s\n", joinPIDs(state.MonitorPIDs))
	fmt.Fprintf(i.stdout, "plugin_pids=%s\n", joinPIDs(state.PluginPIDs))
	fmt.Fprintf(i.stdout, "neighbors=%d\n", len(state.Neighbors))
	for _, neighbor := range state.Neighbors {
		fmt.Fprintf(i.stdout, "neighbor=%s mac=%s state=%s\n", neighbor.Address, neighbor.MAC, neighbor.State)
	}
	if state.ObservationError != "" {
		fmt.Fprintf(i.stdout, "observation_error=%s\n", state.ObservationError)
	}
	fmt.Fprintf(i.stdout, "install_dir=%s\n", state.InstallDir)
	fmt.Fprintf(i.stdout, "runtime_dir=%s\n", state.RuntimeDir)
	fmt.Fprintf(i.stdout, "plugin_log_file=%s exists=%t\n", state.PluginLogFile, fileExists(state.PluginLogFile))
	fmt.Fprintf(i.stdout, "monitor_file=%s exists=%t\n", state.MonitorFile, fileExists(state.MonitorFile))
	fmt.Fprintf(i.stdout, "monitor_config=%s exists=%t\n", state.MonitorConfig, fileExists(state.MonitorConfig))
	fmt.Fprintf(i.stdout, "log_file=%s\n", state.LogFile)
}

func (i *App) ensureNamespacePrereqs() error {
	if _, err := execLookPath("ip"); err != nil {
		return fmt.Errorf("ip command not found: %w", err)
	}
	return nil
}

func (i *App) prepareNamespaceMonitor() error {
	if err := os.MkdirAll(i.params.installDir, 0o755); err != nil {
		return err
	}
	if !fileExists(i.params.monitorFile) {
		if err := i.downloader.Download(i.params.monitorDownloadURL, i.params.monitorFile); err != nil {
			return err
		}
	}
	if err := chmodAdd(i.params.monitorFile, 0o111); err != nil {
		return err
	}
	if i.params.router == router.SteamDeck {
		if err := i.patchSteamDeckMonitorIdentity(); err != nil {
			return err
		}
	}
	return i.writeMonitorConfig(i.params.model)
}

func (i *App) stopLegacyHostPlugin() error {
	if err := i.runSilent("systemctl", "stop", "uuplugin"); err != nil {
		i.logf("stop legacy service warning: %v", err)
	}
	pids, err := plugin.PIDsByPattern(plugin.MonitorFilename)
	if err != nil {
		i.logf("list legacy monitor warning: %v", err)
	} else if err := terminatePIDList(pids); err != nil {
		i.logf("stop legacy monitor warning: %v", err)
	}
	if err := plugin.Stop(); err != nil {
		i.logf("stop legacy plugin warning: %v", err)
	}
	pids, err = plugin.PIDsByPattern(plugin.Executable)
	if err != nil {
		i.logf("list residual legacy plugin warning: %v", err)
	} else if err := terminatePIDList(namespacePluginPIDs(pids)); err != nil {
		i.logf("stop residual legacy plugin warning: %v", err)
	}
	i.cleanupStalePluginPIDFile()
	return nil
}

func (i *App) createNamespace(cfg namespaceConfig) (err error) {
	success := false
	defer func() {
		if success {
			return
		}
		_ = i.runSilent("ip", "link", "delete", cfg.Link)
		_ = i.runSilent("ip", "netns", "delete", cfg.Name)
		_ = os.RemoveAll(filepath.Join("/etc/netns", cfg.Name))
	}()

	_ = i.runSilent("ip", "link", "delete", cfg.Link)
	_ = os.RemoveAll(filepath.Join("/etc/netns", cfg.Name))

	if err := os.MkdirAll(filepath.Join("/etc/netns", cfg.Name), 0o755); err != nil {
		return err
	}
	if err := i.writeNamespaceDNS(cfg); err != nil {
		return err
	}
	if err := i.runCommand("ip", "netns", "add", cfg.Name); err != nil {
		return err
	}

	switch cfg.Mode {
	case "macvlan":
		if err := i.runCommand("ip", "link", "add", "link", cfg.Parent, "name", cfg.Link, "type", "macvlan", "mode", "bridge"); err != nil {
			return err
		}
	case "ipvlan":
		if err := i.runCommand("ip", "link", "add", "link", cfg.Parent, "name", cfg.Link, "type", "ipvlan", "mode", "l2"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported namespace mode %q", cfg.Mode)
	}

	if err := i.runCommand("ip", "link", "set", cfg.Link, "netns", cfg.Name); err != nil {
		return err
	}
	if err := i.runCommand("ip", "-n", cfg.Name, "link", "set", "lo", "up"); err != nil {
		return err
	}
	if err := i.runCommand("ip", "-n", cfg.Name, "addr", "add", cfg.Address, "dev", cfg.Link); err != nil {
		return err
	}
	if err := i.runCommand("ip", "-n", cfg.Name, "link", "set", cfg.Link, "up"); err != nil {
		return err
	}
	if err := i.runCommand("ip", "-n", cfg.Name, "route", "replace", "default", "via", cfg.Gateway, "dev", cfg.Link); err != nil {
		return err
	}
	success = true
	return nil
}

func (i *App) startNamespaceMonitor(cfg namespaceConfig) error {
	cmd := []string{"netns", "exec", cfg.Name, "/bin/sh", i.params.monitorFile}
	return i.startDetached("ip", cmd...)
}

func (i *App) waitNamespacePlugin(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pids, err := i.namespacePIDs(name)
		if err == nil {
			if len(namespacePluginPIDs(pids)) > 0 {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("%s did not start in namespace %s", plugin.Executable, name)
}

func (i *App) namespaceExists(name string) (bool, error) {
	out, err := i.commandOutput("ip", "netns", "list")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true, nil
		}
	}
	return false, nil
}

func (i *App) namespacePIDs(name string) ([]int, error) {
	out, err := i.commandOutput("ip", "netns", "pids", name)
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		for _, field := range fields {
			pid, err := strconv.Atoi(field)
			if err == nil {
				pids = append(pids, pid)
				break
			}
		}
	}
	sort.Ints(pids)
	return uniqueInts(pids), nil
}

func (i *App) namespaceNetworkState(cfg namespaceConfig) ([]namespaceSocket, []namespaceNeighbor, error) {
	sockets, socketErr := i.namespaceSockets(cfg)
	neighbors, neighborErr := i.namespaceNeighbors(cfg)
	return sockets, neighbors, errors.Join(socketErr, neighborErr)
}

func (i *App) namespaceSockets(cfg namespaceConfig) ([]namespaceSocket, error) {
	out, err := i.commandOutput("ip", "netns", "exec", cfg.Name, "ss", "-H", "-tunap")
	if err != nil {
		return nil, fmt.Errorf("inspect namespace sockets: %w", err)
	}
	sockets := parseNamespaceSockets(out)
	for idx := range sockets {
		sockets[idx].LANPeer = isNamespaceLANPeer(cfg, sockets[idx].PeerAddress)
	}
	return sockets, nil
}

func (i *App) namespaceNeighbors(cfg namespaceConfig) ([]namespaceNeighbor, error) {
	out, err := i.commandOutput("ip", "-n", cfg.Name, "neigh", "show", "dev", cfg.Link)
	if err != nil {
		return nil, fmt.Errorf("inspect namespace neighbors: %w", err)
	}
	return parseNamespaceNeighbors(out), nil
}

func parseNamespaceSockets(output string) []namespaceSocket {
	var sockets []namespaceSocket
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		localAddress, localPort := splitSocketEndpoint(fields[4])
		peerAddress, peerPort := splitSocketEndpoint(fields[5])
		socket := namespaceSocket{
			Protocol:     fields[0],
			State:        fields[1],
			LocalAddress: localAddress,
			LocalPort:    localPort,
			PeerAddress:  peerAddress,
			PeerPort:     peerPort,
		}
		if len(fields) > 6 {
			socket.Process = strings.Join(fields[6:], " ")
		}
		sockets = append(sockets, socket)
	}
	sort.Slice(sockets, func(left, right int) bool {
		a := sockets[left]
		b := sockets[right]
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		if a.LocalAddress != b.LocalAddress {
			return compareIP(a.LocalAddress, b.LocalAddress) < 0
		}
		if a.LocalPort != b.LocalPort {
			return a.LocalPort < b.LocalPort
		}
		if a.PeerAddress != b.PeerAddress {
			return compareIP(a.PeerAddress, b.PeerAddress) < 0
		}
		return a.PeerPort < b.PeerPort
	})
	return sockets
}

func parseNamespaceNeighbors(output string) []namespaceNeighbor {
	var neighbors []namespaceNeighbor
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || net.ParseIP(fields[0]) == nil {
			continue
		}
		neighbor := namespaceNeighbor{Address: fields[0]}
		for idx := 1; idx < len(fields); idx++ {
			if fields[idx] == "lladdr" && idx+1 < len(fields) {
				neighbor.MAC = fields[idx+1]
				idx++
				continue
			}
			if isNeighborState(fields[idx]) {
				neighbor.State = fields[idx]
			}
		}
		neighbors = append(neighbors, neighbor)
	}
	sort.Slice(neighbors, func(left, right int) bool {
		return compareIP(neighbors[left].Address, neighbors[right].Address) < 0
	})
	return neighbors
}

func splitSocketEndpoint(endpoint string) (string, string) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", ""
	}
	if host, port, err := net.SplitHostPort(endpoint); err == nil {
		return strings.Trim(host, "[]"), port
	}
	idx := strings.LastIndex(endpoint, ":")
	if idx < 0 {
		return strings.Trim(endpoint, "[]"), ""
	}
	return strings.Trim(endpoint[:idx], "[]"), endpoint[idx+1:]
}

func isNamespaceLANPeer(cfg namespaceConfig, address string) bool {
	ip := net.ParseIP(address)
	if ip == nil || ip.IsUnspecified() {
		return false
	}
	namespaceIP, network, err := net.ParseCIDR(cfg.Address)
	if err != nil || !network.Contains(ip) {
		return false
	}
	if ip.Equal(namespaceIP) {
		return false
	}
	gateway := net.ParseIP(cfg.Gateway)
	return gateway == nil || !ip.Equal(gateway)
}

func isNeighborState(value string) bool {
	switch value {
	case "INCOMPLETE", "REACHABLE", "STALE", "DELAY", "PROBE", "FAILED", "NOARP", "PERMANENT":
		return true
	default:
		return false
	}
}

func compareIP(left string, right string) int {
	leftIP := net.ParseIP(left)
	rightIP := net.ParseIP(right)
	if leftIP == nil || rightIP == nil {
		return strings.Compare(left, right)
	}
	leftBytes := leftIP.To16()
	rightBytes := rightIP.To16()
	if left4 := leftIP.To4(); left4 != nil {
		leftBytes = left4
	}
	if right4 := rightIP.To4(); right4 != nil {
		rightBytes = right4
	}
	return bytes.Compare(leftBytes, rightBytes)
}

func (i *App) cleanupLegacyPIDFile(pids []int) {
	if len(pids) == 0 {
		return
	}
	pidSet := map[int]struct{}{}
	for _, pid := range pids {
		pidSet[pid] = struct{}{}
	}
	pidText, err := os.ReadFile(plugin.PIDFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		return
	}
	if _, ok := pidSet[pid]; ok {
		_ = os.Remove(plugin.PIDFile)
	}
}

func (i *App) cleanupStalePluginPIDFile() {
	pidText, err := os.ReadFile(plugin.PIDFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err == nil && plugin.ProcessMatches(pid, plugin.Executable) {
		return
	}
	if err := os.Remove(plugin.PIDFile); err != nil && !os.IsNotExist(err) {
		i.logf("remove stale %s failed: %v", plugin.PIDFile, err)
	}
}

func (i *App) writeNamespaceDNS(cfg namespaceConfig) error {
	path := filepath.Join("/etc/netns", cfg.Name, "resolv.conf")
	var builder strings.Builder
	for _, dns := range cfg.DNS {
		builder.WriteString("nameserver ")
		builder.WriteString(dns)
		builder.WriteString("\n")
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func (i *App) resolveNamespaceConfig() (namespaceConfig, error) {
	name := firstNonEmpty(i.params.namespaceName, "uu-ns")
	parent := strings.TrimSpace(i.params.namespaceParent)
	if parent == "" {
		var err error
		parent, err = defaultRouteInterface()
		if err != nil {
			return namespaceConfig{}, err
		}
	}

	link := firstNonEmpty(i.params.namespaceLink, "uu-macvlan0")
	mode := strings.ToLower(firstNonEmpty(i.params.namespaceMode, "macvlan"))
	dns, err := parseNamespaceDNS(i.params.namespaceDNS)
	if err != nil {
		return namespaceConfig{}, err
	}
	gateway := strings.TrimSpace(i.params.namespaceGateway)
	if gateway == "" {
		gateway, err = defaultRouteGateway()
		if err != nil {
			return namespaceConfig{}, err
		}
	}
	address := strings.TrimSpace(i.params.namespaceAddress)
	if address == "" {
		address, err = inferredNamespaceAddress(parent)
		if err != nil {
			return namespaceConfig{}, err
		}
	}

	return namespaceConfig{
		Name:    name,
		Parent:  parent,
		Link:    link,
		Address: address,
		Gateway: gateway,
		DNS:     dns,
		Mode:    mode,
	}, nil
}

func validateNamespaceConfig(cfg namespaceConfig) error {
	if err := validateNamespaceName(cfg.Name); err != nil {
		return err
	}
	if err := validateInterfaceName(cfg.Parent); err != nil {
		return fmt.Errorf("invalid namespace parent interface %q: %w", cfg.Parent, err)
	}
	if err := validateInterfaceName(cfg.Link); err != nil {
		return fmt.Errorf("invalid namespace link name %q: %w", cfg.Link, err)
	}
	ip, _, err := net.ParseCIDR(cfg.Address)
	if err != nil {
		return fmt.Errorf("invalid namespace address %q: %w", cfg.Address, err)
	}
	if ip.To4() == nil {
		return fmt.Errorf("namespace address %q must be ipv4", cfg.Address)
	}
	if ip := net.ParseIP(cfg.Gateway); ip == nil || ip.To4() == nil {
		return fmt.Errorf("invalid namespace gateway %q", cfg.Gateway)
	}
	if len(cfg.DNS) == 0 {
		return fmt.Errorf("at least one namespace dns server is required")
	}
	switch cfg.Mode {
	case "macvlan", "ipvlan":
		return nil
	default:
		return fmt.Errorf("unsupported namespace mode %q", cfg.Mode)
	}
}

func validateNamespaceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("namespace name is required")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return fmt.Errorf("namespace name contains invalid character %q", r)
		}
	}
	return nil
}

func validateInterfaceName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("interface name is required")
	}
	if len(name) > 15 {
		return fmt.Errorf("interface name %q exceeds linux limit", name)
	}
	if strings.Contains(name, "/") || strings.ContainsAny(name, " \t\n\r") {
		return fmt.Errorf("interface name %q contains invalid characters", name)
	}
	return nil
}

func parseNamespaceDNS(value string) ([]string, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(parts) == 0 {
		return nil, fmt.Errorf("no namespace dns servers configured")
	}
	dns := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(part); err == nil {
			part = host
		}
		if ip := net.ParseIP(part); ip == nil {
			return nil, fmt.Errorf("invalid dns server %q", part)
		}
		dns = append(dns, part)
	}
	if len(dns) == 0 {
		return nil, fmt.Errorf("no namespace dns servers configured")
	}
	return dns, nil
}

func defaultRouteInterface() (string, error) {
	out, err := execLookOutput("ip", "-4", "route", "show", "default")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for idx := 0; idx < len(fields); idx++ {
			if fields[idx] == "dev" && idx+1 < len(fields) {
				return fields[idx+1], nil
			}
		}
	}
	return "", fmt.Errorf("unable to determine default route interface")
}

func defaultRouteGateway() (string, error) {
	out, err := execLookOutput("ip", "-4", "route", "show", "default")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		for idx := 0; idx < len(fields); idx++ {
			if fields[idx] == "via" && idx+1 < len(fields) {
				return fields[idx+1], nil
			}
		}
	}
	return "", fmt.Errorf("unable to determine default route gateway")
}

func inferredNamespaceAddress(parent string) (string, error) {
	iface, err := net.InterfaceByName(parent)
	if err != nil {
		return "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		var ipnet *net.IPNet
		switch v := addr.(type) {
		case *net.IPNet:
			ipnet = v
		case *net.IPAddr:
			if v.IP == nil {
				continue
			}
			ipnet = &net.IPNet{IP: v.IP, Mask: v.IP.DefaultMask()}
		}
		if ipnet == nil {
			continue
		}
		ip4 := ipnet.IP.To4()
		if ip4 == nil {
			continue
		}
		ones, bits := ipnet.Mask.Size()
		if bits != 32 {
			continue
		}
		candidate := net.IPv4(ip4[0], ip4[1], ip4[2], 250)
		if candidate.Equal(ip4) || !ipnet.Contains(candidate) {
			candidate = net.IPv4(ip4[0], ip4[1], ip4[2], 251)
		}
		if candidate.Equal(ip4) || !ipnet.Contains(candidate) {
			return "", fmt.Errorf("unable to infer address for %s; provide --namespace-address", parent)
		}
		return candidate.String() + "/" + strconv.Itoa(ones), nil
	}
	return "", fmt.Errorf("parent interface %s has no ipv4 address", parent)
}

func filterPIDs(pids []int, pattern string) []int {
	var matched []int
	for _, pid := range pids {
		if plugin.ProcessMatches(pid, pattern) {
			matched = append(matched, pid)
		}
	}
	sort.Ints(matched)
	return uniqueInts(matched)
}

func namespacePluginPIDs(pids []int) []int {
	var matched []int
	for _, pid := range pids {
		if isNamespacePluginProcess(pid) {
			matched = append(matched, pid)
		}
	}
	sort.Ints(matched)
	return uniqueInts(matched)
}

func isNamespacePluginProcess(pid int) bool {
	if !plugin.ProcessMatches(pid, plugin.Executable) {
		return false
	}
	if plugin.ProcessMatches(pid, plugin.MonitorFilename) {
		return false
	}
	return true
}

func terminatePIDList(pids []int) error {
	for _, pid := range pids {
		if pid <= 0 || pid == os.Getpid() {
			continue
		}
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			return err
		}
	}
	for attempt := 0; attempt < 30; attempt++ {
		if !anyPIDAlive(pids) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, pid := range pids {
		if pid <= 0 || pid == os.Getpid() {
			continue
		}
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
	}
	return nil
}

func anyPIDAlive(pids []int) bool {
	for _, pid := range pids {
		if pid <= 0 || pid == os.Getpid() {
			continue
		}
		if err := syscall.Kill(pid, 0); err == nil || err == syscall.EPERM {
			return true
		}
	}
	return false
}

func uniqueInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	var last int
	for idx, value := range values {
		if idx > 0 && value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}

func boolState(v bool) string {
	if v {
		return "running"
	}
	return "stopped"
}

func execLookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func execLookOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
