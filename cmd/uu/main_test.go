package main

import "testing"

func TestParseCommandOptionsDefaultsToInstall(t *testing.T) {
	command, opts, err := parseCommandOptions([]string{"--router", "openwrt"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "install" {
		t.Fatalf("command = %q, want install", command)
	}
	if opts.Router != "openwrt" {
		t.Fatalf("router = %q, want openwrt", opts.Router)
	}
}

func TestParseCommandOptionsSubcommand(t *testing.T) {
	command, opts, err := parseCommandOptions([]string{"ns-start", "--follow-log-timeout", "30s", "--namespace-address", "192.168.1.250/24"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "ns-start" {
		t.Fatalf("command = %q, want ns-start", command)
	}
	if opts.FollowLogTimeout.String() != "30s" {
		t.Fatalf("timeout = %s, want 30s", opts.FollowLogTimeout)
	}
	if opts.NamespaceAddress != "192.168.1.250/24" {
		t.Fatalf("NamespaceAddress = %q", opts.NamespaceAddress)
	}
}
