package plugin

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	Executable        = "uuplugin"
	MonitorFilename   = "uuplugin_monitor.sh"
	MonitorConfigName = "uuplugin_monitor.config"
	UninstallFilename = "uninstall.sh"
	PIDFile           = "/var/run/uuplugin.pid"
)

func Status() (int, bool) {
	pid, err := readPID()
	if err != nil {
		return 0, false
	}
	return pid, ProcessMatches(pid, Executable)
}

func WaitRunning(attempts int, interval time.Duration) error {
	for attempt := 0; attempt < attempts; attempt++ {
		if _, running := Status(); running {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("%s is not running", Executable)
}

func Stop() error {
	pid, running := Status()
	if !running {
		return nil
	}
	return terminatePID(pid)
}

func StopProcessesByPattern(pattern string) error {
	pids, err := PIDsByPattern(pattern)
	if err != nil {
		return err
	}
	for _, pid := range pids {
		if pid == os.Getpid() {
			continue
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		_ = process.Signal(syscall.SIGTERM)
	}
	return nil
}

func ProcessMatches(pid int, name string) bool {
	if pid <= 0 {
		return false
	}

	readAnyProcFile := false
	for _, procFile := range []string{
		fmt.Sprintf("/proc/%d/comm", pid),
		fmt.Sprintf("/proc/%d/cmdline", pid),
	} {
		content, err := os.ReadFile(procFile)
		if err != nil {
			continue
		}
		readAnyProcFile = true
		content = bytes.ReplaceAll(content, []byte{0}, []byte(" "))
		if strings.Contains(string(content), name) {
			return true
		}
	}
	if readAnyProcFile {
		return false
	}

	return psContains(pid, name, "ps") || psContains(pid, name, "ps", "-ax", "-o", "pid,cmd")
}

func PIDsByPattern(pattern string) ([]int, error) {
	out, err := exec.Command("ps", "-ax", "-o", "pid=,cmd=").Output()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, pattern) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func readPID() (int, error) {
	pidText, err := os.ReadFile(PIDFile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func terminatePID(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	for attempt := 0; attempt < 30; attempt++ {
		if !ProcessMatches(pid, Executable) {
			_ = os.Remove(PIDFile)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := process.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	_ = os.Remove(PIDFile)
	return nil
}

func psContains(pid int, name string, args ...string) bool {
	if len(args) == 0 {
		return false
	}
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		return false
	}
	pidText := strconv.Itoa(pid)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != pidText {
			continue
		}
		if strings.Contains(line, name) {
			return true
		}
	}
	return false
}
