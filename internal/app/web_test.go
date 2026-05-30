package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateLocalListen(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8088", "[::1]:8088", "localhost:8088"} {
		if err := validateLocalListen(addr); err != nil {
			t.Fatalf("validateLocalListen(%q) = %v", addr, err)
		}
	}
	if err := validateLocalListen("0.0.0.0:8088"); err == nil {
		t.Fatal("validateLocalListen accepted a non-loopback address")
	}
}

func TestControlHandlerStatus(t *testing.T) {
	token := "test-token"
	handler := newControlHandler(controlActions{
		status: func() (namespaceState, error) {
			return namespaceState{
				Name:        "uu-ns",
				Exists:      true,
				Parent:      "enp34s0",
				Address:     "192.168.1.250/24",
				Gateway:     "192.168.1.1",
				DNS:         []string{"223.5.5.5"},
				Mode:        "macvlan",
				PluginPIDs:  []int{1234},
				MonitorPIDs: []int{1235},
				LogFile:     "/tmp/monitor.log",
			}, nil
		},
		start:   func() error { return nil },
		stop:    func() error { return nil },
		restart: func() error { return nil },
	}, token)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rr.Code)
	}
	var payload controlResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.State.Name != "uu-ns" || len(payload.State.PluginPIDs) != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestControlHandlerStartRequiresToken(t *testing.T) {
	handler := newControlHandler(controlActions{
		status:  func() (namespaceState, error) { return namespaceState{Name: "uu-ns"}, nil },
		start:   func() error { return nil },
		stop:    func() error { return nil },
		restart: func() error { return nil },
	}, "test-token")

	req := httptest.NewRequest(http.MethodPost, "/api/start", strings.NewReader(""))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status code = %d, want 403", rr.Code)
	}
}
