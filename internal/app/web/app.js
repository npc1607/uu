const token = document.querySelector('meta[name="uu-control-token"]').content;
const buttons = Array.from(document.querySelectorAll("button"));
const errorBox = document.getElementById("error");
const canvas = document.getElementById("mesh-canvas");
const ctx = canvas.getContext("2d", { alpha: true });
let requestPending = false;

const setText = (id, value) => {
  const node = document.getElementById(id);
  if (!node) return;
  node.textContent = value || "-";
};

const endpoint = (address, port) => {
  if (!address && !port) return "-";
  const host = address && address.includes(":") ? `[${address}]` : (address || "*");
  return `${host}:${port || "*"}`;
};

const pluginHealth = (state) => {
  const running = Boolean(state.exists && state.plugin_pids && state.plugin_pids.length);
  if (!running) {
    return { label: "Stopped", className: "stopped", message: "Namespace is idle" };
  }
  if (state.observation_error) {
    return { label: "Degraded", className: "degraded", message: "Status inspection failed" };
  }

  const phoneIP = state.address ? state.address.split("/")[0] : "";
  const sockets = state.sockets || [];
  const neighbors = state.neighbors || [];
  const clientNeighbors = neighbors.filter((neighbor) => {
    return neighbor.address && neighbor.address !== state.gateway && neighbor.address !== phoneIP;
  });
  const hasLANPeer = sockets.some((socket) => socket.lan_peer);
  const hasClient = hasLANPeer || clientNeighbors.length > 0;
  if (hasClient) {
    return { label: "Active", className: "running", message: "Phone client detected" };
  }

  const hasUUListener = sockets.some((socket) => {
    const port = String(socket.local_port || "");
    return (port === "16363" || port === "14554") && (socket.state === "LISTEN" || socket.state === "UNCONN");
  });
  const hasUpstream = sockets.some((socket) => {
    return socket.state === "ESTAB" && socket.local_address === phoneIP && socket.peer_address !== state.gateway;
  });
  if (hasUUListener && hasUpstream) {
    return { label: "No Phone", className: "waiting", message: "Plugin online, no phone client seen" };
  }
  if (hasUUListener) {
    return { label: "Ready", className: "waiting", message: "Plugin listening, waiting for phone" };
  }
  return { label: "Degraded", className: "degraded", message: "Plugin process has no UU listener" };
};

let fitFrame = 0;

const fitValue = (node) => {
  node.classList.remove("fit-wrap");
  node.style.removeProperty("--fit-font-size");

  if (!node.textContent || node.textContent === "-") return;

  const width = node.clientWidth;
  if (width <= 0) return;

  const computed = window.getComputedStyle(node);
  const maxFontSize = parseFloat(computed.fontSize) || 24;
  const minFontSize = Number(node.dataset.minFontSize || 11);
  const fits = () => node.scrollWidth <= node.clientWidth + 1;

  if (fits()) return;

  let low = minFontSize;
  let high = maxFontSize;
  let best = minFontSize;

  for (let step = 0; step < 9; step++) {
    const mid = (low + high) / 2;
    node.style.setProperty("--fit-font-size", `${mid}px`);
    if (fits()) {
      best = mid;
      low = mid;
    } else {
      high = mid;
    }
  }

  node.style.setProperty("--fit-font-size", `${best.toFixed(2)}px`);
  if (!fits()) node.classList.add("fit-wrap");
};

