/* ============================================================================
 * pTerminal – Production App JS
 * ============================================================================
 */
(() => {
  /* ===================== RPC ===================== */

  function rpc(req) {
    return window.rpc(JSON.stringify(req)).then((res) => {
      const data = JSON.parse(res);
      if (!data.ok) throw data;
      return data;
    });
  }

  const el = (id) => document.getElementById(id);

  const esc = (s) =>
    String(s).replace(
      /[&<>"']/g,
      (m) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        }[m])
    );

  const b64enc = (s) =>
    btoa(
      Array.from(new TextEncoder().encode(s))
        .map((b) => String.fromCharCode(b))
        .join("")
    );

  const b64dec = (s) =>
    new TextDecoder().decode(Uint8Array.from(atob(s), (c) => c.charCodeAt(0)));

  /* ===================== State ===================== */

  let config = null;
  let activeNetworkId = null;
  let activeHostId = null;
  let activeState = "disconnected";

  const hostTerms = new Map(); // hostId -> { pane, term, fitAddon, clipboardAddon, searchAddon, ... }
  let activePane = null;

  let term = null;
  let fitAddon = null;
  let clipboardAddon = null;
  let searchAddon = null;
  let webLinksAddon = null;
  let webglAddon = null;
  let serializeAddon = null;
  let unicode11Addon = null;
  let ligaturesAddon = null;
  let imageAddon = null;
  let resizeBound = false;
  let clipboardBound = false;

  let pasteBuffer = null;
  let pendingInput = "";
  let inputFlushTimer = null;

  /* ===================== Terminal ===================== */

  function safeLoadAddon(t, addon, name) {
    try {
      t.loadAddon(addon);
      return true;
    } catch (e) {
      console.warn(`Addon failed to load: ${name}`, e);
      return false;
    }
  }

  function queueInput(data) {
    if (!activeHostId || !data) return;
    if (activeState !== "connected") return;
    pendingInput += data;

    if (inputFlushTimer) return;
    inputFlushTimer = setTimeout(() => {
      const hostId = activeHostId;
      const toSend = pendingInput;
      pendingInput = "";
      inputFlushTimer = null;
      if (!hostId || !toSend) return;

      // High-frequency path: avoid JSON parsing and just fire-and-forget.
      window
        .rpc(
          JSON.stringify({
            type: "input",
            hostId,
            dataB64: b64enc(toSend),
          })
        )
        .catch(() => {});
    }, 8);
  }

  function ensurePasteBuffer() {
    if (pasteBuffer) return pasteBuffer;

    pasteBuffer = document.createElement("textarea");
    pasteBuffer.style.position = "fixed";
    pasteBuffer.style.opacity = "0";
    pasteBuffer.style.pointerEvents = "none";
    pasteBuffer.style.left = "-1000px";
    pasteBuffer.style.top = "-1000px";

    document.body.appendChild(pasteBuffer);

    return pasteBuffer;
  }

  async function writeClipboardText(text) {
    if (!text) return;

    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // fall through
    }

    const buf = ensurePasteBuffer();
    buf.value = text;
    buf.select();
    document.execCommand("copy");
  }

  async function readClipboardText() {
    try {
      return (await navigator.clipboard.readText()) || "";
    } catch {
      // fall through
    }

    const buf = ensurePasteBuffer();
    buf.value = "";
    buf.focus();
    document.execCommand("paste");
    await new Promise((r) => setTimeout(r, 0));
    const text = buf.value || "";
    term?.focus?.();
    return text;
  }

  function sendInputChunk(data) {
    if (!activeHostId || !data) return;
    if (activeState !== "connected") return;
    window
      .rpc(
        JSON.stringify({
          type: "input",
          hostId: activeHostId,
          dataB64: b64enc(data),
        })
      )
      .catch(() => {});
  }

  function pasteText(text) {
    if (!text) return;

    // Normalize common clipboard newlines for terminals
    const normalized = String(text).replace(/\r\n/g, "\n");

    // Chunk to avoid UI stalls on large pastes/base64 encodes.
    const chunkSize = 16384;
    let i = 0;

    const pump = () => {
      const end = Math.min(i + chunkSize, normalized.length);
      sendInputChunk(normalized.slice(i, end));
      i = end;
      if (i < normalized.length) setTimeout(pump, 0);
    };

    pump();
  }

  async function pasteFromClipboard() {
    const text = await readClipboardText();
    pasteText(text);
  }

  async function copySelectionToClipboard() {
    const sel = term?.getSelection?.() || "";
    if (!sel) return;
    await writeClipboardText(sel);
    term?.focus?.();
  }

  function createTerminalForHost(hostId) {
    const pane = document.createElement("div");
    // Create visible; we'll hide other panes in activateTerminalForHost.
    pane.className = "term-pane";
    pane.dataset.hostId = String(hostId);
    el("terminal-container").appendChild(pane);

    const t = new Terminal({
      cursorBlink: true,
      fontFamily: "monospace",
      fontSize: 13,
      scrollback: 5000,
      // Required for some addons (e.g. unicode11, ligatures, image)
      allowProposedApi: true,
    });

    t.open(pane);

    const entry = {
      pane,
      term: t,
      fitAddon: new FitAddon.FitAddon(),
      clipboardAddon: new ClipboardAddon.ClipboardAddon(),
      searchAddon: new SearchAddon.SearchAddon(),
      webLinksAddon: new WebLinksAddon.WebLinksAddon(),
      webglAddon: new WebglAddon.WebglAddon(),
      serializeAddon: new SerializeAddon.SerializeAddon(),
      unicode11Addon: new Unicode11Addon.Unicode11Addon(),
      ligaturesAddon: new LigaturesAddon.LigaturesAddon(),
      imageAddon: new ImageAddon.ImageAddon(),
    };

    // Load core addons first; optional ones are best-effort.
    safeLoadAddon(t, entry.fitAddon, "fit");
    safeLoadAddon(t, entry.clipboardAddon, "clipboard");
    safeLoadAddon(t, entry.searchAddon, "search");
    safeLoadAddon(t, entry.webLinksAddon, "web-links");
    safeLoadAddon(t, entry.serializeAddon, "serialize");
    safeLoadAddon(t, entry.unicode11Addon, "unicode11");
    safeLoadAddon(t, entry.webglAddon, "webgl");
    safeLoadAddon(t, entry.ligaturesAddon, "ligatures");
    safeLoadAddon(t, entry.imageAddon, "image");

    try {
      t.unicode.activeVersion = "11";
    } catch {
      // ignore
    }

    t.onData(queueInput);

    // ---- Clipboard ----
    ensurePasteBuffer();
    t.attachCustomKeyEventHandler((e) => {
      if (e.type !== "keydown") return true;

      // Paste: Ctrl+Shift+V, Shift+Insert
      if (
        (e.ctrlKey && e.shiftKey && e.key === "V") ||
        (e.shiftKey && e.key === "Insert")
      ) {
        pasteFromClipboard().catch(() => {});
        return false;
      }

      // Copy: Ctrl+Shift+C, Ctrl+Insert
      if (
        (e.ctrlKey && e.shiftKey && e.key === "C") ||
        (e.ctrlKey && e.key === "Insert")
      ) {
        copySelectionToClipboard().catch(() => {});
        return false;
      }

      return true;
    });

    hostTerms.set(hostId, entry);
    return entry;
  }

  function activateTerminalForHost(hostId) {
    if (!hostId) {
      if (activePane) activePane.classList.add("hidden");
      activePane = null;
      term = null;
      fitAddon = null;
      clipboardAddon = null;
      searchAddon = null;
      webLinksAddon = null;
      webglAddon = null;
      serializeAddon = null;
      unicode11Addon = null;
      ligaturesAddon = null;
      imageAddon = null;
      updateTerminalActions();
      return;
    }

    let entry = hostTerms.get(hostId);
    if (!entry) entry = createTerminalForHost(hostId);

    if (activePane && activePane !== entry.pane) activePane.classList.add("hidden");
    entry.pane.classList.remove("hidden");
    activePane = entry.pane;

    term = entry.term;
    fitAddon = entry.fitAddon;
    clipboardAddon = entry.clipboardAddon;
    searchAddon = entry.searchAddon;
    webLinksAddon = entry.webLinksAddon;
    webglAddon = entry.webglAddon;
    serializeAddon = entry.serializeAddon;
    unicode11Addon = entry.unicode11Addon;
    ligaturesAddon = entry.ligaturesAddon;
    imageAddon = entry.imageAddon;

    // Defer a tick so layout is settled (avoids "not opened" / 0-size issues in some WebViews).
    requestAnimationFrame(() => fitAddon?.fit?.());
    term.focus();
    updateTerminalActions();

    if (!clipboardBound) {
      clipboardBound = true;

      // Right-click → Paste
      el("terminal-container").addEventListener("contextmenu", (e) => {
        e.preventDefault();
        pasteFromClipboard().catch(() => {});
      });
    }

    if (!resizeBound) {
      resizeBound = true;
      window.addEventListener("resize", () => {
        if (!term || !activeHostId) return;
        fitAddon.fit();
        window
          .rpc(
            JSON.stringify({
              type: "resize",
              hostId: activeHostId,
              cols: term.cols,
              rows: term.rows,
            })
          )
          .catch(() => {});
      });
    }
  }

  window.dispatchPTY = (hostId, b64) => {
    const entry = hostTerms.get(hostId);
    if (!entry) return;
    entry.term.write(b64dec(b64));
  };

  /* ===================== Status ===================== */

  function updateStatus(state) {
    const status = el("status");
    const text = status.querySelector(".status-text");
    const attempts = status.querySelector(".status-attempts");

    status.classList.remove(
      "status-connected",
      "status-disconnected",
      "status-reconnecting"
    );

    if (!state || state.state === "disconnected") {
      activeState = "disconnected";
      status.classList.add("status-disconnected");
      text.textContent = "disconnected";
      attempts.textContent = "";
      updateTerminalActions();
      return;
    }

    if (state.state === "connected") {
      activeState = "connected";
      status.classList.add("status-connected");
      text.textContent = "connected";
      attempts.textContent = "";
      updateTerminalActions();
      return;
    }

    if (state.state === "reconnecting") {
      activeState = "reconnecting";
      status.classList.add("status-reconnecting");
      text.textContent = "reconnecting";
      attempts.textContent = state.attempts ? ` (#${state.attempts})` : "";
      updateTerminalActions();
    }
  }

  setInterval(() => {
    if (!activeHostId) return updateStatus(null);
    rpc({ type: "state", hostId: activeHostId })
      .then(updateStatus)
      .catch(() => {});
  }, 1200);

  /* ===================== Rendering ===================== */

  let hostMenuTarget = null;

  function hideHostMenu() {
    const menu = el("host-menu");
    menu.classList.add("hidden");
    hostMenuTarget = null;
  }

  function showHostMenuAt(x, y) {
    const menu = el("host-menu");
    const margin = 10;
    const w = menu.offsetWidth || 200;
    const h = menu.offsetHeight || 120;
    const maxX = window.innerWidth - w - margin;
    const maxY = window.innerHeight - h - margin;
    menu.style.left = `${Math.max(margin, Math.min(x, maxX))}px`;
    menu.style.top = `${Math.max(margin, Math.min(y, maxY))}px`;
    menu.classList.remove("hidden");
  }

  async function openHostMenu(e, host) {
    e.preventDefault();
    e.stopPropagation();

    hostMenuTarget = host;

    const connectBtn = el("host-menu-connect");
    connectBtn.disabled = true;
    connectBtn.classList.remove("hidden");
    connectBtn.textContent = "Connect";

    try {
      const state = await rpc({ type: "state", hostId: host.id });
      if (state.state === "connected") {
        connectBtn.classList.add("hidden");
      } else if (state.state === "reconnecting") {
        connectBtn.disabled = true;
        connectBtn.textContent = "Reconnecting…";
      } else {
        connectBtn.disabled = false;
        connectBtn.textContent = "Connect";
      }
    } catch {
      // If state fails, keep connect enabled as best effort
      connectBtn.disabled = false;
      connectBtn.textContent = "Connect";
    }

    showHostMenuAt(e.clientX, e.clientY);
  }

  function deleteHostFromConfig(host) {
    if (!host) return;
    if (host.id === activeHostId) {
      alert("Disconnect before deleting this host.");
      return;
    }
    if (!confirm(`Delete host "${host.name}"?`)) return;

    const net = config.networks.find((n) => n.id === activeNetworkId);
    if (!net) return;
    net.hosts = net.hosts.filter((h) => h.id !== host.id);
    saveConfig();
  }

  function nextId(items) {
    const used = new Set();
    (items || []).forEach((it) => {
      const id = Number(it?.id) || 0;
      if (id > 0 && Number.isFinite(id)) used.add(id);
    });

    let candidate = 1;
    while (used.has(candidate)) candidate++;
    return candidate;
  }

  function nextNetworkId() {
    return nextId(config?.networks);
  }

  function nextHostId() {
    const allHosts = (config?.networks || []).flatMap((n) => n.hosts || []);
    return nextId(allHosts);
  }

  function renderNetworks() {
    const sel = el("network-select");
    sel.innerHTML = "";

    config.networks.forEach((net) => {
      const opt = document.createElement("option");
      opt.value = net.id;
      opt.textContent = net.name;
      sel.appendChild(opt);
    });

    if (!activeNetworkId && config.networks.length) {
      activeNetworkId = config.networks[0].id;
    }

    sel.value = activeNetworkId ?? "";
  }

  function renderHosts() {
    const container = el("hosts");
    container.innerHTML = "";

    const net = config.networks.find((n) => n.id === activeNetworkId);
    if (!net) return;

    net.hosts.forEach((h) => {
      const div = document.createElement("div");
      div.className = "node";
      if (h.id === activeHostId) div.classList.add("active");

      div.innerHTML = `
        <div class="node-name">${esc(h.name)}</div>
        <div class="node-meta">
          ${esc(h.user)}@${esc(h.host)}:${esc(h.port ?? 22)}
          · ${esc(h.driver || "ssh")}
          · ${esc(h.auth?.method || "password")}
        </div>
      `;

      div.onclick = () => connectHost(h);
      div.ondblclick = () => openEditor("host", "edit", h);
      div.oncontextmenu = (e) => openHostMenu(e, h);

      container.appendChild(div);
    });
  }

  /* ===================== Connection ===================== */

  async function connectHost(host) {
    try {
      activeHostId = host.id;
      activeState = "reconnecting";
      el("title").textContent = `${host.name} (${host.user}@${host.host})`;
      activateTerminalForHost(host.id);
      renderHosts();

      const driver = host.driver || "ssh";
      const req = {
        type: "select",
        hostId: host.id,
        cols: term.cols,
        rows: term.rows,
        // 🔑 send stored password immediately if available
        passwordB64:
          driver === "ssh" &&
          host.auth?.method === "password" &&
          host.auth.password
            ? b64enc(host.auth.password)
            : "",
      };

      try {
        await rpc(req);
        updateStatus({ state: "connected" });
        return;
      } catch (err) {
        // 🔐 Host key trust (new or changed)
        if (
          err.error === "unknown_host_key" ||
          err.error === "host_key_mismatch"
        ) {
          showTrustDialog(host.id, err.hostPort, err.fingerprint);
          return;
        }

        // 🔑 Password auth fallback ONLY if password missing
        if (
          err.error === "password_required" &&
          driver === "ssh" &&
          host.auth?.method === "password"
        ) {
          const pw = prompt(`Password for ${host.user}@${host.host}:`);
          if (!pw) return;

          try {
            await rpc({
              ...req,
              passwordB64: b64enc(pw),
            });

            // Cache entered password in memory (optional but UX-friendly)
            host.auth.password = pw;
            saveConfig();

            return;
          } catch {
            alert("Authentication failed.");
            return;
          }
        }

        alert(err.detail || err.error || "Connection failed");
      }
    } catch (e) {
      console.error("CONNECT ERROR", e);
      alert("Unexpected connection error.");
    }
  }

  async function disconnectActiveHost() {
    if (!activeHostId) return;
    try {
      await rpc({ type: "disconnect", hostId: activeHostId });
      activeState = "disconnected";
      updateStatus({ state: "disconnected" });
      const entry = hostTerms.get(activeHostId);
      if (entry) {
        entry.term.reset();
        entry.fitAddon.fit();
        entry.term.write("\r\n\x1b[31m[disconnected]\x1b[0m\r\n");
      }
    } catch (e) {
      alert(e.detail || e.error || "Failed to disconnect");
    }
  }

  /* ===================== Trust dialog ===================== */

  function showTrustDialog(hostId, hostPort, fingerprint) {
    el("trust-host").textContent = hostPort;
    el("trust-fingerprint").textContent = fingerprint;

    const modal = el("trust-modal");
    modal.classList.remove("hidden");

    el("trust-cancel").onclick = () => modal.classList.add("hidden");

    el("trust-accept").onclick = () => {
      rpc({ type: "trust_host", hostId })
        .then(() => {
          modal.classList.add("hidden");
          const host = config.networks
            .flatMap((n) => n.hosts)
            .find((h) => h.id === hostId);
          if (host) connectHost(host);
        })
        .catch((e) => alert(e.detail || e.error || "Failed to trust host"));
    };
  }

  /* ===================== Editor modal ===================== */

  let editorMode = null;
  let editorType = null;
  let editorTarget = null;

  function openEditor(type, mode, target = null) {
    editorType = type;
    editorMode = mode;
    editorTarget = target;

    el("editor-title").textContent = `${
      mode === "create" ? "Add" : "Edit"
    } ${type}`;

    // Show only relevant fields
    document.querySelectorAll("[data-scope]").forEach((n) => {
      n.classList.toggle("hidden", n.dataset.scope !== type);
    });

    el("editor-delete").classList.toggle("hidden", mode !== "edit");

    if (type === "network") {
      el("net-name").value = target?.name || "";
      validateEditor();
      el("editor-modal").classList.remove("hidden");
      return;
    }

    /* ---------- HOST ---------- */

    const auth = target?.auth || { method: "password", password: "" };
    const driver = target?.driver || "ssh";

    const hostHost = el("host-host");
    el("host-name").value = target?.name || "";
    hostHost.value = target?.host || "";
    hostHost.disabled = false;
    hostHost.readOnly = false;

    el("host-user").value = target?.user || "root";
    el("host-port").value = target?.port || 22;

    // Connection driver
    el("host-driver").value = driver;

    // Auth method
    el("host-auth").value = auth.method || "password";

    // Password field (ssh + ioshell)
    el("host-password").value = auth.password || "";

    // IOshell fields
    el("ioshell-path").value = target?.ioshell?.path || "";
    el("ioshell-protocol").value = target?.ioshell?.protocol || "ssh";
    el("ioshell-command").value = target?.ioshell?.command || "";

    applyHostDriverVisibility();

    validateEditor();
    el("editor-modal").classList.remove("hidden");
  }

  el("host-auth").addEventListener("change", () => {
    validateEditor();
  });

  function applyHostDriverVisibility() {
    const driver = el("host-driver")?.value || "ssh";
    document.querySelectorAll("#editor-form [data-driver]").forEach((n) => {
      n.classList.toggle("hidden", n.dataset.driver !== driver);
    });
    validateEditor();
  }

  el("host-driver").addEventListener("change", applyHostDriverVisibility);

  function closeEditor() {
    el("editor-modal").classList.add("hidden");
    editorMode = editorType = editorTarget = null;
  }

  function validateEditor() {
    let ok = true;

    if (editorType === "network") {
      ok = !!el("net-name").value.trim();
    }

    if (editorType === "host") {
      const driver = el("host-driver").value || "ssh";
      ok =
        el("host-name").value.trim() &&
        el("host-host").value.trim() &&
        el("host-user").value.trim() &&
        Number(el("host-port").value) > 0 &&
        driver &&
        (driver !== "ioshell" ||
          (el("ioshell-path").value.trim() && el("ioshell-protocol").value));
    }

    el("editor-save").disabled = !ok;
  }

  [
    "net-name",
    "host-name",
    "host-host",
    "host-user",
    "host-port",
    "host-driver",
    "host-auth",
    "ioshell-path",
    "ioshell-protocol",
    "ioshell-command",
  ].forEach((id) => el(id)?.addEventListener("input", validateEditor));

  el("editor-save").onclick = () => {
    if (editorType === "network") {
      if (editorMode === "create") {
        config.networks.push({
          id: nextNetworkId(),
          name: el("net-name").value.trim(),
          hosts: [],
        });
      } else {
        editorTarget.name = el("net-name").value.trim();
      }
    }

    if (editorType === "host") {
      const net = config.networks.find((n) => n.id === activeNetworkId);
      if (!net) return;

      const driver = el("host-driver").value || "ssh";

      const data = {
        name: el("host-name").value.trim(),
        host: el("host-host").value.trim(),
        user: el("host-user").value.trim(),
        port: Number(el("host-port").value),
        driver,
        auth: {
          method: el("host-auth").value,
          password: el("host-password").value || "",
        },
        ioshell:
          driver === "ioshell"
            ? {
                path: el("ioshell-path").value.trim(),
                protocol: el("ioshell-protocol").value || "ssh",
                command: el("ioshell-command").value || "",
              }
            : undefined,
      };

      if (editorMode === "create") {
        net.hosts.push({
          id: nextHostId(),
          ...data,
          hostKey: { mode: "known_hosts" },
          sftpEnabled: false,
        });
      } else {
        Object.assign(editorTarget, data);
      }
    }

    closeEditor();
    saveConfig();
  };

  el("editor-delete").onclick = () => {
    if (!editorTarget) return;

    if (editorType === "host") {
      if (editorTarget.id === activeHostId) {
        alert("Disconnect before deleting this host.");
        return;
      }

      if (!confirm(`Delete host "${editorTarget.name}"?`)) return;

      const net = config.networks.find((n) => n.id === activeNetworkId);
      net.hosts = net.hosts.filter((h) => h.id !== editorTarget.id);
    }

    if (editorType === "network") {
      if (editorTarget.hosts?.length) {
        alert("Delete all hosts in this network first.");
        return;
      }

      if (!confirm(`Delete network "${editorTarget.name}"?`)) return;

      config.networks = config.networks.filter((n) => n.id !== editorTarget.id);

      activeNetworkId = null;
      activeHostId = null;
    }

    closeEditor();
    saveConfig();
  };

  el("editor-cancel").onclick = closeEditor;
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") closeEditor();
  });

  /* ===================== Config ===================== */

  function saveConfig() {
    rpc({ type: "config_save", config })
      .then(loadConfig)
      .catch((e) => alert(e.detail || e.error || "Failed to save config"));
  }

  function loadConfig() {
    rpc({ type: "config_get" })
      .then((res) => {
        config = res.config;
        renderNetworks();
        renderHosts();
      })
      .catch((e) => alert("Failed to load config: " + (e.detail || e.error)));
  }

  /* ===================== Bind UI ===================== */

  function updateTerminalActions() {
    const hasTerm = !!term;
    const hasHost = !!activeHostId;
    const isConnected = activeState === "connected";

    el("btn-disconnect").disabled = !hasHost;
    el("btn-copy").disabled = !hasTerm;
    el("btn-clear").disabled = !hasTerm;
    el("btn-paste").disabled = !hasTerm || !isConnected;
    el("term-search").disabled = !hasTerm;
    el("btn-find-prev").disabled = !hasTerm;
    el("btn-find-next").disabled = !hasTerm;
  }

  function runSearch(next) {
    const q = el("term-search").value || "";
    if (!q || !searchAddon) return;
    const ok = next
      ? searchAddon.findNext(q, { incremental: true })
      : searchAddon.findPrevious(q, { incremental: true });
    if (!ok) {
      // no-op
    }
  }

  function bindUI() {
    const sel = el("network-select");

    sel.onchange = (e) => {
      activeNetworkId = Number(e.target.value) || null;
      activeHostId = null;
      activeState = "disconnected";
      activateTerminalForHost(null);
      el("title").textContent = "Select a host";
      renderHosts();
      updateStatus(null);
    };

    sel.ondblclick = () => {
      const net = config.networks.find((n) => n.id === activeNetworkId);
      if (net) openEditor("network", "edit", net);
    };

    el("btn-add-network").onclick = () => openEditor("network", "create");

    el("btn-add-host").onclick = () => {
      if (!activeNetworkId) {
        alert("Select a network first.");
        return;
      }
      openEditor("host", "create");
    };

    el("btn-export").onclick = () =>
      rpc({ type: "config_export" }).then((r) =>
        alert(`Config exported to:\n${r.path}`)
      );

    function showAbout() {
      el("about-modal").classList.remove("hidden");
    }

    function hideAbout() {
      el("about-modal").classList.add("hidden");
    }

    el("btn-about").onclick = () => showAbout();
    el("about-close").onclick = () => hideAbout();

    el("about-copy-email").onclick = () =>
      writeClipboardText(el("about-email").textContent || "").catch(() => {});

    el("about-copy-github").onclick = () =>
      writeClipboardText(el("about-github").textContent || "").catch(() => {});

    el("btn-copy").onclick = () => copySelectionToClipboard().catch(() => {});
    el("btn-paste").onclick = () => pasteFromClipboard().catch(() => {});
    el("btn-clear").onclick = () => term?.clear?.();
    el("btn-disconnect").onclick = () => disconnectActiveHost();

    el("term-search").addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        runSearch(!e.shiftKey);
        e.preventDefault();
      } else if (e.key === "Escape") {
        el("term-search").value = "";
        term?.focus?.();
        e.preventDefault();
      }
    });

    el("btn-find-next").onclick = () => runSearch(true);
    el("btn-find-prev").onclick = () => runSearch(false);

    document.querySelectorAll(".term-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        btn.classList.remove("pulse");
        // force reflow so repeated clicks retrigger animation
        void btn.offsetWidth;
        btn.classList.add("pulse");
        setTimeout(() => btn.classList.remove("pulse"), 180);
      });
    });

    // Host context menu
    el("host-menu-connect").onclick = () => {
      const h = hostMenuTarget;
      hideHostMenu();
      if (h) connectHost(h);
    };
    el("host-menu-edit").onclick = () => {
      const h = hostMenuTarget;
      hideHostMenu();
      if (h) openEditor("host", "edit", h);
    };
    el("host-menu-delete").onclick = () => {
      const h = hostMenuTarget;
      hideHostMenu();
      if (h) deleteHostFromConfig(h);
    };

    document.addEventListener("click", () => hideHostMenu());
    document.addEventListener("contextmenu", () => hideHostMenu());
    window.addEventListener("blur", () => hideHostMenu());

    // IOshell path picker (prefer native dialog so we get a real absolute path)
    el("ioshell-browse").onclick = async () => {
      try {
        const r = await rpc({ type: "ioshell_pick" });
        if (r.path) {
          el("ioshell-path").value = r.path;
          validateEditor();
          return;
        }
      } catch {
        // fall back to file input
      }

      el("ioshell-path-picker").click();
    };

    // Fallback: in some WebViews File objects expose .path
    el("ioshell-path-picker").addEventListener("change", () => {
      const f = el("ioshell-path-picker").files?.[0];
      const p = f?.path || "";
      if (p) el("ioshell-path").value = p;
      validateEditor();
    });

    // About modal: close on background click / escape
    el("about-modal").addEventListener("click", (e) => {
      if (e.target === el("about-modal")) hideAbout();
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") hideAbout();
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    bindUI();
    loadConfig();
    updateTerminalActions();
  });
})();
