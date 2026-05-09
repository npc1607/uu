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
	FollowLogs       bool
	FollowLogFile    string
	FollowLogLines   int
	FollowLogTimeout time.Duration
}

func DefaultOptions(baseDir string) Options {
	return Options{
		Router:         router.Default,
		Model:          router.DefaultModel,
		BaseDir:        baseDir,
		LogDir:         DefaultLogDir,
		FollowLogFile:  DefaultFollowLogFile,
		FollowLogLines: DefaultFollowLogLines,
	}
}
