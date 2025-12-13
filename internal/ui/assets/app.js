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

  let term = null;
  let fitAddon = null;
  let resizeBound = false;

  let pasteBuffer = null;

  function ensurePasteBuffer() {
    if (pasteBuffer) return pasteBuffer;

    pasteBuffer = document.createElement("textarea");
    pasteBuffer.style.position = "fixed";
    pasteBuffer.style.opacity = "0";
    pasteBuffer.style.pointerEvents = "none";
    pasteBuffer.style.left = "-1000px";
    pasteBuffer.style.top = "-1000px";

    document.body.appendChild(pasteBuffer);

    pasteBuffer.addEventListener("paste", (e) => {
      const text = (e.clipboardData || window.clipboardData)?.getData("text");
      if (text && activeHostId) {
        rpc({
          type: "input",
          hostId: activeHostId,
          dataB64: b64enc(text),
        }).catch(() => {});
      }
    });

    return pasteBuffer;
  }

  /* ===================== Terminal ===================== */

  function ensurePasteBuffer() {
    if (pasteBuffer) return pasteBuffer;

    pasteBuffer = document.createElement("textarea");
    pasteBuffer.style.position = "fixed";
    pasteBuffer.style.opacity = "0";
    pasteBuffer.style.pointerEvents = "none";
    pasteBuffer.style.left = "-1000px";
    pasteBuffer.style.top = "-1000px";

    document.body.appendChild(pasteBuffer);

    pasteBuffer.addEventListener("paste", (e) => {
      const text =
        (e.clipboardData || window.clipboardData)?.getData("text") || "";
      if (text && activeHostId) {
        rpc({
          type: "input",
          hostId: activeHostId,
          dataB64: b64enc(text),
        }).catch(() => {});
      }
    });

    return pasteBuffer;
  }

  function initTerminal() {
    if (term) term.dispose();

    term = new Terminal({
      cursorBlink: true,
      fontFamily: "monospace",
      fontSize: 13,
      scrollback: 5000,
    });

    fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);

    // ---- Normal typing ----
    term.onData((data) => {
      if (!activeHostId) return;

      // ✅ Only locally echo printable characters
      // Printable ASCII range + UTF-8 text
      if (/^[\x20-\x7E]+$/.test(data)) {
        term.write(data);
      }

      // 🔑 Always send ALL data to SSH
      rpc({
        type: "input",
        hostId: activeHostId,
        dataB64: b64enc(data),
      }).catch(() => {});
    });

    term.open(el("terminal-container"));
    fitAddon.fit();

    // ---- Clipboard (WebView-safe) ----
    const pasteTarget = ensurePasteBuffer();

    term.attachCustomKeyEventHandler((e) => {
      // Ctrl+Shift+V → Paste
      if (e.type === "keydown" && e.ctrlKey && e.shiftKey && e.key === "V") {
        pasteTarget.value = "";
        pasteTarget.focus();
        document.execCommand("paste");
        term.focus();
        return false;
      }

      // Ctrl+Shift+C → Copy
      if (e.type === "keydown" && e.ctrlKey && e.shiftKey && e.key === "C") {
        const sel = term.getSelection();
        if (sel) {
          pasteTarget.value = sel;
          pasteTarget.select();
          document.execCommand("copy");
          term.focus();
        }
        return false;
      }

      return true;
    });

    // Right-click → Paste
    el("terminal-container").addEventListener("contextmenu", (e) => {
      e.preventDefault();
      pasteTarget.value = "";
      pasteTarget.focus();
      document.execCommand("paste");
      term.focus();
    });

    // ---- Resize handling ----
    if (!resizeBound) {
      resizeBound = true;
      window.addEventListener("resize", () => {
        if (!term || !activeHostId) return;
        fitAddon.fit();
        rpc({
          type: "resize",
          hostId: activeHostId,
          cols: term.cols,
          rows: term.rows,
        }).catch(() => {});
      });
    }
  }

  window.dispatchPTY = (hostId, b64) => {
    if (hostId !== activeHostId || !term) return;
    term.write(b64dec(b64));
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
      status.classList.add("status-disconnected");
      text.textContent = "disconnected";
      attempts.textContent = "";
      return;
    }

    if (state.state === "connected") {
      status.classList.add("status-connected");
      text.textContent = "connected";
      attempts.textContent = "";
      return;
    }

    if (state.state === "reconnecting") {
      status.classList.add("status-reconnecting");
      text.textContent = "reconnecting";
      attempts.textContent = state.attempts ? ` (#${state.attempts})` : "";
    }
  }

  setInterval(() => {
    if (!activeHostId) return updateStatus(null);
    rpc({ type: "state", hostId: activeHostId })
      .then(updateStatus)
      .catch(() => {});
  }, 1200);

  /* ===================== Rendering ===================== */

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
          · ${esc(h.auth?.method || "password")}
        </div>
      `;

      div.onclick = () => connectHost(h);
      div.ondblclick = () => openEditor("host", "edit", h);

      container.appendChild(div);
    });
  }

  /* ===================== Connection ===================== */

  async function connectHost(host) {
    try {
      activeHostId = host.id;
      el("title").textContent = `${host.name} (${host.user}@${host.host})`;
      initTerminal();
      renderHosts();

      const req = {
        type: "select",
        hostId: host.id,
        cols: term.cols,
        rows: term.rows,
        // 🔑 send stored password immediately if available
        passwordB64:
          host.auth?.method === "password" && host.auth.password
            ? b64enc(host.auth.password)
            : "",
      };

      try {
        await rpc(req);
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

    const hostHost = el("host-host");
    el("host-name").value = target?.name || "";
    hostHost.value = target?.host || "";
    hostHost.disabled = false;
    hostHost.readOnly = false;

    el("host-user").value = target?.user || "root";
    el("host-port").value = target?.port || 22;

    // Auth method
    el("host-auth").value = auth.method || "password";

    // Password field (only for password auth)
    el("host-password").value = auth.password || "";
    el("host-password-row")?.classList.toggle(
      "hidden",
      el("host-auth").value !== "password"
    );

    validateEditor();
    el("editor-modal").classList.remove("hidden");
  }

  el("host-auth").addEventListener("change", () => {
    const isPassword = el("host-auth").value === "password";
    el("host-password-row")?.classList.toggle("hidden", !isPassword);
    validateEditor();
  });

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
      ok =
        el("host-name").value.trim() &&
        el("host-host").value.trim() &&
        el("host-user").value.trim() &&
        Number(el("host-port").value) > 0 &&
        el("host-auth").value;
    }

    el("editor-save").disabled = !ok;
  }

  [
    "net-name",
    "host-name",
    "host-host",
    "host-user",
    "host-port",
    "host-auth",
  ].forEach((id) => el(id)?.addEventListener("input", validateEditor));

  el("editor-save").onclick = () => {
    if (editorType === "network") {
      if (editorMode === "create") {
        config.networks.push({
          id: Date.now(),
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

      const data = {
        name: el("host-name").value.trim(),
        host: el("host-host").value.trim(),
        user: el("host-user").value.trim(),
        port: Number(el("host-port").value),
        auth: {
          method: el("host-auth").value,
          password: el("host-password").value || "",
        },
      };

      if (editorMode === "create") {
        net.hosts.push({
          id: Date.now(),
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

  function bindUI() {
    const sel = el("network-select");

    sel.onchange = (e) => {
      activeNetworkId = Number(e.target.value) || null;
      activeHostId = null;
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

    el("btn-about").onclick = () =>
      rpc({ type: "about" }).then((r) => alert(r.text));
  }

  document.addEventListener("DOMContentLoaded", () => {
    bindUI();
    loadConfig();
  });
})();
