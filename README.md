# uu

Go rewrite of `uu.sh` for installing and managing the official UU plugin on
supported routers and Steam Deck/Linux hosts.

## Scope

This project intentionally runs `uuplugin` in the host network namespace. The
previous `ns-*` experiment was removed because isolating the plugin also
isolates it from the host-side `dae0`/eBPF interception path that acceleration
depends on. The old commands could create network resources and start a
process, but that did not demonstrate working game acceleration.

The supported workflow now focuses on the parts that have a clear effect:

- install, reinstall, or uninstall the official monitor and plugin;
- preserve the Steam Deck plugin identity across reinstall and cleanup;
- manage the installed service or monitor process;
- inspect process and service state;

The design notes under `docs/` describe the removed namespace experiment and
the additional routing work that would be required before such a mode could be
considered functional.

## Build and test

```sh
make build
make test
make vet
```

The binary is written to `bin/uu`. Release packages contain the same `uu`
binary, so use `./uu` in the extracted release directory.

## Official upstream

The authoritative Steam Deck installer is served directly by UU at:

- installer script: <https://uudeck.com>
- monitor metadata: <https://router.uu.163.com/api/script/monitor?type=steam-deck-plugin>
- uninstall metadata: <https://router.uu.163.com/api/script/uninstall?type=steam-deck-plugin>
- x86_64 plugin metadata: <https://router.uu.163.com/api/plugin?type=steam-deck-plugin-x86_64>

This repository keeps a snapshot of the official installer at
[`scripts/uu.sh`](scripts/uu.sh). To check for an upstream change without
overwriting the local copy:

```sh
curl -fsSL https://uudeck.com -o /tmp/uudeck-install.sh
diff -u scripts/uu.sh /tmp/uudeck-install.sh
sha256sum scripts/uu.sh /tmp/uudeck-install.sh
```

After reviewing the diff, synchronize the snapshot with:

```sh
install -m 0644 /tmp/uudeck-install.sh scripts/uu.sh
sh -n scripts/uu.sh
```

The metadata endpoints return a comma-separated download URL and MD5 checksum.
The dated path in the monitor response is useful for identifying the upstream
release being tested. When the installer or monitor changes, review the Go
implementation and run the complete test suite instead of updating only the
snapshot.

## Commands

Running `uu` without a command is equivalent to `uu install`.

```sh
sudo ./bin/uu install  # install/reinstall, create the service, and start it now
sudo ./bin/uu uninstall # stop and remove the plugin and its service
sudo ./bin/uu start    # start the installed service or monitor
sudo ./bin/uu stop     # stop the service, monitor, and plugin process
sudo ./bin/uu status   # print service and process state
```

For `steam-deck-plugin`, the default install directory is the directory that
contains the `uu` binary. Override it with `--install-dir` or `install_dir` in
the YAML configuration.

The Steam Deck installer writes and enables
`/etc/systemd/system/uuplugin.service`, starts it immediately, and displays the
binding QR code produced by the official plugin. During an upgrade the
installer also stops and removes the obsolete `uuplugin-ns.service` left by
older versions.

After a successful installation, keep the terminal open and scan the displayed
character QR code with the UU host accelerator app. The command exits after
binding finishes. If a QR code was generated but is no longer visible, print
the current one manually with:

```sh
cat ./bin/runtime/.steam_deck_sn_qrcode
```

The exact path is `<install_dir>/runtime/.steam_deck_sn_qrcode` when a custom
installation directory is configured.

## Configuration

All options can be passed as flags or through a flat YAML file:

```sh
sudo ./bin/uu install --config configs/uu.yaml.example
```

Example:

```yaml
router: steam-deck-plugin
model: x86_64
# install_dir: /opt/uu/
# log_dir: /tmp

```

Use `./bin/uu --help` for the complete flag list.

## Supported targets

- `steam-deck-plugin`
- `openwrt`
- `asuswrt-merlin`
- `xiaomi`
- `hiwifi`

The default router is `steam-deck-plugin` and the default model is `x86_64`.

## Troubleshooting

The current Steam Deck plugin requires a working `iptables`/nftables kernel
stack. The installer checks `iptables --version` before changing the existing
installation. If it reports `Protocol not supported` after a kernel package
upgrade, reboot so the running kernel matches the installed kernel modules,
then run `sudo ./bin/uu install` again.

## Internal layout

```text
cmd/uu              CLI entrypoint and flag parsing
internal/app        install/uninstall/start/stop/status orchestration
internal/config     config defaults and YAML loading
internal/downloader remote script download and MD5 verification
internal/plugin     uuplugin process state and stop helpers
internal/router     supported router names and defaults
scripts/uu.sh       original shell installer reference
```

## License

MIT
