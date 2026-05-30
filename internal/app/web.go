package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const controlTokenPlaceholder = "__UU_CONTROL_TOKEN__"

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
	if err := validateLocalListen(i.params.webListen); err != nil {
		fmt.Fprintf(i.stderr, "serve failed: %v\n", err)
		return 1
	}
	token, err := newControlToken()
	if err != nil {
		fmt.Fprintf(i.stderr, "serve failed: %v\n", err)
		return 1
	}

	server := &http.Server{
		Addr:              i.params.webListen,
		Handler:           newControlHandler(i.controlActions(), token),
		ReadHeaderTimeout: 5 * time.Second,
	}
	fmt.Fprintf(i.stdout, "UU namespace control: http://%s/\n", i.params.webListen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(i.stderr, "serve failed: %v\n", err)
		return 1
	}
	return 0
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
	page := strings.Replace(controlPageHTML, controlTokenPlaceholder, token, 1)

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

const controlPageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>UU Namespace Control</title>
  <style>
    :root { color-scheme: light; font-family: Inter, ui-sans-serif, system-ui, sans-serif; }
    * { box-sizing: border-box; }
    body { margin: 0; background: #f3f5f7; color: #1b2530; }
    header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 24px; background: #17212b; color: #fff; }
    h1 { margin: 0; font-size: 18px; font-weight: 650; letter-spacing: 0; }
    main { max-width: 920px; margin: 0 auto; padding: 28px 20px; }
    section { background: #fff; border: 1px solid #d9e0e6; border-radius: 8px; overflow: hidden; }
    .summary { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 18px 20px; border-bottom: 1px solid #e2e7eb; }
    .label { color: #607080; font-size: 12px; font-weight: 700; text-transform: uppercase; }
    .state { margin-top: 4px; font-size: 22px; font-weight: 700; }
    .badge { display: inline-flex; align-items: center; min-width: 76px; justify-content: center; border-radius: 999px; padding: 6px 10px; background: #edf1f4; color: #596a78; font-size: 12px; font-weight: 700; }
    .badge.running { background: #d9f4df; color: #176b30; }
    .badge.stopped { background: #fbe4e4; color: #a32c2c; }
    dl { display: grid; grid-template-columns: minmax(150px, 220px) minmax(0, 1fr); margin: 0; padding: 10px 20px; }
    dt, dd { margin: 0; padding: 10px 0; border-bottom: 1px solid #edf0f2; font-size: 14px; overflow-wrap: anywhere; }
    dt { color: #607080; }
    dd { color: #1f2d38; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    .actions { display: flex; flex-wrap: wrap; gap: 10px; padding: 18px 20px; background: #f9fafb; }
    button { min-height: 38px; border: 1px solid #cbd5dc; border-radius: 6px; padding: 0 14px; background: #fff; color: #25333e; cursor: pointer; font-size: 14px; font-weight: 650; }
    button.primary { border-color: #24713c; background: #24713c; color: #fff; }
    button.danger { border-color: #c03939; color: #a32c2c; }
    button:disabled { cursor: wait; opacity: .58; }
    .error { margin: 16px 20px 20px; border-left: 3px solid #bd3030; padding: 9px 12px; background: #fff2f2; color: #8f2222; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; }
    [hidden] { display: none; }
    @media (max-width: 560px) {
      header { padding: 16px; }
      main { padding: 16px; }
      .summary { padding: 16px; }
      dl { grid-template-columns: 1fr; padding: 8px 16px; }
      dt { border-bottom: 0; padding-bottom: 2px; }
      dd { padding-top: 0; }
      .actions { padding: 16px; }
      button { flex: 1 1 calc(50% - 5px); }
    }
  </style>
</head>
<body>
  <header>
    <h1>UU Namespace Control</h1>
    <button type="button" data-action="refresh">Refresh</button>
  </header>
  <main>
    <section>
      <div class="summary">
        <div>
          <div class="label">Namespace</div>
          <div class="state" id="namespace-name">uu-ns</div>
        </div>
        <div class="badge" id="status-badge">Loading</div>
      </div>
      <dl>
        <dt>Parent interface</dt><dd id="parent">-</dd>
        <dt>Phone target IP</dt><dd id="address">-</dd>
        <dt>Gateway</dt><dd id="gateway">-</dd>
        <dt>DNS</dt><dd id="dns">-</dd>
        <dt>Link mode</dt><dd id="mode">-</dd>
        <dt>UU plugin PID</dt><dd id="plugin-pids">-</dd>
        <dt>Monitor PID</dt><dd id="monitor-pids">-</dd>
        <dt>Log file</dt><dd id="log-file">-</dd>
      </dl>
      <div class="actions">
        <button type="button" class="primary" data-action="start">Start</button>
        <button type="button" class="danger" data-action="stop">Stop</button>
        <button type="button" data-action="restart">Restart</button>
      </div>
      <pre class="error" id="error" hidden></pre>
    </section>
  </main>
  <script>
    const token = "` + controlTokenPlaceholder + `";
    const buttons = Array.from(document.querySelectorAll("button"));
    const errorBox = document.getElementById("error");
    const setText = (id, value) => {
      document.getElementById(id).textContent = value || "-";
    };
    function render(state) {
      const running = Boolean(state.exists && state.plugin_pids && state.plugin_pids.length);
      setText("namespace-name", state.name);
      setText("parent", state.parent);
      setText("address", state.address ? state.address.split("/")[0] : "");
      setText("gateway", state.gateway);
      setText("dns", state.dns ? state.dns.join(", ") : "");
      setText("mode", state.mode);
      setText("plugin-pids", state.plugin_pids ? state.plugin_pids.join(", ") : "");
      setText("monitor-pids", state.monitor_pids ? state.monitor_pids.join(", ") : "");
      setText("log-file", state.log_file);
      const badge = document.getElementById("status-badge");
      badge.textContent = running ? "Running" : "Stopped";
      badge.className = "badge " + (running ? "running" : "stopped");
    }
    function setBusy(value) {
      buttons.forEach((button) => { button.disabled = value; });
    }
    async function request(path, method = "GET") {
      setBusy(true);
      errorBox.hidden = true;
      try {
        const options = { method, headers: {} };
        if (method === "POST") options.headers["X-UU-Control-Token"] = token;
        const response = await fetch(path, options);
        const payload = await response.json();
        if (!response.ok) throw new Error(payload.error || "Request failed");
        render(payload.state);
      } catch (error) {
        errorBox.textContent = error.message;
        errorBox.hidden = false;
      } finally {
        setBusy(false);
      }
    }
    document.addEventListener("click", (event) => {
      const action = event.target.dataset.action;
      if (!action) return;
      request(action === "refresh" ? "/api/status" : "/api/" + action, action === "refresh" ? "GET" : "POST");
    });
    request("/api/status");
  </script>
</body>
</html>
`
