# uu

Go rewrite of `uu.sh`.

## Why This Version

Compared with the original `uu.sh` installer, this Go version keeps the same default Steam Deck install behavior while adding a few practical improvements:

- Persistent Steam Deck identity: `.uuplugin_uuid` and `.uid` are saved in the install directory and restored to `/tmp/uu`, so reinstalling, upgrading, or cleaning up the runtime directory does not create a new plugin identity.
- Reusable CLI commands: install, start, stop, status, and logs are available without rerunning the full shell installer flow.
- Isolated namespace mode: the official `uuplugin` can be started inside a dedicated `uu-ns` network namespace with its own macvlan link, DNS, and routing so it does not share the host network stack.
- Local control page: `serve` starts a loopback-only HTML control surface for namespace start, stop, restart, and status.
- Clearer diagnostics: install logs, resolved paths, systemd state, monitor state, and `uuplugin` process state are easier to inspect.
- Configurable install options: router, model, install directory, log directory, and log-following behavior can be set with flags or YAML.

## Build

```sh
make build
```

Release assets are built by GitHub Actions when a `v*` tag is pushed. The workflow uploads a `linux/amd64` tarball and checksum file to the GitHub Release.

## Run

```sh
sudo ./bin/uu install
```

The default behavior matches the original script defaults:

```yaml
router: steam-deck-plugin
model: x86_64
```

For `steam-deck-plugin`, the default install directory is the directory containing the `uu` binary. Use `--install-dir` or `install_dir` in YAML to override it.

Optional flags:

```sh
sudo ./bin/uu --router steam-deck-plugin --model x86_64
sudo ./bin/uu --config configs/uu.yaml.example
sudo ./bin/uu --follow-logs
```

Commands:

```sh
sudo ./bin/uu install   # install or reinstall
sudo ./bin/uu start     # start the installed service/monitor only
sudo ./bin/uu stop      # stop the service/process
sudo ./bin/uu status    # print service/process status
sudo ./bin/uu logs      # follow the configured log file
sudo ./bin/uu ns-start  # create the isolated namespace and start the official uuplugin inside it
sudo ./bin/uu ns-stop   # stop the namespace and remove its macvlan link
sudo ./bin/uu ns-status # print the isolated namespace state
sudo ./bin/uu serve     # start the local web control page on 127.0.0.1
```

Running `./bin/uu` without a command is kept as a compatibility shortcut for `./bin/uu install`.

`status` prints the resolved install directory, monitor file/config presence, systemd state on Steam Deck, monitor process state, `uuplugin` process state, and the configured log file path.

## Isolated UU Namespace

For a host that already runs daed, use namespace mode instead of starting the official plugin in the host network stack:

```sh
sudo ./bin/uu ns-start \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1
```

The phone should connect to the namespace IP, for example `192.168.1.250`. The host keeps daed on its normal interfaces, while `uuplugin`, its DNS, routes, and `tun163` live in `uu-ns`. If `--namespace-parent`, `--namespace-address`, or `--namespace-gateway` are omitted, the tool tries to infer them from the host default IPv4 route and uses `.250` on the parent subnet.

`namespace_mode` defaults to `macvlan`. `ipvlan` is also accepted for networks where the NIC or AP does not handle macvlan well.

To use the browser control page:

```sh
sudo ./bin/uu serve
```

Open `http://127.0.0.1:8088/`. The control server refuses non-loopback bind addresses.

The YAML config intentionally uses a flat schema, including `router`, `model`, `install_dir`, `log_dir`, and the `follow_log_*` options below.
Namespace options use the same flat schema: `web_listen`, `namespace_name`, `namespace_parent`, `namespace_link`, `namespace_address`, `namespace_gateway`, `namespace_dns`, and `namespace_mode`.

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
scripts             original shell installer reference
```

## License

MIT
