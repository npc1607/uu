package app

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

func (i *App) openLog(cleanOld bool) {
	i.logWriter = io.Discard
	if i.opts.LogDir == "" {
		i.logger = log.New(i.logWriter, "", log.LstdFlags)
		return
	}
	if err := os.MkdirAll(i.opts.LogDir, 0o755); err != nil {
		fmt.Fprintf(i.stderr, "create log dir failed: %v\n", err)
		i.logger = log.New(i.logWriter, "", log.LstdFlags)
		return
	}
	if cleanOld {
		matches, _ := filepath.Glob(filepath.Join(i.opts.LogDir, steamDeckLogPattern))
		for _, match := range matches {
			_ = os.Remove(match)
		}
	}
	logPath := filepath.Join(i.opts.LogDir, fmt.Sprintf("%s%d.log", steamDeckLogPrefix, time.Now().Unix()))
	file, err := os.Create(logPath)
	if err != nil {
		fmt.Fprintf(i.stderr, "create log file failed: %v\n", err)
		i.logger = log.New(i.logWriter, "", log.LstdFlags)
		return
	}
	i.logFile = file
	i.logWriter = file
	i.logger = log.New(file, "", log.LstdFlags)
}

func (i *App) closeLog() {
	if i.logFile != nil {
		_ = i.logFile.Close()
	}
}

func (i *App) logf(format string, args ...any) {
	if i.logger != nil {
		i.logger.Printf(format, args...)
	}
}
