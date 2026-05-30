const token = document.querySelector('meta[name="uu-control-token"]').content;
const buttons = Array.from(document.querySelectorAll("button"));
const errorBox = document.getElementById("error");
const canvas = document.getElementById("mesh-canvas");
const ctx = canvas.getContext("2d", { alpha: true });
let requestPending = false;
const latencyHistory = [];
const maxLatencySamples = 28;

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

const parseIPv4 = (value) => {
  if (!value) return null;
  const parts = value.trim().split(".");
  if (parts.length !== 4) return null;
  const octets = [];
  for (const part of parts) {
    if (part === "" || !/^\d+$/.test(part)) return null;
    const octet = Number(part);
    if (!Number.isInteger(octet) || octet < 0 || octet > 255) return null;
    octets.push(octet);
  }
  return (((octets[0] * 256 + octets[1]) * 256 + octets[2]) * 256 + octets[3]) >>> 0;
};

const parseCidr = (value) => {
  if (!value) return null;
  const parts = value.trim().split("/");
  const address = parseIPv4(parts[0]);
  if (address === null) return null;
  const prefix = parts.length > 1 ? Number(parts[1]) : 32;
  if (!Number.isInteger(prefix) || prefix < 0 || prefix > 32) return null;
  const mask = prefix === 0 ? 0 : (0xffffffff << (32 - prefix)) >>> 0;
  return { address, mask };
};

const isSameSubnet = (address, cidr) => {
  const ip = parseIPv4(address);
  const network = parseCidr(cidr);
  if (ip === null || network === null) return false;
  return (ip & network.mask) === (network.address & network.mask);
};

const externalSockets = (state) => {
  const phoneIP = state.address ? state.address.split("/")[0] : "";
  return (state.sockets || []).filter((socket) => {
    if (!socket.peer_address || socket.peer_address === "0.0.0.0" || socket.peer_address === "127.0.0.1") {
      return false;
    }
    if (socket.peer_address === state.gateway || socket.peer_address === phoneIP) {
      return false;
    }
    return !isSameSubnet(socket.peer_address, state.address);
  });
};

const activeAccelerationSockets = (state) => {
  return externalSockets(state).filter((socket) => {
    const peerPort = String(socket.peer_port || "");
    return socket.state === "ESTAB" && peerPort !== "16000";
  });
};

const parseLatency = (value) => {
  if (!value) return null;
  const latency = Number.parseFloat(String(value).replace("ms", "").trim());
  return Number.isFinite(latency) ? latency : null;
};

const formatLatency = (latency) => {
  if (!Number.isFinite(latency)) return "";
  return `${latency < 10 ? latency.toFixed(1) : Math.round(latency)} ms`;
};

const latencyClass = (latency) => {
  if (!Number.isFinite(latency)) return "latency-unknown";
  if (latency <= 45) return "latency-good";
  if (latency <= 90) return "latency-fair";
  return "latency-poor";
};

const tag = (text, className = "") => {
  const node = document.createElement("span");
  node.className = `tag ${className}`.trim();
  node.textContent = text || "-";
  return node;
};

const stateTag = (state) => {
  const value = state || "-";
  const normalized = value.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  return tag(value, `state-tag state-${normalized}`);
};

const protocolTag = (protocol) => tag(protocol || "-", "protocol-tag");

const latencyTag = (rtt) => {
  const latency = parseLatency(rtt);
  return latencyTagValue(latency);
};

const latencyTagValue = (latency) => {
  return tag(formatLatency(latency) || "No RTT", `latency-tag ${latencyClass(latency)}`);
};

const peerTag = (value) => {
  return value ? tag("LAN", "peer-tag") : tag("WAN", "peer-tag peer-wan");
};

