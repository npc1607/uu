const token = document.querySelector('meta[name="uu-control-token"]').content;
const buttons = Array.from(document.querySelectorAll("button"));
const errorBox = document.getElementById("error");
let requestPending = false;

const setText = (id, value) => {
  document.getElementById(id).textContent = value || "-";
};

const endpoint = (address, port) => {
  if (!address && !port) return "-";
  const host = address && address.includes(":") ? `[${address}]` : (address || "*");
  return `${host}:${port || "*"}`;
};

const renderRows = (id, rows, columns) => {
  const body = document.getElementById(id);
  body.replaceChildren();
  if (!rows.length) {
    const row = document.createElement("tr");
    const cell = document.createElement("td");
    cell.className = "empty";
    cell.colSpan = columns.length;
    cell.textContent = "None";
    row.appendChild(cell);
    body.appendChild(row);
    return;
  }
  rows.forEach((item) => {
    const row = document.createElement("tr");
    columns.forEach((column) => {
      const cell = document.createElement("td");
      cell.textContent = column(item) || "-";
      row.appendChild(cell);
    });
    body.appendChild(row);
  });
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
  setText("install-dir", state.install_dir);
  setText("runtime-dir", state.runtime_dir);
  setText("plugin-log-file", state.plugin_log_file);
  setText("log-file", state.log_file);
  setText("observed-at", state.observed_at ? new Date(state.observed_at).toLocaleTimeString() : "");

  renderRows("clients-body", state.clients || [], [
    (client) => client.address,
    (client) => client.mac,
    (client) => client.last_seen,
    (client) => client.source,
    (client) => client.evidence,
    (client) => String(client.connections || 0),
  ]);
  renderRows("sockets-body", state.sockets || [], [
    (socket) => socket.protocol,
    (socket) => socket.state,
    (socket) => endpoint(socket.local_address, socket.local_port),
    (socket) => endpoint(socket.peer_address, socket.peer_port),
    (socket) => socket.lan_peer ? "yes" : "",
    (socket) => socket.process,
  ]);

  errorBox.textContent = state.observation_error || "";
  errorBox.hidden = !state.observation_error;
  const badge = document.getElementById("status-badge");
  badge.textContent = running ? "Running" : "Stopped";
  badge.className = `badge ${running ? "running" : "stopped"}`;
}

function setBusy(value) {
  buttons.forEach((button) => {
    button.disabled = value;
  });
}

async function request(path, method = "GET") {
  if (requestPending) return;
  requestPending = true;
  const mutating = method === "POST";
  if (mutating) setBusy(true);
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
    requestPending = false;
    if (mutating) setBusy(false);
  }
}

document.addEventListener("click", (event) => {
  const action = event.target.dataset.action;
  if (!action) return;
  request(action === "refresh" ? "/api/status" : `/api/${action}`, action === "refresh" ? "GET" : "POST");
});

request("/api/status");
setInterval(() => request("/api/status"), 3000);
