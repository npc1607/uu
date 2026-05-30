package app

import (
	"strings"
	"testing"
)

func TestSteamDeckSystemdUnitAutostartsAndRestarts(t *testing.T) {
	unit := steamDeckSystemdUnit("/opt/uu/uuplugin_monitor.sh")

	for _, want := range []string{
		"ExecStart=/bin/sh /opt/uu/uuplugin_monitor.sh",
		"Restart=always",
		"RestartSec=5s",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit does not contain %q:\n%s", want, unit)
		}
	}
}

func TestNamespaceSystemdUnitStartsWebAndNamespace(t *testing.T) {
	unit := namespaceSystemdUnit("/opt/uu/uu", []string{
		"ns-serve",
		"--listen", "127.0.0.1:8088",
		"--namespace-address", "192.168.1.250/24",
	}, []string{
		"ns-stop",
		"--namespace-name", "uu-ns",
	})

	for _, want := range []string{
		"ExecStart=/opt/uu/uu ns-serve --listen 127.0.0.1:8088 --namespace-address 192.168.1.250/24",
		"ExecStop=/opt/uu/uu ns-stop --namespace-name uu-ns",
		"Restart=always",
		"WantedBy=multi-user.target",
		"Conflicts=uuplugin.service",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit does not contain %q:\n%s", want, unit)
		}
	}
}

func TestSystemdQuoteArg(t *testing.T) {
	got := systemdExecLine("/opt/uu app/uu", []string{"ns-serve", "--install-dir", `/opt/uu "quoted"`})
	want := `"/opt/uu app/uu" ns-serve --install-dir "/opt/uu \"quoted\""`
	if got != want {
		t.Fatalf("systemdExecLine = %q, want %q", got, want)
	}
}
