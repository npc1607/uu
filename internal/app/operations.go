package app

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/npc1607/uu/internal/logtail"
	"github.com/npc1607/uu/internal/plugin"
	"github.com/npc1607/uu/internal/router"
)

func (i *App) Start() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "start failed: %v\n", err)
		return 1
	}

	if pid, running := plugin.Status(); running {
		fmt.Fprintf(i.stdout, "%s already running (pid %d)\n", plugin.Executable, pid)
		return i.finishSuccessfulInstall()
	}

	var err error
	if i.params.router == router.SteamDeck {
		err = i.runCommand("systemctl", "start", "uuplugin")
	} else {
		err = i.startMonitor()
	}
	if err != nil {
		fmt.Fprintf(i.stderr, "start failed: %v\n", err)
		return 1
	}

	if err := plugin.WaitRunning(90, time.Second); err != nil {
		fmt.Fprintf(i.stderr, "start failed: %v\n", err)
		return 1
	}
	pid, _ := plugin.Status()
	fmt.Fprintf(i.stdout, "%s started (pid %d)\n", plugin.Executable, pid)
	return i.finishSuccessfulInstall()
}

func (i *App) Stop() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "stop failed: %v\n", err)
		return 1
	}

	var serviceErr error
	if i.params.router == router.SteamDeck {
		serviceErr = i.runCommand("systemctl", "stop", "uuplugin")
	}

	monitorErr := plugin.StopProcessesByPattern(plugin.MonitorFilename)
	pluginErr := plugin.Stop()
	if serviceErr != nil && pluginErr != nil {
		fmt.Fprintf(i.stderr, "stop failed: service=%v plugin=%v\n", serviceErr, pluginErr)
		return 1
	}
	if monitorErr != nil {
		i.logf("stop monitor warning: %v", monitorErr)
	}

	if pid, running := plugin.Status(); running {
		fmt.Fprintf(i.stderr, "%s is still running (pid %d)\n", plugin.Executable, pid)
		return 1
	}
	fmt.Fprintf(i.stdout, "%s stopped\n", plugin.Executable)
	return 0
}

func (i *App) Status() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "status failed: %v\n", err)
		return 1
	}

	if i.params.router == router.SteamDeck {
		if state, err := systemdState("uuplugin"); err == nil && state != "" {
			fmt.Fprintf(i.stdout, "service=uuplugin state=%s\n", state)
		}
	}

	pid, running := plugin.Status()
	if running {
		fmt.Fprintf(i.stdout, "%s running (pid %d)\n", plugin.Executable, pid)
		return 0
	}
	if pid > 0 {
		fmt.Fprintf(i.stdout, "%s not running (stale pid %d)\n", plugin.Executable, pid)
		return 1
	}
	fmt.Fprintf(i.stdout, "%s not running\n", plugin.Executable)
	return 1
}

func (i *App) Logs() int {
	i.openLog(false)
	defer i.closeLog()
	if err := logtail.Follow(logtail.Options{
		File:    i.opts.FollowLogFile,
		Lines:   i.opts.FollowLogLines,
		Timeout: i.opts.FollowLogTimeout,
		Writer:  i.stdout,
	}); err != nil {
		fmt.Fprintf(i.stderr, "logs failed: %v\n", err)
		return 1
	}
	return 0
}

func systemdState(service string) (string, error) {
	out, err := exec.Command("systemctl", "is-active", service).Output()
	state := strings.TrimSpace(string(out))
	if err != nil && state == "" {
		return "", err
	}
	return state, nil
}
