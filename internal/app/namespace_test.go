package app

import (
	"reflect"
	"testing"
)

func TestParseNamespaceDNS(t *testing.T) {
	got, err := parseNamespaceDNS("223.5.5.5, 119.29.29.29 8.8.8.8:53")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"223.5.5.5", "119.29.29.29", "8.8.8.8"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNamespaceDNS = %#v, want %#v", got, want)
	}
}

func TestValidateNamespaceConfig(t *testing.T) {
	cfg := namespaceConfig{
		Name:    "uu-ns",
		Parent:  "enp34s0",
		Link:    "uu-macvlan0",
		Address: "192.168.1.250/24",
		Gateway: "192.168.1.1",
		DNS:     []string{"223.5.5.5"},
		Mode:    "macvlan",
	}
	if err := validateNamespaceConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Address = "fd00::2/64"
	if err := validateNamespaceConfig(cfg); err == nil {
		t.Fatal("validateNamespaceConfig accepted an ipv6 namespace address")
	}
}
