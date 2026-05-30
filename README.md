# uu

Go rewrite of `uu.sh`.

## Why This Version

Compared with the original `uu.sh` installer, this Go version keeps the Steam Deck install flow compatible while adding a few practical improvements:

- Persistent Steam Deck identity: `.uuplugin_uuid` and `.uid` are saved in the install directory and restored to the local `runtime/` directory next to the `uu` binary, so reinstalling, upgrading, or cleaning up runtime files does not create a new plugin identity.
- Reusable CLI commands: install, start, stop, status, and logs are available without rerunning the full shell installer flow.
- Isolated namespace mode: the official `uuplugin` can be started inside a dedicated `uu-ns` network namespace with its own macvlan link, DNS, and routing so it does not share the host network stack.
- Local control page: `serve` starts an embedded loopback-only HTML/CSS/JS dashboard with namespace start, stop, restart, status, neighbor cache, and namespace sockets.
- Clearer diagnostics: install logs, resolved paths, systemd state, monitor state, and `uuplugin` process state are easier to inspect.
- Configurable install options: router, model, install directory, log directory, and log-following behavior can be set with flags or YAML.

## Build

```sh
make build
```

Release assets are built by GitHub Actions when a `v*` tag is pushed. The workflow uploads a `linux/amd64` tarball and checksum file to the GitHub Release.

## Usage

Build the binary first when running from source:

```sh
make build
```

Release packages contain the same `uu` binary. After extracting a release tarball, run commands from the extracted directory with `./uu` instead of `./bin/uu`.

The default config matches the original script defaults:

```yaml
router: steam-deck-plugin
model: x86_64
```

For `steam-deck-plugin`, the default install directory is the directory containing the `uu` binary. Use `--install-dir` or `install_dir` in YAML to override it.

### Recommended: Namespace Autostart + Web

Use this mode on a host that already runs daed or other host-network routing. It starts the official `uuplugin` inside an isolated network namespace and also starts the embedded local web control page.

```sh
sudo ./bin/uu ns-install \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1
```

This writes and enables `/etc/systemd/system/uuplugin-ns.service`, then starts it immediately. The service runs `uu ns-serve`, which starts both:

- the isolated namespace, default name `uu-ns`
- the local web control page, default `http://127.0.0.1:8088/`

The phone should connect to the namespace IP, for example `192.168.1.250`. The browser control page is loopback-only and must be opened on the host itself.

