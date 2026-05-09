package app

import (
	"io"
	"os/exec"
	"strings"
	"syscall"
)

func (i *App) runCommand(name string, args ...string) error {
	i.logf("run: %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = i.stdout
	cmd.Stderr = i.logWriter
	return cmd.Run()
}

func (i *App) runSilent(name string, args ...string) error {
	i.logf("run silent: %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func (i *App) startDetached(name string, args ...string) error {
	i.logf("start detached: %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