const fitValues = () => {
  cancelAnimationFrame(fitFrame);
  fitFrame = requestAnimationFrame(() => {
    document.querySelectorAll(".fit-value").forEach(fitValue);
  });
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

function drawMesh() {
  if (!ctx) return;
  const dpr = window.devicePixelRatio || 1;
  const width = window.innerWidth;
  const height = window.innerHeight;
  canvas.width = Math.floor(width * dpr);
  canvas.height = Math.floor(height * dpr);
  canvas.style.width = `${width}px`;
  canvas.style.height = `${height}px`;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);

  const nodes = [];
  const count = Math.max(12, Math.min(24, Math.floor(width / 70)));
  for (let idx = 0; idx < count; idx++) {
    nodes.push({
      x: (idx + 1) * width / (count + 1),
      y: height * (0.18 + (idx % 5) * 0.13),
      r: 1.5 + (idx % 3) * 0.55,
      vx: (idx % 2 ? 0.12 : -0.12),
      vy: (idx % 3 ? 0.08 : -0.08),
    });
  }

  const tick = () => {
    ctx.clearRect(0, 0, width, height);
    ctx.strokeStyle = "rgba(84, 105, 255, .14)";
    ctx.lineWidth = 1;
    for (let left = 0; left < nodes.length; left++) {
      const a = nodes[left];
      a.x += a.vx;
      a.y += a.vy;
      if (a.x < 0 || a.x > width) a.vx *= -1;
      if (a.y < 0 || a.y > height) a.vy *= -1;
      for (let right = left + 1; right < nodes.length; right++) {
        const b = nodes[right];
        const distance = Math.hypot(a.x - b.x, a.y - b.y);
        if (distance < 180) {
          ctx.globalAlpha = (1 - distance / 180) * 0.9;
          ctx.beginPath();
          ctx.moveTo(a.x, a.y);
          ctx.lineTo(b.x, b.y);
          ctx.stroke();
        }
      }
    }
    ctx.globalAlpha = 1;
    for (const node of nodes) {
      const glow = ctx.createRadialGradient(node.x, node.y, 0, node.x, node.y, 18);
      glow.addColorStop(0, "rgba(37, 99, 235, .35)");
      glow.addColorStop(0.45, "rgba(124, 58, 237, .18)");
      glow.addColorStop(1, "rgba(255, 255, 255, 0)");
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(node.x, node.y, 18, 0, Math.PI * 2);
      ctx.fill();

      ctx.fillStyle = "rgba(37, 99, 235, .78)";
      ctx.beginPath();
      ctx.arc(node.x, node.y, node.r, 0, Math.PI * 2);
      ctx.fill();
    }
    requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);
}

function render(state) {
  const health = pluginHealth(state);
  setText("namespace-name", state.name);
  setText("parent", state.parent);
  setText("address", state.address ? state.address.split("/")[0] : "");
  setText("link", state.link);
  setText("gateway", state.gateway);
  setText("dns", state.dns ? state.dns.join(", ") : "");
  setText("web-listen", state.web_listen);
  setText("mode", state.mode);
  setText("namespace-pids", state.namespace_pids ? state.namespace_pids.join(", ") : "");
  setText("plugin-pids", state.plugin_pids ? state.plugin_pids.join(", ") : "");
  setText("monitor-pids", state.monitor_pids ? state.monitor_pids.join(", ") : "");
  setText("install-dir", state.install_dir);
  setText("runtime-dir", state.runtime_dir);
  setText("plugin-log-file", state.plugin_log_file);
  setText("monitor-config", state.monitor_config);
  setText("log-file", state.log_file);
  setText("observed-at", state.observed_at ? new Date(state.observed_at).toLocaleString() : "-");
  setText("running-signal", health.message);
  setText("neighbors-count", String((state.neighbors || []).length));
  setText("sockets-count", String((state.sockets || []).length));

  renderRows("neighbors-body", state.neighbors || [], [
    (neighbor) => neighbor.address,
    (neighbor) => neighbor.mac,
    (neighbor) => neighbor.state,
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
  badge.textContent = health.label;
  badge.className = `badge ${health.className}`;
  fitValues();
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
  const action = event.target.closest("button")?.dataset.action;
  if (!action) return;
  request(action === "refresh" ? "/api/status" : `/api/${action}`, action === "refresh" ? "GET" : "POST");
});

window.addEventListener("resize", () => {
  drawMesh();
  fitValues();
});
drawMesh();
request("/api/status");
setInterval(() => request("/api/status"), 3000);