If you do not know the values for `--namespace-parent`, `--namespace-address`, or `--namespace-gateway`, see [Namespace Parameters](#namespace-parameters).

Check it:

```sh
systemctl status uuplugin-ns
sudo ./bin/uu ns-status \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1
```

Uninstall namespace autostart and clean generated runtime files:

```sh
sudo ./bin/uu ns-uninstall
```

`ns-uninstall` checks whether `uuplugin-ns` is running before it removes anything. If the service is active, it stops it first, then disables and removes `/etc/systemd/system/uuplugin-ns.service`, removes the namespace/link/DNS files, and deletes the generated monitor/config/runtime files next to the `uu` binary.

### Ordinary Host-Network Install

Use this only if you want the original Steam Deck-style behavior where `uuplugin` runs in the host network namespace.

```sh
sudo ./bin/uu install
```

This writes and enables `/etc/systemd/system/uuplugin.service`, then starts it immediately. Running `./bin/uu` without a command is the same as `./bin/uu install`.

Check it:

```sh
systemctl status uuplugin
sudo ./bin/uu status
```

`install` and `ns-install` are intended to be mutually exclusive. Ordinary `install` removes namespace autostart; `ns-install` disables/stops the ordinary `uuplugin.service`.

### One-Shot Namespace Mode

Use this to test the namespace without installing systemd autostart:

```sh
sudo ./bin/uu ns-start \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1
```

Then inspect or stop it:

```sh
sudo ./bin/uu ns-status \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1

sudo ./bin/uu ns-stop
```

To run namespace mode and the web page in one foreground process without installing systemd:

```sh
sudo ./bin/uu ns-serve \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1
```

### Web Control Page

`serve` starts only the embedded web control page. It can start, stop, restart, and inspect namespace mode through the page actions:

```sh
sudo ./bin/uu serve \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1
```

Open `http://127.0.0.1:8088/`. The control server refuses non-loopback bind addresses.
When a phone connects to the isolated UU IP, the page shows the namespace neighbor cache from `ip neigh` and current TCP/UDP socket state from `ss`. The patched Steam Deck monitor writes the official plugin output to `<install_dir>/runtime/uuplugin.log`.

The web files are real HTML/CSS/JS assets under `internal/app/web/` and are embedded into the `uu` binary at build time.

### Namespace Parameters

Namespace mode creates a second LAN identity for `uuplugin`. The host keeps its normal network address for daed, browsers, Steam, and other processes. `uuplugin` gets a separate address on the same LAN, and the phone connects to that separate address.

Typical home LAN example:

```text
router/gateway: 192.168.1.1
host address:   192.168.1.23/24 on enp34s0
uu address:     192.168.1.250/24 on uu-macvlan0 inside uu-ns
phone target:   192.168.1.250
```

`--namespace-parent`

This is the host network interface that is connected to your LAN. The namespace creates a `macvlan` or `ipvlan` child from this interface.

Find it from the default route:

```sh
ip -4 route show default
```

Example output:

```text
default via 192.168.1.1 dev enp34s0 proto dhcp metric 100
```

Use the value after `dev`:

```sh
--namespace-parent enp34s0
```

For Wi-Fi it may look like `wlan0` or `wlp2s0`; for Ethernet it often looks like `eth0`, `enp34s0`, or `eno1`.

`--namespace-gateway`

This is your LAN router IP. It is usually the `via` value from the default route:

```text
default via 192.168.1.1 dev enp34s0
```

Use:

```sh
--namespace-gateway 192.168.1.1
```

If your default route has no `via`, inspect the interface address and your router/DHCP settings:

```sh
ip -4 addr show dev enp34s0
ip route
```

`--namespace-address`

This is the extra LAN IP assigned to `uuplugin`, in CIDR form. It must be on the same subnet as the host and gateway, and it must not already be used by another device.

First check your host interface:

```sh
ip -4 addr show dev enp34s0
```

Example output includes:

```text
inet 192.168.1.23/24
```

That means the LAN subnet is `192.168.1.0/24`, so a reasonable namespace address is:

```sh
--namespace-address 192.168.1.250/24
```

Before using an address, check that it is not already in use:

```sh
ping -c 1 192.168.1.250
ip neigh show 192.168.1.250
```

No ping reply and no neighbor entry usually means it is free. For a more reliable setup, reserve that IP in your router DHCP settings or pick an address outside the DHCP pool but still inside the subnet.

`--namespace-name`

This is the Linux network namespace name. Default:

```sh
--namespace-name uu-ns
```

You normally do not need to change it. It appears in commands like:

```sh
ip netns list
ip netns exec uu-ns ss -tunap
```

`--namespace-link`

This is the interface name created for the namespace. Default:

```sh
--namespace-link uu-macvlan0
```

You normally do not need to change it unless another interface already uses that name. Linux interface names must be 15 characters or shorter.

`--namespace-dns`

These DNS servers are written to `/etc/netns/<namespace-name>/resolv.conf` for processes inside the namespace. Default:

```sh
--namespace-dns 223.5.5.5,119.29.29.29
```

You can use your router or public DNS:

```sh
--namespace-dns 192.168.1.1,223.5.5.5
```

`--namespace-mode`

This controls the link type used for the namespace. Default:

```sh
--namespace-mode macvlan
```

Use `macvlan` first. It gives the namespace its own LAN-facing identity. Some Wi-Fi drivers, switches, or AP isolation settings do not handle macvlan well; if the phone cannot reach the namespace IP, try:

```sh
--namespace-mode ipvlan
```

Quick command to derive the common values:

```sh
ip -4 route show default
ip -4 addr show dev enp34s0
```

Then run:

```sh
sudo ./bin/uu ns-install \
  --namespace-parent enp34s0 \
  --namespace-address 192.168.1.250/24 \
  --namespace-gateway 192.168.1.1
```

After it starts, the phone should use `192.168.1.250`, not the host's normal IP.

### Commands

```sh
sudo ./bin/uu install     # ordinary host-network install and autostart
sudo ./bin/uu start       # start ordinary installed service/monitor
sudo ./bin/uu stop        # stop ordinary service/process
sudo ./bin/uu status      # print ordinary service/process status
sudo ./bin/uu logs        # follow the configured log file

sudo ./bin/uu ns-install  # install namespace + web autostart
sudo ./bin/uu ns-uninstall # uninstall namespace autostart and generated files
sudo ./bin/uu ns-start    # start namespace once
sudo ./bin/uu ns-stop     # stop namespace and remove its link
sudo ./bin/uu ns-status   # print namespace state
sudo ./bin/uu ns-serve    # foreground namespace + web process
sudo ./bin/uu serve       # foreground web control page only
```

### Config

All options can be passed as flags or through a flat YAML file:

```sh
sudo ./bin/uu ns-install --config configs/uu.yaml.example
```

Useful YAML keys:

```yaml
router: steam-deck-plugin
model: x86_64
install_dir: /home/pcong/project/uu/bin/
log_dir: /tmp
web_listen: 127.0.0.1:8088

namespace_name: uu-ns
namespace_parent: enp34s0
namespace_link: uu-macvlan0
namespace_address: 192.168.1.250/24
namespace_gateway: 192.168.1.1
namespace_dns: 223.5.5.5,119.29.29.29
namespace_mode: macvlan
```

If `--namespace-parent`, `--namespace-address`, or `--namespace-gateway` are omitted, the tool tries to infer them from the host default IPv4 route and uses `.250` on the parent subnet. Explicit values are recommended for systemd autostart so boot behavior is predictable.

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
internal/app/web    embedded web control assets
internal/config     config defaults and YAML loading
internal/downloader remote script download and MD5 verification
internal/logtail    terminal log following
internal/plugin     uuplugin process state and stop helpers
internal/router     supported router names and defaults
scripts             original shell installer reference
```

## License

MIT
