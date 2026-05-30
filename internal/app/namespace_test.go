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

func TestParseNamespaceSockets(t *testing.T) {
	output := `tcp LISTEN 0 128 0.0.0.0:16363 0.0.0.0:* users:(("uuplugin",pid=100,fd=7))
tcp ESTAB 0 0 192.168.1.250:16363 192.168.1.8:53210 users:(("uuplugin",pid=100,fd=8))
udp UNCONN 0 0 0.0.0.0:5353 0.0.0.0:* users:(("uuplugin",pid=100,fd=9))
`
	got := parseNamespaceSockets(output)
	if len(got) != 3 {
		t.Fatalf("parseNamespaceSockets length = %d, want 3", len(got))
	}

	var established namespaceSocket
	for _, socket := range got {
		if socket.State == "ESTAB" {
			established = socket
			break
		}
	}
	if established.LocalAddress != "192.168.1.250" || established.LocalPort != "16363" {
		t.Fatalf("local endpoint = %s:%s", established.LocalAddress, established.LocalPort)
	}
	if established.PeerAddress != "192.168.1.8" || established.PeerPort != "53210" {
		t.Fatalf("peer endpoint = %s:%s", established.PeerAddress, established.PeerPort)
	}
}

func TestParseNamespaceNeighbors(t *testing.T) {
	output := `192.168.1.1 lladdr 00:11:22:33:44:55 REACHABLE
192.168.1.8 lladdr aa:bb:cc:dd:ee:ff STALE
192.168.1.9 FAILED
`
	got := parseNamespaceNeighbors(output)
	want := []namespaceNeighbor{
		{Address: "192.168.1.1", MAC: "00:11:22:33:44:55", State: "REACHABLE"},
		{Address: "192.168.1.8", MAC: "aa:bb:cc:dd:ee:ff", State: "STALE"},
		{Address: "192.168.1.9", State: "FAILED"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNamespaceNeighbors = %#v, want %#v", got, want)
	}
}
