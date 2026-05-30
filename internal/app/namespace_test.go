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

func TestSummarizeNamespaceClients(t *testing.T) {
	cfg := namespaceConfig{
		Address: "192.168.1.250/24",
		Gateway: "192.168.1.1",
	}
	sockets := []namespaceSocket{
		{Protocol: "tcp", PeerAddress: "192.168.1.8", LANPeer: true},
		{Protocol: "udp", PeerAddress: "192.168.1.8", LANPeer: true},
		{Protocol: "tcp", PeerAddress: "203.0.113.8"},
	}
	neighbors := []namespaceNeighbor{
		{Address: "192.168.1.1", MAC: "00:11:22:33:44:55", State: "REACHABLE"},
		{Address: "192.168.1.8", MAC: "aa:bb:cc:dd:ee:ff", State: "STALE"},
	}
	got := summarizeNamespaceClients(cfg, sockets, neighbors)
	want := []namespaceClient{{
		Address:       "192.168.1.8",
		MAC:           "aa:bb:cc:dd:ee:ff",
		NeighborState: "STALE",
		Protocols:     []string{"tcp", "udp"},
		Connections:   2,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summarizeNamespaceClients = %#v, want %#v", got, want)
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

func TestMergeClientsFromLog(t *testing.T) {
	cfg := namespaceConfig{
		Address: "192.168.1.250/24",
		Gateway: "192.168.1.1",
	}
	clients := map[string]*namespaceClient{}
	neighbors := map[string]namespaceNeighbor{
		"192.168.1.100": {
			Address: "192.168.1.100",
			MAC:     "a6:e3:a4:74:f5:16",
			State:   "STALE",
		},
	}
	log := []byte(`2026/05/30 13:10:01 internal tcp server accept new connection sock 9: 192.168.1.100:55000 -> 192.168.1.250:16363
2026/05/30 13:10:02 mainlink connect to proxy 157.148.78.24:10004
2026/05/30 13:10:03 device connect mac aa:bb:cc:dd:ee:ff ip 192.168.1.100
`)

	mergeClientsFromLog(cfg, clients, neighbors, "/tmp/uu/uuplugin.log", log)

	got := clients["192.168.1.100"]
	if got == nil {
		t.Fatal("client 192.168.1.100 was not extracted")
	}
	if got.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("MAC = %q", got.MAC)
	}
	if got.NeighborState != "STALE" {
		t.Fatalf("NeighborState = %q", got.NeighborState)
	}
	if got.Source != "uuplugin.log" {
		t.Fatalf("Source = %q", got.Source)
	}
	if got.Connections != 2 {
		t.Fatalf("Connections = %d, want 2", got.Connections)
	}
	if _, ok := clients["157.148.78.24"]; ok {
		t.Fatal("remote proxy IP should not be extracted as a LAN client")
	}
}
