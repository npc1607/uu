package config

import "github.com/npc1607/uu/internal/router"

const (
	DefaultLogDir = "/tmp"
)

type Options struct {
	Router     string
	Model      string
	BaseDir    string
	InstallDir string
	LogDir     string
}

func DefaultOptions(baseDir string) Options {
	return Options{
		Router:  router.Default,
		Model:   router.DefaultModel,
		BaseDir: baseDir,
		LogDir:  DefaultLogDir,
	}
}