const pluginHealth = (state) => {
  const running = Boolean(state.exists && state.plugin_pids && state.plugin_pids.length);
  if (!running) {
    return { label: "Stopped", className: "stopped", message: "Namespace is idle" };
  }
  if (state.observation_error) {
    return { label: "Degraded", className: "degraded", message: "Status inspection failed" };
  }

  const sockets = state.sockets || [];
  const hasUUListener = sockets.some((socket) => {
    const port = String(socket.local_port || "");
    return (port === "16363" || port === "14554") && (socket.state === "LISTEN" || socket.state === "UNCONN");
  });

  const activePeers = activeAccelerationSockets(state);
  if (activePeers.length > 0) {
    return {
      label: "Active",
      className: "running",
      message: `Acceleration active (${activePeers.length} external connection${activePeers.length === 1 ? "" : "s"})`,
    };
  }

  const coolingPeers = externalSockets(state).filter((socket) => {
    const peerPort = String(socket.peer_port || "");
    return socket.state === "TIME-WAIT" && peerPort !== "16000";
  });
  if (coolingPeers.length > 0) {
    return {
      label: "Cooling",
      className: "waiting",
      message: `Acceleration recently disconnected (${coolingPeers.length} socket${coolingPeers.length === 1 ? "" : "s"} cooling down)`,
    };
  }

  const controlPeers = externalSockets(state).filter((socket) => {
    return socket.state === "ESTAB" && String(socket.peer_port || "") === "16000";
  });
  if (hasUUListener || controlPeers.length > 0) {
    return {
      label: "Ready",
      className: "waiting",
      message: controlPeers.length > 0 ? "Plugin online, waiting for acceleration traffic" : "Plugin listening, waiting for phone",
    };
  }

  return { label: "Degraded", className: "degraded", message: "Plugin process has no UU listener" };
};

const activeLatencies = (state) => {
  return activeAccelerationSockets(state)
    .map((socket) => parseLatency(socket.rtt))
    .filter((latency) => Number.isFinite(latency));
};

const latencySummary = (values) => {
  if (!values.length) {
    return { min: null, avg: null, max: null };
  }
  const min = Math.min(...values);
  const max = Math.max(...values);
  const avg = values.reduce((sum, latency) => sum + latency, 0) / values.length;
  return { min, avg, max };
};

const setLatencyMetric = (state) => {
  const node = document.getElementById("latency");
  if (!node) return;
  const values = activeLatencies(state);
  if (!values.length) {
    node.textContent = "No RTT";
    node.className = "metric-value fit-value";
    return;
  }
  const { avg } = latencySummary(values);
  node.textContent = `${formatLatency(avg)} avg`;
  node.className = "metric-value fit-value";
};

const socketSummary = (state) => {
  const sockets = state.sockets || [];
  const active = activeAccelerationSockets(state);
  const external = externalSockets(state);
  return {
    total: sockets.length,
    active: active.length,
    external: external.length,
    listeners: sockets.filter((socket) => socket.state === "LISTEN" || socket.state === "UNCONN").length,
    cooling: external.filter((socket) => socket.state === "TIME-WAIT").length,
    control: external.filter((socket) => socket.state === "ESTAB" && String(socket.peer_port || "") === "16000").length,
    udp: sockets.filter((socket) => String(socket.protocol || "").startsWith("udp")).length,
  };
};

const renderSocketMix = (state) => {
  const body = document.getElementById("socket-mix");
  if (!body) return;
  const summary = socketSummary(state);
  body.replaceChildren(
    tag(`Active ${summary.active}`, "mix-tag latency-good"),
    tag(`External ${summary.external}`, "mix-tag protocol-tag"),
    tag(`Control ${summary.control}`, "mix-tag state-estab"),
    tag(`Cooling ${summary.cooling}`, "mix-tag state-time-wait"),
    tag(`Listening ${summary.listeners}`, "mix-tag state-listen"),
    tag(`UDP ${summary.udp}`, "mix-tag peer-tag"),
  );
};

const renderFlow = (state, health) => {
  setText("flow-phone", state.address ? state.address.split("/")[0] : "");
  setText("flow-gateway", state.gateway);
  const active = activeAccelerationSockets(state);
  const upstream = active[0] ? endpoint(active[0].peer_address, active[0].peer_port) : "";
  setText("flow-upstream", active.length > 1 ? `${upstream} +${active.length - 1}` : upstream);
  setText("flow-quality", health.label);
  renderSocketMix(state);
};

const setPath = (id, value) => {
  const node = document.getElementById(id);
  if (node) node.setAttribute("d", value);
};

