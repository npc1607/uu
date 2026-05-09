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
	command, opts, err := parseCommandOptions([]string{"logs", "--follow-log-timeout", "30s"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "logs" {
		t.Fatalf("command = %q, want logs", command)
	}
	if opts.FollowLogTimeout.String() != "30s" {
		t.Fatalf("timeout = %s, want 30s", opts.FollowLogTimeout)
	}
}
