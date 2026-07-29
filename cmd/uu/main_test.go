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
	command, _, err := parseCommandOptions([]string{"status"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "status" {
		t.Fatalf("command = %q, want status", command)
	}
}

func TestParseCommandOptionsUninstall(t *testing.T) {
	command, _, err := parseCommandOptions([]string{"uninstall"})
	if err != nil {
		t.Fatal(err)
	}
	if command != "uninstall" {
		t.Fatalf("command = %q, want uninstall", command)
	}
}

func TestParseCommandOptionsRejectsRemovedNamespaceCommand(t *testing.T) {
	if _, _, err := parseCommandOptions([]string{"ns-start"}); err == nil {
		t.Fatal("parseCommandOptions accepted removed ns-start command")
	}
}

func TestParseCommandOptionsRejectsRemovedLogsCommand(t *testing.T) {
	if _, _, err := parseCommandOptions([]string{"logs"}); err == nil {
		t.Fatal("parseCommandOptions accepted removed logs command")
	}
}
