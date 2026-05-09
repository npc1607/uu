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
	defer func() {
		if !i.initialized {
			return
		}
		if status > 4 && fileExists(i.params.uninstallFile) {
			fmt.Fprintln(i.stdout, "Cleaning up.")
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
		i.logf("init params failed: %v", err)
		return 9
	}

	if err := i.configRouter(); err != nil {
		i.logf("config router failed: %v", err)
		return 1
	}

	if err := os.MkdirAll(i.params.installDir, 0o755); err != nil {
		i.logf("create install dir failed: %v", err)
		return 2
	}

	if err := i.downloader.Download(i.params.uninstallDownloadURL, i.params.uninstallFile); err != nil {
		i.logf("download uninstall failed: %v", err)
		return 3
	}

	if err := i.cleanUp(); err != nil {
		i.logf("cleanup failed: %v", err)
		return 4
	}

	if err := i.downloader.Download(i.params.monitorDownloadURL, i.params.monitorFile); err != nil {
		_ = os.Remove(i.params.monitorFile)
		i.logf("download monitor failed: %v", err)
		return 5
	}
	if err := chmodAdd(i.params.monitorFile, 0o111); err != nil {
		i.logf("chmod monitor failed: %v", err)
		return 5
	}

	if i.params.router == router.SteamDeck {
		if err := i.writeMonitorConfig(router.DefaultModel); err != nil {
			fmt.Fprintln(i.stdout, "Installation failed!")
			i.logf("write monitor config failed: %v", err)
			return 6
		}
		bootErr := i.configBootup()
		runningErr := plugin.WaitRunning(90, time.Second)
		uninstallErr := i.createUninstall()
		if bootErr != nil || runningErr != nil || uninstallErr != nil {
			fmt.Fprintln(i.stdout, "Installation failed!")
			i.logf("steam deck boot=%v running=%v create_uninstall=%v", bootErr, runningErr, uninstallErr)
			return 6
		}
		fmt.Fprintln(i.stdout, "Installation succeeded!")
		return i.finishSuccessfulInstall()
	}

	if err := i.startMonitor(); err != nil {
		i.logf("start monitor failed: %v", err)
		return 6
	}

	if err := plugin.WaitRunning(90, time.Second); err != nil {
		i.logf("check running failed: %v", err)
		return 7
	}

	if err := i.configBootup(); err != nil {
		i.logf("config bootup failed: %v", err)
		return 8
	}

	if err := i.printSN(); err != nil {
		i.logf("print sn failed: %v", err)
		return 10
	}
	return i.finishSuccessfulInstall()
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

func (i *App) cleanUp() error {
	if !fileExists(i.params.uninstallFile) {
		return fmt.Errorf("%s does not exist", i.params.uninstallFile)
	}
	if err := chmodAdd(i.params.uninstallFile, 0o100); err != nil {
		return err
	}
	return i.runSilent("/bin/sh", i.params.uninstallFile, i.params.router, i.params.model)
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
