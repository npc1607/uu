package app

import (
	"strings"
	"testing"
)

func TestSteamDeckSystemdUnitRunsAndRestarts(t *testing.T) {
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
