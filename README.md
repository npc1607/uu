# uu

Go rewrite of `uu.sh`.

## Build

```sh
go build -buildvcs=false -o uu ./cmd/uu
```

## Run

```sh
sudo ./uu install
```

The default behavior matches the original script defaults:

```yaml
router: steam-deck-plugin
model: x86_64
```

For `steam-deck-plugin`, the default install directory is the directory containing the `uu` binary. Use `--install-dir` or `install_dir` in YAML to override it.

Optional flags:

```sh
sudo ./uu --router steam-deck-plugin --model x86_64
sudo ./uu --config configs/uu.yaml.example
sudo ./uu --follow-logs
```

Commands:

```sh
sudo ./uu install   # install or reinstall
sudo ./uu start     # start the installed service/monitor only
sudo ./uu stop      # stop the service/process
sudo ./uu status    # print service/process status
sudo ./uu logs      # follow the configured log file
```

Running `./uu` without a command is kept as a compatibility shortcut for `./uu install`.

The YAML config intentionally uses a flat schema, including `router`, `model`, `install_dir`, `log_dir`, and the `follow_log_*` options below.

To keep the terminal attached and print monitor logs after install:

```yaml
follow_logs: true
follow_log_file: /tmp/monitor.log
follow_log_lines: 100
follow_log_timeout: 0
```

## Internal Layout

```text
cmd/uu              CLI entrypoint and flag parsing
internal/app        install/start/stop/status/logs orchestration
internal/config     config defaults and YAML loading
internal/downloader remote script download and MD5 verification
internal/logtail    terminal log following
internal/plugin     uuplugin process state and stop helpers
internal/router     supported router names and defaults
```
