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

func TestParseCommandOptionsNamespaceInstall(t *testing.T) {
	command, opts, err := parseCommandOptions([]string{"ns-install", "--namespace-parent", "enp34s0"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "ns-install" {
		t.Fatalf("command = %q, want ns-install", command)
	}
	if opts.NamespaceParent != "enp34s0" {
		t.Fatalf("NamespaceParent = %q", opts.NamespaceParent)
	}
}

func TestParseCommandOptionsNamespaceUninstall(t *testing.T) {
	command, opts, err := parseCommandOptions([]string{"ns-uninstall", "--namespace-name", "uu-test", "--namespace-link", "uu-test0"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "ns-uninstall" {
		t.Fatalf("command = %q, want ns-uninstall", command)
	}
	if opts.NamespaceName != "uu-test" {
		t.Fatalf("NamespaceName = %q", opts.NamespaceName)
	}
	if opts.NamespaceLink != "uu-test0" {
		t.Fatalf("NamespaceLink = %q", opts.NamespaceLink)
	}
}
