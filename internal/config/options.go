package config

import (
	"time"

	"github.com/npc1607/uu/internal/router"
)

const (
	DefaultLogDir         = "/tmp"
	DefaultFollowLogFile  = "/tmp/monitor.log"
	DefaultFollowLogLines = 100
)

type Options struct {
	Router           string
	Model            string
	BaseDir          string
	InstallDir       string
	LogDir           string
	WebListen        string
	NamespaceName    string
	NamespaceParent  string
	NamespaceLink    string
	NamespaceAddress string
	NamespaceGateway string
	NamespaceDNS     string
	NamespaceMode    string
	FollowLogs       bool
	FollowLogFile    string
	FollowLogLines   int
	FollowLogTimeout time.Duration
}

func DefaultOptions(baseDir string) Options {
	return Options{
		Router:        router.Default,
		Model:         router.DefaultModel,
		BaseDir:       baseDir,
		LogDir:        DefaultLogDir,
		WebListen:     "127.0.0.1:8088",
		NamespaceName: "uu-ns",
		NamespaceLink: "uu-macvlan0",
		NamespaceDNS:  "223.5.5.5,119.29.29.29",
		NamespaceMode: "macvlan",
		FollowLogFile: DefaultFollowLogFile,
		FollowLogLines: DefaultFollowLogLines,
	}
}