const renderLatencyTrend = (state) => {
  const current = latencySummary(activeLatencies(state));
  if (Number.isFinite(current.avg)) {
    latencyHistory.push(current.avg);
    if (latencyHistory.length > maxLatencySamples) latencyHistory.shift();
  }

  if (!latencyHistory.length) {
    setPath("latency-area", "");
    setPath("latency-path", "");
    setText("latency-min", "");
    setText("latency-avg", "");
    setText("latency-max", "");
    setText("latency-range", "No RTT");
    return;
  }

  const stats = Number.isFinite(current.avg) ? current : latencySummary(latencyHistory);
  setText("latency-min", formatLatency(stats.min));
  setText("latency-avg", formatLatency(stats.avg));
  setText("latency-max", formatLatency(stats.max));
  setText("latency-range", Number.isFinite(current.avg) ? `${activeLatencies(state).length} RTT samples` : "Last RTT samples");

  const width = 420;
  const height = 150;
  const padX = 16;
  const padY = 18;
  const min = Math.min(...latencyHistory);
  const max = Math.max(...latencyHistory);
  const span = Math.max(max - min, 1);
  const points = latencyHistory.map((value, index) => {
    const x = latencyHistory.length === 1
      ? width / 2
      : padX + index * ((width - padX * 2) / (latencyHistory.length - 1));
    const y = height - padY - ((value - min) / span) * (height - padY * 2);
    return [x, y];
  });
  const line = points.map(([x, y], index) => `${index === 0 ? "M" : "L"} ${x.toFixed(1)} ${y.toFixed(1)}`).join(" ");
  const first = points[0];
  const last = points[points.length - 1];
  const area = `${line} L ${last[0].toFixed(1)} ${height - padY} L ${first[0].toFixed(1)} ${height - padY} Z`;
  setPath("latency-path", line);
  setPath("latency-area", area);
};

const renderTargets = (state) => {
  const body = document.getElementById("targets-grid");
  if (!body) return;
  const active = activeAccelerationSockets(state);
  setText("targets-count", `${active.length} upstream`);
  body.replaceChildren();
  if (!active.length) {
    const empty = document.createElement("div");
    empty.className = "empty-panel";
    empty.textContent = "No active upstream traffic";
    body.appendChild(empty);
    return;
  }

  const targets = new Map();
  active.forEach((socket) => {
    const key = endpoint(socket.peer_address, socket.peer_port);
    const latency = parseLatency(socket.rtt);
    const target = targets.get(key) || {
      endpoint: key,
      protocol: socket.protocol,
      state: socket.state,
      count: 0,
      latencyTotal: 0,
      latencyCount: 0,
    };
    target.count += 1;
    if (Number.isFinite(latency)) {
      target.latencyTotal += latency;
      target.latencyCount += 1;
    }
    targets.set(key, target);
  });

  Array.from(targets.values())
    .sort((left, right) => right.count - left.count || left.endpoint.localeCompare(right.endpoint))
    .slice(0, 6)
    .forEach((target) => {
      const card = document.createElement("article");
      card.className = "target-card";
      const title = document.createElement("strong");
      title.textContent = target.endpoint;
      const meta = document.createElement("div");
      meta.className = "target-meta";
      const detail = document.createElement("span");
      detail.textContent = `${target.protocol || "-"} / ${target.state || "-"} / ${target.count} conn`;
      const latency = target.latencyCount ? target.latencyTotal / target.latencyCount : null;
      meta.append(detail, latencyTagValue(latency));
      card.append(title, meta);
      body.appendChild(card);
    });
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
      const value = column(item);
      if (value instanceof Node) {
        cell.appendChild(value);
      } else {
        cell.textContent = value || "-";
      }
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
  setLatencyMetric(state);
  renderFlow(state, health);
  renderLatencyTrend(state);
  renderTargets(state);

  renderRows("neighbors-body", state.neighbors || [], [
    (neighbor) => neighbor.address,
    (neighbor) => neighbor.mac,
    (neighbor) => neighbor.state,
  ]);
  renderRows("sockets-body", state.sockets || [], [
    (socket) => protocolTag(socket.protocol),
    (socket) => stateTag(socket.state),
    (socket) => endpoint(socket.local_address, socket.local_port),
    (socket) => endpoint(socket.peer_address, socket.peer_port),
    (socket) => latencyTag(socket.rtt),
    (socket) => peerTag(socket.lan_peer),
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
