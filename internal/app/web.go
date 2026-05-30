package app

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const controlTokenPlaceholder = "__UU_CONTROL_TOKEN__"

//go:embed web/index.html web/style.css web/app.js
var controlAssets embed.FS

type controlActions struct {
	status  func() (namespaceState, error)
	start   func() error
	stop    func() error
	restart func() error
}

type controlResponse struct {
	OK    bool           `json:"ok"`
	State namespaceState `json:"state"`
	Error string         `json:"error,omitempty"`
}

func (i *App) Serve() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "serve failed: %v\n", err)
		return 1
	}
	return i.serveControlPage("serve")
}

func (i *App) NamespaceServe() int {
	i.openLog(false)
	defer i.closeLog()
	if err := i.initParams(); err != nil {
		fmt.Fprintf(i.stderr, "namespace serve failed: %v\n", err)
		return 1
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(i.stderr, "namespace serve failed: root privileges required for namespace mode; rerun with sudo")
		return 1
	}

	i.mu.Lock()
	err := i.namespaceStartLocked()
	i.mu.Unlock()
	if err != nil {
		fmt.Fprintf(i.stderr, "namespace serve failed: %v\n", err)
		return 1
	}
	defer i.stopNamespaceAfterServe()

	return i.serveControlPage("namespace serve")
}

func (i *App) serveControlPage(label string) int {
	if err := validateLocalListen(i.params.webListen); err != nil {
		fmt.Fprintf(i.stderr, "%s failed: %v\n", label, err)
		return 1
	}
	token, err := newControlToken()
	if err != nil {
		fmt.Fprintf(i.stderr, "%s failed: %v\n", label, err)
		return 1
	}

	server := &http.Server{
		Addr:              i.params.webListen,
		Handler:           newControlHandler(i.controlActions(), token),
		ReadHeaderTimeout: 5 * time.Second,
	}
	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopSignals)
	go func() {
		<-stopSignals
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	fmt.Fprintf(i.stdout, "UU namespace control: http://%s/\n", i.params.webListen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(i.stderr, "%s failed: %v\n", label, err)
		return 1
	}
	return 0
}

func (i *App) stopNamespaceAfterServe() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.namespaceStopLocked(); err != nil {
		i.logf("namespace stop after serve failed: %v", err)
	}
}

func (i *App) controlActions() controlActions {
	return controlActions{
		status: func() (namespaceState, error) {
			i.mu.Lock()
			defer i.mu.Unlock()
			return i.namespaceStateLocked()
		},
		start: func() error {
			i.mu.Lock()
			defer i.mu.Unlock()
			return i.namespaceStartLocked()
		},
		stop: func() error {
			i.mu.Lock()
			defer i.mu.Unlock()
			return i.namespaceStopLocked()
		},
		restart: func() error {
			i.mu.Lock()
			defer i.mu.Unlock()
			if err := i.namespaceStopLocked(); err != nil {
				return err
			}
			return i.namespaceStartLocked()
		},
	}
}

func newControlHandler(actions controlActions, token string) http.Handler {
	mux := http.NewServeMux()
	page := controlIndexPage(token)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
	mux.HandleFunc("/assets/style.css", controlAssetHandler("style.css", "text/css; charset=utf-8"))
	mux.HandleFunc("/assets/app.js", controlAssetHandler("app.js", "application/javascript; charset=utf-8"))
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeControlJSON(w, http.StatusMethodNotAllowed, controlResponse{Error: "method not allowed"})
			return
		}
		state, err := actions.status()
		if err != nil {
			state.Error = err.Error()
			writeControlJSON(w, http.StatusInternalServerError, controlResponse{State: state, Error: err.Error()})
			return
		}
		writeControlJSON(w, http.StatusOK, controlResponse{OK: true, State: state})
	})
	mux.HandleFunc("/api/start", controlActionHandler(token, actions.start, actions.status))
	mux.HandleFunc("/api/stop", controlActionHandler(token, actions.stop, actions.status))
	mux.HandleFunc("/api/restart", controlActionHandler(token, actions.restart, actions.status))
	return mux
}

func controlIndexPage(token string) string {
	content, err := controlAssets.ReadFile("web/index.html")
	if err != nil {
		panic(err)
	}
	return strings.Replace(string(content), controlTokenPlaceholder, token, 1)
}

func controlAssetHandler(name string, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content, err := controlAssets.ReadFile("web/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(content)
	}
}

func controlActionHandler(token string, action func() error, status func() (namespaceState, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeControlJSON(w, http.StatusMethodNotAllowed, controlResponse{Error: "method not allowed"})
			return
		}
		if token == "" || r.Header.Get("X-UU-Control-Token") != token {
			writeControlJSON(w, http.StatusForbidden, controlResponse{Error: "invalid control token"})
			return
		}
		if err := action(); err != nil {
			writeControlJSON(w, http.StatusInternalServerError, controlResponse{Error: err.Error()})
			return
		}
		state, err := status()
		if err != nil {
			state.Error = err.Error()
			writeControlJSON(w, http.StatusInternalServerError, controlResponse{State: state, Error: err.Error()})
			return
		}
		writeControlJSON(w, http.StatusOK, controlResponse{OK: true, State: state})
	}
}

func writeControlJSON(w http.ResponseWriter, status int, response controlResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

func validateLocalListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("web control must listen on a loopback address, got %q", addr)
	}
	return nil
}

func newControlToken() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
