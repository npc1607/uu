package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/npc1607/uu/internal/app"
	"github.com/npc1607/uu/internal/config"
)

func main() {
	command, opts, err := parseCommandOptions(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	application := app.New(opts)
	switch command {
	case "install":
		os.Exit(application.Install())
	case "start":
		os.Exit(application.Start())
	case "stop":
		os.Exit(application.Stop())
	case "status":
		os.Exit(application.Status())
	case "logs":
		os.Exit(application.Logs())
	case "ns-start":
		os.Exit(application.NamespaceStart())
	case "ns-stop":
		os.Exit(application.NamespaceStop())
	case "ns-status":
		os.Exit(application.NamespaceStatus())
	case "serve":
		os.Exit(application.Serve())
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		os.Exit(2)
	}
}

func parseCommandOptions(args []string) (string, config.Options, error) {
	command := "install"
	if len(args) > 0 && isCommand(args[0]) {
		command = args[0]
		args = args[1:]
	}

	baseDir := "."
	if exe, err := os.Executable(); err == nil {
		baseDir = filepath.Dir(exe)
	}

	opts := config.DefaultOptions(baseDir)
	cli := opts
	var configPath string

	fs := flag.NewFlagSet("uu", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: uu [install|start|stop|status|logs|ns-start|ns-stop|ns-status|serve] [flags]\n\n")
		fmt.Fprintf(fs.Output(), "Commands:\n")
		fmt.Fprintf(fs.Output(), "  install  install or reinstall the plugin\n")
		fmt.Fprintf(fs.Output(), "  start    start the installed plugin service/monitor\n")
		fmt.Fprintf(fs.Output(), "  stop     stop the plugin service/process\n")
		fmt.Fprintf(fs.Output(), "  status   print plugin process status\n")
		fmt.Fprintf(fs.Output(), "  logs     follow the configured log file\n\n")
		fmt.Fprintf(fs.Output(), "  ns-start   start official uuplugin inside an isolated network namespace\n")
		fmt.Fprintf(fs.Output(), "  ns-stop    stop and remove the isolated network namespace\n")
		fmt.Fprintf(fs.Output(), "  ns-status  print isolated namespace status\n")
		fmt.Fprintf(fs.Output(), "  serve      start the local web control page\n\n")
		fmt.Fprintf(fs.Output(), "Flags:\n")
		fs.PrintDefaults()
	}
	fs.StringVar(&configPath, "config", "", "optional YAML config file")
	fs.StringVar(&cli.Router, "router", opts.Router, "router type")
	fs.StringVar(&cli.Model, "model", opts.Model, "device model")
	fs.StringVar(&cli.InstallDir, "install-dir", opts.InstallDir, "override install directory")
	fs.StringVar(&cli.LogDir, "log-dir", opts.LogDir, "application log directory")
	fs.StringVar(&cli.WebListen, "listen", opts.WebListen, "local web control listen address")
	fs.StringVar(&cli.NamespaceName, "namespace-name", opts.NamespaceName, "network namespace name for isolated mode")
	fs.StringVar(&cli.NamespaceParent, "namespace-parent", opts.NamespaceParent, "parent LAN interface for isolated mode; empty means default route interface")
	fs.StringVar(&cli.NamespaceLink, "namespace-link", opts.NamespaceLink, "namespace LAN link name for isolated mode")
	fs.StringVar(&cli.NamespaceAddress, "namespace-address", opts.NamespaceAddress, "isolated LAN address in CIDR form; empty means auto .250 on parent /24")
	fs.StringVar(&cli.NamespaceGateway, "namespace-gateway", opts.NamespaceGateway, "isolated LAN default gateway; empty means host default gateway")
	fs.StringVar(&cli.NamespaceDNS, "namespace-dns", opts.NamespaceDNS, "comma-separated DNS servers for /etc/netns/<name>/resolv.conf")
	fs.StringVar(&cli.NamespaceMode, "namespace-mode", opts.NamespaceMode, "isolated link mode; currently macvlan")
	fs.BoolVar(&cli.FollowLogs, "follow-logs", opts.FollowLogs, "follow process log after successful install")
	fs.StringVar(&cli.FollowLogFile, "follow-log-file", opts.FollowLogFile, "log file to follow after successful install")
	fs.IntVar(&cli.FollowLogLines, "follow-log-lines", opts.FollowLogLines, "number of existing log lines to print before following")
	fs.DurationVar(&cli.FollowLogTimeout, "follow-log-timeout", opts.FollowLogTimeout, "stop following logs after this duration; 0 means no timeout")

	if err := fs.Parse(args); err != nil {
		return command, opts, err
	}
	if fs.NArg() > 0 {
		return command, opts, fmt.Errorf("unknown argument: %s", fs.Arg(0))
	}

	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			return command, opts, err
		}
		cfg.Apply(&opts)
	}

	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	if visited["router"] {
		opts.Router = cli.Router
	}
	if visited["model"] {
		opts.Model = cli.Model
	}
	if visited["install-dir"] {
		opts.InstallDir = cli.InstallDir
	}
	if visited["log-dir"] {
		opts.LogDir = cli.LogDir
	}
	if visited["listen"] {
		opts.WebListen = cli.WebListen
	}
	if visited["namespace-name"] {
		opts.NamespaceName = cli.NamespaceName
	}
	if visited["namespace-parent"] {
		opts.NamespaceParent = cli.NamespaceParent
	}
	if visited["namespace-link"] {
		opts.NamespaceLink = cli.NamespaceLink
	}
	if visited["namespace-address"] {
		opts.NamespaceAddress = cli.NamespaceAddress
	}
	if visited["namespace-gateway"] {
		opts.NamespaceGateway = cli.NamespaceGateway
	}
	if visited["namespace-dns"] {
		opts.NamespaceDNS = cli.NamespaceDNS
	}
	if visited["namespace-mode"] {
		opts.NamespaceMode = cli.NamespaceMode
	}
	if visited["follow-logs"] {
		opts.FollowLogs = cli.FollowLogs
	}
	if visited["follow-log-file"] {
		opts.FollowLogFile = cli.FollowLogFile
	}
	if visited["follow-log-lines"] {
		opts.FollowLogLines = cli.FollowLogLines
	}
	if visited["follow-log-timeout"] {
		opts.FollowLogTimeout = cli.FollowLogTimeout
	}

	return command, opts, nil
}

func isCommand(arg string) bool {
	switch arg {
	case "install", "start", "stop", "status", "logs", "ns-start", "ns-stop", "ns-status", "serve":
		return true
	default:
		return false
	}
}
