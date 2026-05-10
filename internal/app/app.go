package app

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/npc1607/uu/internal/config"
	"github.com/npc1607/uu/internal/downloader"
	"github.com/npc1607/uu/internal/logtail"
	"github.com/npc1607/uu/internal/plugin"
	"github.com/npc1607/uu/internal/router"
)

type App struct {
	opts        config.Options
	params      runtimeParams
	client      *http.Client
	downloader  downloader.Client
	stdout      io.Writer
	stderr      io.Writer
	logFile     *os.File
	logPath     string
	logger      *log.Logger
	logWriter   io.Writer
	initialized bool
}

type Option func(*App)

func WithOutput(stdout io.Writer, stderr io.Writer) Option {
	return func(app *App) {
		if stdout != nil {
			app.stdout = stdout
		}
		if stderr != nil {
			app.stderr = stderr
		}
	}
}

func New(opts config.Options, options ...Option) *App {
	if opts.Router == "" {
		opts.Router = router.Default
	}
	if opts.Model == "" {
		opts.Model = router.DefaultModel
	}
	if opts.LogDir == "" {
		opts.LogDir = config.DefaultLogDir
	}
	if opts.FollowLogFile == "" {
		opts.FollowLogFile = config.DefaultFollowLogFile
	}
	if opts.FollowLogLines < 0 {
		opts.FollowLogLines = 0
	}
	if opts.BaseDir == "" {
		opts.BaseDir = "."
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Match the original curl -k behavior.

	client := &http.Client{Timeout: 60 * time.Second, Transport: transport}
	app := &App{
		opts:       opts,
		client:     client,
		downloader: downloader.Client{HTTP: client},
		stdout:     os.Stdout,
		stderr:     os.Stderr,
	}
	for _, option := range options {
		option(app)
	}
	return app
}

func (i *App) Install() (status int) {
	i.openLog(true)
	defer i.closeLog()
	if i.logPath != "" {
		fmt.Fprintf(i.stdout, "Install log: %s\n", i.logPath)
	}
	defer func() {
		if !i.initialized {
			return
		}
		if status > 4 && fileExists(i.params.uninstallFile) {
			fmt.Fprintf(i.stdout, "Cleaning up partial install; details: %s\n", i.logPath)
			if err := i.cleanUp(); err != nil {
				i.logf("final cleanup failed: %v", err)
			}
		}
		if fileExists(i.params.uninstallFile) {
			if err := os.Remove(i.params.uninstallFile); err != nil {
				i.logf("remove temporary uninstall failed: %v", err)
			}
		}
	}()

	if err := i.initParams(); err != nil {
		return i.failInstall(9, "init params failed: %v", err)
	}

	if err := i.configRouter(); err != nil {
		return i.failInstall(1, "config router failed: %v", err)
	}

	if err := os.MkdirAll(i.params.installDir, 0o755); err != nil {
		return i.failInstall(2, "create install dir %s failed: %v", i.params.installDir, err)
	}

	if err := i.downloader.Download(i.params.uninstallDownloadURL, i.params.uninstallFile); err != nil {
		return i.failInstall(3, "download uninstall script failed: %v", err)
	}

	if err := i.cleanUp(); err != nil {
		return i.failInstall(4, "cleanup old install failed: %v", err)
	}

	if err := i.downloader.Download(i.params.monitorDownloadURL, i.params.monitorFile); err != nil {
		_ = os.Remove(i.params.monitorFile)
		return i.failInstall(5, "download monitor script failed: %v", err)
	}
	if err := chmodAdd(i.params.monitorFile, 0o111); err != nil {
		return i.failInstall(5, "chmod monitor script failed: %v", err)
	}

	if i.params.router == router.SteamDeck {
		if err := i.patchSteamDeckMonitorIdentity(); err != nil {
			return i.failInstall(6, "patch monitor identity persistence failed: %v", err)
		}
		if err := i.writeMonitorConfig(router.DefaultModel); err != nil {
			return i.failInstall(6, "write monitor config failed: %v", err)
		}
		if err := i.configBootup(); err != nil {
			return i.failInstall(6, "configure boot service failed: %v", err)
		}
		if err := plugin.WaitRunning(90, time.Second); err != nil {
			return i.failInstall(6, "uuplugin did not start: %v", err)
		}
		if err := i.createUninstall(); err != nil {
			return i.failInstall(6, "create uninstall script failed: %v", err)
		}
		fmt.Fprintln(i.stdout, "Installation succeeded!")
		return i.finishSuccessfulInstall()
	}

	if err := i.startMonitor(); err != nil {
		return i.failInstall(6, "start monitor failed: %v", err)
	}

	if err := plugin.WaitRunning(90, time.Second); err != nil {
		return i.failInstall(7, "uuplugin did not start: %v", err)
	}

	if err := i.configBootup(); err != nil {
		return i.failInstall(8, "configure boot service failed: %v", err)
	}

	if err := i.printSN(); err != nil {
		return i.failInstall(10, "print sn failed: %v", err)
	}
	return i.finishSuccessfulInstall()
}

func (i *App) failInstall(code int, format string, args ...any) int {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(i.stderr, "Installation failed: %s\n", msg)
	if i.logPath != "" {
		fmt.Fprintf(i.stderr, "Install log: %s\n", i.logPath)
	}
	i.logf("installation failed: %s", msg)
	return code
}

func (i *App) finishSuccessfulInstall() int {
	if i.initialized && fileExists(i.params.uninstallFile) {
		if err := os.Remove(i.params.uninstallFile); err != nil {
			i.logf("remove temporary uninstall failed: %v", err)
		}
	}
	if !i.opts.FollowLogs {
		return 0
	}
	if err := logtail.Follow(logtail.Options{
		File:    i.opts.FollowLogFile,
		Lines:   i.opts.FollowLogLines,
		Timeout: i.opts.FollowLogTimeout,
		Writer:  i.stdout,
	}); err != nil {
		fmt.Fprintf(i.stderr, "follow log failed: %v\n", err)
		i.logf("follow log failed: %v", err)
	}
	return 0
}

func (i *App) writeMonitorConfig(model string) error {
	content := fmt.Sprintf("router=%s\nmodel=%s\n", i.params.router, model)
	return os.WriteFile(i.params.monitorConfig, []byte(content), 0o644)
}

func (i *App) startMonitor() error {
	if !fileExists(i.params.monitorFile) {
		return fmt.Errorf("%s does not exist", i.params.monitorFile)
	}
	if err := i.writeMonitorConfig(i.params.model); err != nil {
		return err
	}
	if err := chmodAdd(i.params.monitorFile, 0o100); err != nil {
		return err
	}
	return i.startDetached("/bin/sh", i.params.monitorFile)
}
