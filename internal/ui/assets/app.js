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

  let toastSeq = 0;

  function notify(message, type = "info", opts = {}) {
    const text = String(message || "").trim();
    if (!text) return;
    let container = el("toast-container");
    if (!container) {
      container = document.createElement("div");
      container.id = "toast-container";
      container.className = "toast-container";
      container.setAttribute("aria-live", "polite");
      container.setAttribute("aria-atomic", "false");
      if (document.body) {
        document.body.appendChild(container);
      }
    }
    if (!container) {
      alert(text);
      return;
    }

    const toast = document.createElement("div");
    toast.className = `toast toast-${type}`;
    toast.setAttribute("role", "status");
    toast.setAttribute("aria-live", "polite");
    toast.dataset.toastId = String(++toastSeq);

    const body = document.createElement("div");
    body.className = "toast-body";
    body.textContent = text;

    const close = document.createElement("button");
    close.className = "toast-close";
    close.type = "button";
    close.textContent = "x";

    const dismiss = () => {
      toast.classList.remove("show");
      setTimeout(() => toast.remove(), 160);
    };

    close.onclick = dismiss;

    toast.appendChild(body);
    toast.appendChild(close);
    container.appendChild(toast);

    while (container.children.length > 5) {
      container.firstElementChild?.remove();
    }

    requestAnimationFrame(() => toast.classList.add("show"));

    const ttl =
      typeof opts.ttl === "number"
        ? opts.ttl
        : type === "error"
        ? 6000
        : 4000;
    if (ttl > 0) {
      setTimeout(dismiss, ttl);
    }
  }

  const notifyInfo = (msg, opts) => notify(msg, "info", opts);
  const notifySuccess = (msg, opts) => notify(msg, "success", opts);
  const notifyWarn = (msg, opts) => notify(msg, "warn", opts);
  const notifyError = (msg, opts) => notify(msg, "error", opts);

  let confirmResolver = null;

  function confirmDialog(message, opts = {}) {
    return new Promise((resolve) => {
      confirmResolver = resolve;
      el("confirm-message").textContent = String(message || "").trim();
      el("confirm-ok").textContent = opts.okText || "Confirm";
      el("confirm-cancel").textContent = opts.cancelText || "Cancel";
      el("confirm-ok").classList.toggle("danger", !!opts.danger);
      el("confirm-modal").classList.remove("hidden");
    });
  }

  function closeConfirm(result) {
    const modal = el("confirm-modal");
    if (modal) modal.classList.add("hidden");
    if (confirmResolver) {
      const resolve = confirmResolver;
      confirmResolver = null;
      resolve(result);
    }
  }

  const textEncoder = new TextEncoder();
  const textDecoder = new TextDecoder();

  function b64enc(s) {
    const bytes = textEncoder.encode(String(s));
    let binary = "";
    const chunk = 0x8000;
    for (let i = 0; i < bytes.length; i += chunk) {
      binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
    }
    return btoa(binary);
  }

  function b64decBytes(s) {
    const binary = atob(String(s));
    const len = binary.length;
    const bytes = new Uint8Array(len);
    for (let i = 0; i < len; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
  }

  function b64decText(s) {
    return textDecoder.decode(b64decBytes(s));
  }

  /* ===================== State ===================== */

  let config = null;
  let activeTeamId = "";
  let activeNetworkId = null;
  let activeHostId = null;
  let activeTermTabId = null;
  let activeState = "disconnected";
  let activeTab = "terminal"; // terminal | files
  let activeHostHasSFTP = false;

  let teamPresence = { peers: [], user: null };
  let teamRepoPaths = {};
  let teamsModalOpen = false;
  let activeTeamDetailId = null;
  let profileDirty = false;
  let profileEditing = false;
  const requestStatusCache = new Map();
  let requestStatusBootstrapped = false;

  const hostTerminals = new Map(); // hostId -> { tabs, tabOrder, tabNames, activeTabId, nextTabId, lastState }
  let activePane = null;

  const hostFiles = new Map(); // hostId -> { pane, cwd, selectedPath, entries, fileInput }
  const hostTabs = new Map(); // hostId -> "terminal" | "files"

  let term = null;
  let fitAddon = null;
  let clipboardAddon = null;
  let searchAddon = null;
  let webLinksAddon = null;
  let resizeBound = false;
  let clipboardBound = false;

  let pasteBuffer = null;
  let pendingInput = "";
  let inputFlushTimer = null;
  let resizeTimer = null;
  let lastResize = { cols: 0, rows: 0 };
  let inputInFlight = false;

  /* ===================== Teams ===================== */

  function normalizeEmail(email) {
    return String(email || "").trim().toLowerCase();
  }

  function isValidEmail(email) {
    const value = String(email || "").trim();
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
  }

  function newLocalId() {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
  }

  function userEmail() {
    return normalizeEmail(config?.user?.email || "");
  }

  function isUserInTeam(team) {
    const email = userEmail();
    if (!email) return false;
    return (team?.members || []).some((m) => normalizeEmail(m.email) === email);
  }

  function teamRole(team, email) {
    const norm = normalizeEmail(email);
    if (!norm) return "";
    const member = (team?.members || []).find(
      (m) => normalizeEmail(m.email) === norm
    );
    return member?.role || "";
  }

  function isTeamAdmin(team) {
    return teamRole(team, userEmail()) === "admin";
  }

  function checkRequestNotifications(cfg) {
    const email = normalizeEmail(cfg?.user?.email || "");
    if (!email) {
      requestStatusBootstrapped = true;
      return;
    }

    (cfg?.teams || []).forEach((team) => {
      if (!team || team.deleted || !team.id) return;
      const req = (team.requests || []).find(
        (r) => normalizeEmail(r.email) === email
      );
      const status = req?.status || "";
      const key = `${team.id}:${email}`;
      const prev = requestStatusCache.get(key);

      if (
        requestStatusBootstrapped &&
        prev === "pending" &&
        (status === "approved" || status === "declined")
      ) {
        const msg = `Team request ${status}: ${team.name || "team"}`;
        if (status === "approved") {
          notifySuccess(msg);
        } else {
          notifyWarn(msg);
        }
      }

      if (status) {
        requestStatusCache.set(key, status);
      } else {
        requestStatusCache.delete(key);
      }
    });

    requestStatusBootstrapped = true;
  }

  function findTeamRequest(team, email) {
    const norm = normalizeEmail(email);
    if (!norm) return null;
    return (team?.requests || []).find(
      (r) => normalizeEmail(r.email) === norm
    );
  }

  function visibleTeams() {
    return (config?.teams || []).filter((t) => !t.deleted && isUserInTeam(t));
  }

  function getTeamById(id) {
    return (config?.teams || []).find((t) => t.id === id);
  }

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

  function pumpOutput(entry) {
    const q = entry.outputQueue;
    if (!q || q.length === 0) return;
    if (entry.outputWriting) return;

    const start = performance.now();
    const parts = [];
    let totalBytes = 0;
    const maxBytes = 96 * 1024;
    const maxTimeMs = 7; // keep UI responsive
    let i = entry.outputReadIndex || 0;

    while (i < q.length) {
      const chunk = q[i++];
      if (!chunk) continue;
      const bytes = b64decBytes(chunk);
      parts.push(bytes);
      totalBytes += bytes.length;
      if (totalBytes >= maxBytes) break;
      if (performance.now() - start >= maxTimeMs) break;
    }

    if (totalBytes > 0) {
      let combined = parts[0];
      if (parts.length > 1) {
        combined = new Uint8Array(totalBytes);
        let offset = 0;
        for (const p of parts) {
          combined.set(p, offset);
          offset += p.length;
        }
      }

      entry.outputWriting = true;
      entry.term.write(combined, () => {
        entry.outputWriting = false;
        scheduleOutputFlush(entry);
      });
    }

    entry.outputReadIndex = i;

    // Compact periodically to avoid O(n) shifts on every pump.
    if (entry.outputReadIndex > 128 || entry.outputReadIndex > q.length / 2) {
      q.splice(0, entry.outputReadIndex);
      entry.outputReadIndex = 0;
    }

    // If xterm isn't currently processing a write, keep draining.
    if (!entry.outputWriting && q.length-(entry.outputReadIndex||0) > 0) {
      setTimeout(() => pumpOutput(entry), 0);
    }
  }

  function scheduleOutputFlush(entry) {
    if (!entry || entry.outputFlushScheduled) return;
    entry.outputFlushScheduled = true;

    setTimeout(() => {
      entry.outputFlushScheduled = false;
      pumpOutput(entry);
    }, 0);
  }

  function flushPendingInputSoon() {
    if (inputFlushTimer) return;
    inputFlushTimer = setTimeout(flushPendingInputNow, 6);
  }

  function flushPendingInputNow() {
    if (inputFlushTimer) {
      clearTimeout(inputFlushTimer);
      inputFlushTimer = null;
    }
    if (inputInFlight) return;

    const hostId = activeHostId;
    const tabId = activeTermTabId;
    const toSend = pendingInput;
    pendingInput = "";
    if (!hostId || !tabId || !toSend) return;
    inputInFlight = true;

    window
      .rpc(
        JSON.stringify({
          type: "input",
          hostId,
          tabId,
          dataB64: b64enc(toSend),
        })
      )
      .catch(() => {})
      .finally(() => {
        inputInFlight = false;
        if (pendingInput) flushPendingInputSoon();
      });
  }

  function queueInput(data) {
    if (!activeHostId || !activeTermTabId || !data) return;
    // Let the backend be authoritative for connection state. The UI state poll can
    // lag, and blocking here can make the first keystroke after connect "disappear".
    if (activeState === "disconnected") return;

    pendingInput += data;

    // If Enter is pressed, flush immediately so the remote runs and responds ASAP.
    if (data.includes("\r")) {
      flushPendingInputNow();
      return;
    }

    flushPendingInputSoon();
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
    if (!activeHostId || !activeTermTabId || !data) return;
    if (activeState === "disconnected") return;
    window
      .rpc(
        JSON.stringify({
          type: "input",
          hostId: activeHostId,
          tabId: activeTermTabId,
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

  function ensureHostTerminalState(hostId) {
    let state = hostTerminals.get(hostId);
    if (!state) {
      state = {
        tabs: new Map(), // tabId -> entry
        tabOrder: [1],
        tabNames: new Map([[1, "Tab 1"]]),
        activeTabId: 1,
        nextTabId: 2,
        lastState: new Map(), // tabId -> { state, attempts, detail, errCode }
      };
      hostTerminals.set(hostId, state);
    }

    if (!Array.isArray(state.tabOrder) || state.tabOrder.length === 0) {
      state.tabOrder = [1];
    }
    if (!(state.tabNames instanceof Map)) state.tabNames = new Map();
    if (!(state.tabs instanceof Map)) state.tabs = new Map();
    if (!(state.lastState instanceof Map)) state.lastState = new Map();

    if (!state.activeTabId || !state.tabOrder.includes(state.activeTabId)) {
      state.activeTabId = state.tabOrder[0];
    }

    const maxId = Math.max(0, ...state.tabOrder);
    if (!state.nextTabId || state.nextTabId <= maxId) state.nextTabId = maxId + 1;

    for (const id of state.tabOrder) {
      if (!state.tabNames.has(id)) state.tabNames.set(id, `Tab ${id}`);
    }

    return state;
  }

  function ensureTabMeta(state, tabId) {
    if (!state.tabOrder.includes(tabId)) state.tabOrder.push(tabId);
    if (!state.tabNames.has(tabId)) state.tabNames.set(tabId, `Tab ${tabId}`);
    if (tabId >= state.nextTabId) state.nextTabId = tabId + 1;
  }

  function createTerminalForTab(hostId, tabId) {
    const state = ensureHostTerminalState(hostId);
    ensureTabMeta(state, tabId);

    const pane = document.createElement("div");
    pane.className = "term-pane hidden";
    pane.dataset.hostId = String(hostId);
    pane.dataset.tabId = String(tabId);
    el("terminal-container").appendChild(pane);

    const t = new Terminal({
      cursorBlink: false,
      fontFamily: "monospace",
      fontSize: 13,
      scrollback: 2000,
    });

    t.open(pane);

    const entry = {
      pane,
      term: t,
      outputQueue: [],
      outputReadIndex: 0,
      outputFlushScheduled: false,
      outputWriting: false,
      fitAddon: new FitAddon.FitAddon(),
      clipboardAddon: new ClipboardAddon.ClipboardAddon(),
      searchAddon: new SearchAddon.SearchAddon(),
      webLinksAddon: new WebLinksAddon.WebLinksAddon(),
    };

    // Load core addons first; optional ones are best-effort.
    safeLoadAddon(t, entry.fitAddon, "fit");
    safeLoadAddon(t, entry.clipboardAddon, "clipboard");
    safeLoadAddon(t, entry.searchAddon, "search");
    safeLoadAddon(t, entry.webLinksAddon, "web-links");

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

    state.tabs.set(tabId, entry);
    return entry;
  }

  function getTerminalEntry(hostId, tabId) {
    const state = ensureHostTerminalState(hostId);
    const id = Number(tabId) || state.activeTabId || 1;
    let entry = state.tabs.get(id);
    if (!entry) entry = createTerminalForTab(hostId, id);
    return { state, tabId: id, entry };
  }

  function renderTerminalTabBar() {
    const bar = el("terminal-tabs");
    const list = el("terminal-tab-list");
    if (!bar || !list) return;

    if (!activeHostId || activeTab !== "terminal") {
      bar.classList.add("hidden");
      list.innerHTML = "";
      return;
    }

    bar.classList.remove("hidden");

    const state = ensureHostTerminalState(activeHostId);
    list.innerHTML = "";

    const canClose = state.tabOrder.length > 1;
    for (const id of state.tabOrder) {
      const btn = document.createElement("div");
      btn.className = "term-tab" + (id === state.activeTabId ? " active" : "");
      btn.setAttribute("role", "tab");
      btn.setAttribute("aria-selected", id === state.activeTabId ? "true" : "false");
      btn.title = "Double-click to rename";

      const label = document.createElement("span");
      label.textContent = state.tabNames.get(id) || `Tab ${id}`;

      btn.appendChild(label);

      if (canClose) {
        const close = document.createElement("span");
        close.className = "term-tab-close";
        close.textContent = "×";
        close.title = "Close tab";
        close.onclick = (e) => {
          e.stopPropagation();
          closeTerminalTab(activeHostId, id);
        };
        btn.appendChild(close);
      }

      btn.onclick = () => {
        const host = findHostById(activeHostId);
        if (!host) return;
        connectHostTab(host, id).catch(() => {});
      };

      btn.ondblclick = (e) => {
        e.preventDefault();
        renameTerminalTab(activeHostId, id);
      };

      list.appendChild(btn);
    }
  }

  function renameTerminalTab(hostId, tabId) {
    const state = ensureHostTerminalState(hostId);
    const current = state.tabNames.get(tabId) || `Tab ${tabId}`;
    const next = prompt("Tab name:", current);
    if (!next) return;
    state.tabNames.set(tabId, String(next).trim() || current);
    renderTerminalTabBar();
  }

  function closeTerminalTab(hostId, tabId) {
    const state = ensureHostTerminalState(hostId);
    if (state.tabOrder.length <= 1) return;

    rpc({ type: "disconnect", hostId, tabId }).catch(() => {});

    const entry = state.tabs.get(tabId);
    if (entry) {
      try {
        entry.term?.dispose?.();
      } catch {}
      try {
        entry.pane?.remove?.();
      } catch {}
    }
    state.tabs.delete(tabId);
    state.tabNames.delete(tabId);
    state.lastState.delete(tabId);
    state.tabOrder = state.tabOrder.filter((id) => id !== tabId);

    if (activeHostId === hostId && activeTermTabId === tabId) {
      const nextId = state.tabOrder[0] || 1;
      state.activeTabId = nextId;
      activeTermTabId = nextId;
      const host = findHostById(hostId);
      if (host) connectHostTab(host, nextId).catch(() => {});
    }

    renderTerminalTabBar();
  }

  function addTerminalTab(host) {
    const state = ensureHostTerminalState(host.id);
    const tabId = state.nextTabId++;
    ensureTabMeta(state, tabId);
    state.activeTabId = tabId;
    activeTermTabId = tabId;
    renderTerminalTabBar();
    connectHostTab(host, tabId).catch(() => {});
  }

  function activateTerminalForHostTab(hostId, tabId) {
    if (!hostId) {
      if (activePane) activePane.classList.add("hidden");
      activePane = null;
      term = null;
      fitAddon = null;
      clipboardAddon = null;
      searchAddon = null;
      webLinksAddon = null;
      activeTermTabId = null;
      updateTerminalActions();
      renderTerminalTabBar();
      return;
    }

    const state = ensureHostTerminalState(hostId);
    const id = Number(tabId) || state.activeTabId || 1;
    ensureTabMeta(state, id);
    state.activeTabId = id;
    activeTermTabId = id;

    let entry = state.tabs.get(id);
    if (!entry) entry = createTerminalForTab(hostId, id);

    if (activePane && activePane !== entry.pane) activePane.classList.add("hidden");
    entry.pane.classList.remove("hidden");
    activePane = entry.pane;

    term = entry.term;
    fitAddon = entry.fitAddon;
    clipboardAddon = entry.clipboardAddon;
    searchAddon = entry.searchAddon;
    webLinksAddon = entry.webLinksAddon;

    // Defer a tick so layout is settled (avoids "not opened" / 0-size issues in some WebViews).
    requestAnimationFrame(() => fitAddon?.fit?.());
    term.focus();
    updateTerminalActions();
    renderTerminalTabBar();

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
        if (!term || !activeHostId || !activeTermTabId) return;
        if (resizeTimer) clearTimeout(resizeTimer);
        resizeTimer = setTimeout(() => {
          resizeTimer = null;
          if (!term || !activeHostId || !activeTermTabId) return;
          fitAddon.fit();

          const cols = term.cols;
          const rows = term.rows;
          if (cols === lastResize.cols && rows === lastResize.rows) return;
          lastResize = { cols, rows };

          window
            .rpc(
              JSON.stringify({
                type: "resize",
                hostId: activeHostId,
                tabId: activeTermTabId,
                cols,
                rows,
              })
            )
            .catch(() => {});
        }, 60);
      });
    }

    // Ensure clicks inside the terminal area always focus xterm's textarea so the
    // first keystroke is not lost on some WebViews.
    if (!window.__pterminalTermFocusBound) {
      window.__pterminalTermFocusBound = true;
      el("terminal-container").addEventListener("mousedown", () => {
        if (activeTab !== "terminal") return;
        term?.focus?.();
      });
      window.addEventListener("focus", () => {
        if (activeTab !== "terminal") return;
        term?.focus?.();
      });
    }
  }

  function nextFrame() {
    return new Promise((resolve) => requestAnimationFrame(() => resolve()));
  }

  async function fitTerminalAndGetSize() {
    if (!term || !fitAddon) return { cols: 80, rows: 24 };
    // Two frames helps on WebView where layout/font metrics settle late.
    await nextFrame();
    fitAddon.fit();
    await nextFrame();
    fitAddon.fit();
    return { cols: term.cols, rows: term.rows };
  }

  window.dispatchPTY = (hostId, tabId, b64) => {
    if (!hostId || !b64) return;
    const { entry } = getTerminalEntry(hostId, tabId);
    entry.outputQueue.push(b64);
    scheduleOutputFlush(entry);
  };

  /* ===================== Tabs ===================== */

  function setActiveTab(tab) {
    if (!activeHostId) tab = "terminal";
    if (!activeHostHasSFTP) tab = "terminal";

    activeTab = tab;
    if (activeHostId) hostTabs.set(activeHostId, tab);

    const tabs = el("main-tabs");
    const tabTerm = el("tab-terminal");
    const tabFiles = el("tab-files");

    if (!activeHostId || !activeHostHasSFTP) {
      tabs.classList.add("hidden");
    } else {
      tabs.classList.remove("hidden");
    }

    tabFiles.disabled = !activeHostHasSFTP;

    tabTerm.classList.toggle("active", tab === "terminal");
    tabFiles.classList.toggle("active", tab === "files");
    tabTerm.setAttribute("aria-selected", tab === "terminal" ? "true" : "false");
    tabFiles.setAttribute("aria-selected", tab === "files" ? "true" : "false");

    el("terminal-container").classList.toggle("hidden", tab !== "terminal");
    el("files-container").classList.toggle("hidden", tab !== "files");

    updateTerminalActions();
    renderTerminalTabBar();

    if (tab === "files" && activeHostId) {
      ensureFilePane(activeHostId);
      refreshFiles(activeHostId).catch(() => {});
    } else if (tab === "terminal") {
      requestAnimationFrame(() => {
        fitAddon?.fit?.();
        if (activeHostId && activeTermTabId && activeState === "connected" && term) {
          window
            .rpc(
              JSON.stringify({
                type: "resize",
                hostId: activeHostId,
                tabId: activeTermTabId,
                cols: term.cols,
                rows: term.rows,
              })
            )
            .catch(() => {});
        }
      });
      term?.focus?.();
    }
  }

  function updateTabsForActiveHost(host) {
    activeHostHasSFTP = !!(host?.sftp?.enabled || host?.sftpEnabled);
    const remembered = activeHostId ? hostTabs.get(activeHostId) : null;
    setActiveTab(remembered || "terminal");
  }

  /* ===================== SFTP File Manager ===================== */

  function formatBytes(n) {
    const v = Number(n) || 0;
    if (v < 1024) return `${v} B`;
    const units = ["KB", "MB", "GB", "TB"];
    let x = v / 1024;
    let i = 0;
    while (x >= 1024 && i < units.length - 1) {
      x /= 1024;
      i++;
    }
    return `${x.toFixed(x >= 10 ? 0 : 1)} ${units[i]}`;
  }

  function formatTime(unix) {
    if (!unix) return "";
    const d = new Date(unix * 1000);
    return d.toLocaleString();
  }

  function findHostById(hostId) {
    return (
      config?.networks?.flatMap((n) => n.hosts || []).find((h) => h.id === hostId) ||
      null
    );
  }

  function showTrustDialogAsync(hostId, hostPort, fingerprint) {
    el("trust-host").textContent = hostPort;
    el("trust-fingerprint").textContent = fingerprint;

    const modal = el("trust-modal");
    modal.classList.remove("hidden");

    return new Promise((resolve, reject) => {
      const cleanup = () => {
        el("trust-cancel").onclick = null;
        el("trust-accept").onclick = null;
      };

      el("trust-cancel").onclick = () => {
        modal.classList.add("hidden");
        cleanup();
        reject(new Error("canceled"));
      };

      el("trust-accept").onclick = () => {
        rpc({ type: "trust_host", hostId })
          .then(() => {
            modal.classList.add("hidden");
            cleanup();
            resolve();
          })
          .catch((e) => {
            cleanup();
            notifyError(e.detail || e.error || "Failed to trust host");
            reject(e);
          });
      };
    });
  }

  async function sftpRpc(hostId, req) {
    const host = findHostById(hostId);
    if (!host) throw { error: "host_not_found" };

    // If SFTP is set to reuse connection credentials, opportunistically include the password.
    const sftpMode = host.sftp?.credentials || "connection";
    const needsConnPw = sftpMode !== "custom" && host.auth?.method === "password";
    const hasConnPw = !!host.auth?.password;

    const tryOnce = async (pw) => {
      const payload = { ...req, hostId };
      if (pw) payload.passwordB64 = b64enc(pw);
      return rpc(payload);
    };

    try {
      return await tryOnce(needsConnPw && hasConnPw ? host.auth.password : "");
    } catch (err) {
      // Host key trust
      if (err.error === "unknown_host_key" || err.error === "host_key_mismatch") {
        await showTrustDialogAsync(hostId, err.hostPort, err.fingerprint);
        return await tryOnce(needsConnPw && hasConnPw ? host.auth.password : "");
      }

      // Password required (only for connection creds + password auth)
      if (err.error === "password_required" && needsConnPw) {
        const pw = prompt(`Password for ${host.user}@${host.host} (SFTP):`);
        if (!pw) throw err;
        host.auth.password = pw;
        saveConfig();
        return await tryOnce(pw);
      }

      throw err;
    }
  }

  function ensureFilePane(hostId) {
    let entry = hostFiles.get(hostId);
    if (entry) return entry;

    const pane = document.createElement("div");
    pane.className = "file-pane hidden";
    pane.dataset.hostId = String(hostId);

    pane.innerHTML = `
      <div class="file-toolbar">
        <button class="btn small secondary" data-action="up" title="Up">Up</button>
        <input class="file-path" data-role="path" spellcheck="false" autocomplete="off" />
        <input class="file-search" data-role="search" type="text" placeholder="Search…" spellcheck="false" autocomplete="off" />
        <button class="btn small secondary" data-action="refresh" title="Refresh">Refresh</button>
        <button class="btn small secondary" data-action="mkdir" title="New folder">New Folder</button>
        <button class="btn small secondary" data-action="upload" title="Upload (or drag & drop)">Upload</button>
        <button class="btn small secondary" data-action="download" title="Download to ~/Downloads">Download</button>
        <button class="btn small secondary" data-action="rename" title="Rename">Rename</button>
        <button class="btn small" data-action="delete" style="color: #ff6b7d; border-color: rgba(255, 107, 125, 0.4)" title="Delete">
          Delete
        </button>
        <input type="file" data-role="file-input" class="hidden" multiple />
      </div>

      <div class="file-list" data-role="dropzone">
        <table class="file-table" role="grid" aria-label="SFTP files">
          <thead>
            <tr>
              <th style="width: 52%">Name</th>
              <th style="width: 18%">Size</th>
              <th style="width: 30%">Modified</th>
            </tr>
          </thead>
          <tbody data-role="tbody"></tbody>
        </table>
      </div>
    `;

    el("files-container").appendChild(pane);

    entry = {
      pane,
      cwd: ".",
      selectedPath: "",
      selectedRow: null,
      rowByPath: new Map(),
      entries: [],
      searchQuery: "",
      searchTimer: null,
      pathEl: pane.querySelector('[data-role="path"]'),
      searchEl: pane.querySelector('[data-role="search"]'),
      bodyEl: pane.querySelector('[data-role="tbody"]'),
      dropzoneEl: pane.querySelector('[data-role="dropzone"]'),
      fileInput: pane.querySelector('[data-role="file-input"]'),
    };

    // Toolbar actions
    pane.querySelectorAll("[data-action]").forEach((btn) => {
      btn.addEventListener("click", () => {
        const action = btn.dataset.action;
        if (action === "refresh") refreshFiles(hostId).catch(() => {});
        if (action === "up") navigateUp(hostId).catch(() => {});
        if (action === "mkdir") createFolder(hostId).catch(() => {});
        if (action === "upload") entry.fileInput.click();
        if (action === "download") downloadSelected(hostId).catch(() => {});
        if (action === "delete") deleteSelected(hostId).catch(() => {});
        if (action === "rename") renameSelected(hostId).catch(() => {});
      });
    });

    entry.pathEl.addEventListener("keydown", (e) => {
      if (e.key === "Enter") {
        navigateTo(hostId, entry.pathEl.value).catch(() => {});
        e.preventDefault();
      }
    });

    entry.searchEl.addEventListener("input", () => {
      entry.searchQuery = entry.searchEl.value || "";
      if (entry.searchTimer) clearTimeout(entry.searchTimer);
      entry.searchTimer = setTimeout(() => {
        entry.searchTimer = null;
        renderFileList(entry);
      }, 90);
    });
    entry.searchEl.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        entry.searchEl.value = "";
        entry.searchQuery = "";
        renderFileList(entry);
        e.preventDefault();
      }
    });

    // Drag & drop upload
    const stop = (e) => {
      e.preventDefault();
      e.stopPropagation();
    };
    entry.dropzoneEl.addEventListener("dragenter", stop);
    entry.dropzoneEl.addEventListener("dragover", stop);
    entry.dropzoneEl.addEventListener("drop", (e) => {
      stop(e);
      const files = Array.from(e.dataTransfer?.files || []);
      if (files.length) uploadFiles(hostId, files).catch(() => {});
    });

    // Context menu: empty area
    entry.dropzoneEl.addEventListener("contextmenu", (e) => {
      openFileMenu(e, { hostId, path: "", isDir: false, cwd: entry.cwd });
    });

    entry.fileInput.addEventListener("change", () => {
      const files = Array.from(entry.fileInput.files || []);
      entry.fileInput.value = "";
      if (files.length) uploadFiles(hostId, files).catch(() => {});
    });

    hostFiles.set(hostId, entry);
    return entry;
  }

  function activateFilePane(hostId) {
    const entry = ensureFilePane(hostId);
    hostFiles.forEach((e, id) => {
      if (e?.pane) e.pane.classList.toggle("hidden", id !== hostId);
    });
    entry.pane.classList.remove("hidden");
    return entry;
  }

  function renderFileList(entry) {
    const tbody = entry.bodyEl;
    tbody.innerHTML = "";
    entry.rowByPath = new Map();
    entry.selectedRow = null;

    const q = (entry.searchQuery || "").trim().toLowerCase();
    const items = q
      ? entry.entries.filter((e) => (e.name || "").toLowerCase().includes(q))
      : entry.entries;

    const frag = document.createDocumentFragment();
    items.forEach((it) => {
      const tr = document.createElement("tr");
      tr.className = "file-row";
      tr.draggable = true;
      tr.dataset.path = it.path;
      tr.dataset.isdir = it.isDir ? "1" : "0";

      if (it.path === entry.selectedPath) {
        tr.classList.add("selected");
        entry.selectedRow = tr;
      }
      entry.rowByPath.set(it.path, tr);

      tr.innerHTML = `
        <td>${esc(it.isDir ? it.name + "/" : it.name)}</td>
        <td class="file-muted">${it.isDir ? "" : esc(formatBytes(it.size))}</td>
        <td class="file-muted">${esc(formatTime(it.modUnix))}</td>
      `;

      tr.addEventListener("click", () => {
        setSelectedPath(entry, it.path);
      });

      tr.addEventListener("contextmenu", (e) => {
        setSelectedPath(entry, it.path);
        openFileMenu(e, {
          hostId: Number(entry.pane.dataset.hostId),
          path: it.path,
          isDir: it.isDir,
          cwd: entry.cwd,
        });
      });

      tr.addEventListener("dblclick", () => {
        if (it.isDir) {
          navigateTo(activeHostId, it.path).catch(() => {});
        } else {
          // convenience: double-click download
          setSelectedPath(entry, it.path);
          downloadSelected(activeHostId).catch(() => {});
        }
      });

      // Drag to move
      tr.addEventListener("dragstart", (e) => {
        e.dataTransfer?.setData("application/x-pterminal-sftp-path", it.path);
        e.dataTransfer?.setData("text/plain", it.path);
      });

      tr.addEventListener("dragover", (e) => {
        const from = e.dataTransfer?.getData("application/x-pterminal-sftp-path");
        if (!from) return;
        if (!it.isDir) return;
        e.preventDefault();
        tr.classList.add("drag-target");
      });

      tr.addEventListener("dragleave", () => tr.classList.remove("drag-target"));

      tr.addEventListener("drop", (e) => {
        const from = e.dataTransfer?.getData("application/x-pterminal-sftp-path");
        if (!from || !it.isDir) return;
        e.preventDefault();
        tr.classList.remove("drag-target");
        const to = `${it.path.replace(/\/+$/, "")}/${from.split("/").pop()}`;
        sftpRpc(activeHostId, { type: "sftp_mv", from, to })
          .then(() => refreshFiles(activeHostId))
          .catch((err) => notifyError(err.detail || err.error || "Move failed"));
      });

      frag.appendChild(tr);
    });
    tbody.appendChild(frag);
  }

  function setSelectedPath(entry, path) {
    entry.selectedPath = path || "";
    const next = entry.rowByPath?.get(entry.selectedPath) || null;
    if (entry.selectedRow && entry.selectedRow !== next) {
      entry.selectedRow.classList.remove("selected");
    }
    if (next) next.classList.add("selected");
    entry.selectedRow = next;
  }

  async function refreshFiles(hostId) {
    if (!hostId) return;
    const host = findHostById(hostId);
    if (!host) return;
    if (!(host.sftp?.enabled || host.sftpEnabled)) return;

    const entry = activateFilePane(hostId);

    const res = await sftpRpc(hostId, { type: "sftp_ls", path: entry.cwd });
    entry.cwd = res.cwd || entry.cwd || ".";
    entry.pathEl.value = entry.cwd;
    entry.entries = res.entries || [];
    entry.searchEl.value = entry.searchQuery || "";

    if (
      entry.selectedPath &&
      !entry.entries.some((e) => e.path === entry.selectedPath)
    ) {
      entry.selectedPath = "";
    }

    renderFileList(entry);
  }

  async function navigateTo(hostId, p) {
    const entry = ensureFilePane(hostId);
    entry.cwd = p || ".";
    await refreshFiles(hostId);
  }

  async function navigateUp(hostId) {
    const entry = ensureFilePane(hostId);
    const cur = entry.cwd || ".";
    const up = cur === "/" ? "/" : cur.replace(/\/+$/, "").split("/").slice(0, -1).join("/") || "/";
    entry.cwd = up;
    await refreshFiles(hostId);
  }

  async function createFolder(hostId) {
    const entry = ensureFilePane(hostId);
    const name = prompt("New folder name:");
    if (!name) return;
    await sftpRpc(hostId, { type: "sftp_mkdir", path: `${entry.cwd.replace(/\/+$/, "")}/${name}` });
    await refreshFiles(hostId);
  }

  async function deleteSelected(hostId) {
    const entry = ensureFilePane(hostId);
    if (!entry.selectedPath) return;
    if (!confirm(`Delete:\n${entry.selectedPath}`)) return;
    await sftpRpc(hostId, { type: "sftp_rm", path: entry.selectedPath });
    entry.selectedPath = "";
    await refreshFiles(hostId);
  }

  async function renameSelected(hostId) {
    const entry = ensureFilePane(hostId);
    const from = entry.selectedPath;
    if (!from) return;
    const base = from.split("/").pop();
    const name = prompt("Rename to:", base);
    if (!name || name === base) return;
    const to = `${entry.cwd.replace(/\/+$/, "")}/${name}`;
    await sftpRpc(hostId, { type: "sftp_mv", from, to });
    entry.selectedPath = to;
    await refreshFiles(hostId);
  }

  async function downloadSelected(hostId) {
    const entry = ensureFilePane(hostId);
    if (!entry.selectedPath) return;
    const r = await sftpRpc(hostId, { type: "sftp_download", path: entry.selectedPath });
    if (r.localPath) notifySuccess(`Downloaded to:\n${r.localPath}`);
  }

  function abToB64(buf) {
    let binary = "";
    const bytes = new Uint8Array(buf);
    const chunk = 0x8000;
    for (let i = 0; i < bytes.length; i += chunk) {
      binary += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk));
    }
    return btoa(binary);
  }

  async function uploadOne(hostId, file) {
    const entry = ensureFilePane(hostId);
    const begin = await sftpRpc(hostId, {
      type: "sftp_upload_begin",
      dir: entry.cwd,
      name: file.name,
    });
    const uploadId = begin.uploadId;
    if (!uploadId) throw new Error("uploadId missing");

    const chunkSize = 256 * 1024;
    let offset = 0;

    while (offset < file.size) {
      const slice = file.slice(offset, offset + chunkSize);
      const buf = await slice.arrayBuffer();
      await sftpRpc(hostId, {
        type: "sftp_upload_chunk",
        uploadId,
        dataB64: abToB64(buf),
      });
      offset += slice.size;
    }

    await sftpRpc(hostId, { type: "sftp_upload_end", uploadId });
  }

  async function uploadFiles(hostId, files) {
    if (!files?.length) return;
    for (const f of files) {
      await uploadOne(hostId, f);
    }
    await refreshFiles(hostId);
  }

  /* ===================== SFTP File Editor ===================== */

  let fileEditState = null; // { hostId, path, original, saving }

  function setupFileEditorModal() {
    const modal = el("file-edit-modal");
    const textarea = el("file-edit-text");
    const btnCancel = el("file-edit-cancel");
    const btnSave = el("file-edit-save");

    function isOpen() {
      return !modal.classList.contains("hidden");
    }

    function canClose() {
      if (!fileEditState) return true;
      const cur = textarea.value || "";
      if (cur === (fileEditState.original || "")) return true;
      return confirm("Discard unsaved changes?");
    }

    function close() {
      if (!canClose()) return;
      modal.classList.add("hidden");
      fileEditState = null;
      textarea.value = "";
      btnSave.disabled = false;
      btnSave.textContent = "Save";
      term?.focus?.();
    }

    btnCancel.onclick = close;

    btnSave.onclick = () => {
      if (!fileEditState || fileEditState.saving) return;
      const { hostId, path } = fileEditState;
      const text = textarea.value || "";

      fileEditState.saving = true;
      btnSave.disabled = true;
      btnSave.textContent = "Saving…";

      sftpRpc(hostId, { type: "sftp_write", path, dataB64: b64enc(text) })
        .then(() => {
          fileEditState.original = text;
          close();
          refreshFiles(hostId).catch(() => {});
        })
        .catch((e) => {
          notifyError(e.detail || e.error || "Save failed");
        })
        .finally(() => {
          if (fileEditState) fileEditState.saving = false;
          btnSave.disabled = false;
          btnSave.textContent = "Save";
        });
    };

    modal.addEventListener("click", (e) => {
      if (e.target === modal) close();
    });

    textarea.addEventListener("keydown", (e) => {
      // Ctrl+S / Cmd+S
      if ((e.ctrlKey || e.metaKey) && (e.key === "s" || e.key === "S")) {
        e.preventDefault();
        btnSave.click();
      } else if (e.key === "Escape") {
        e.preventDefault();
        close();
      }
    });

    document.addEventListener("keydown", (e) => {
      if (!isOpen()) return;
      if (e.key === "Escape") {
        e.preventDefault();
        close();
      }
    });
  }

  async function openFileEditor(hostId, remotePath) {
    if (!hostId || !remotePath) return;
    const r = await sftpRpc(hostId, { type: "sftp_read", path: remotePath });
    const text = b64decText(r.dataB64 || "");

    el("file-edit-path").textContent = remotePath;
    el("file-edit-text").value = text;
    el("file-edit-modal").classList.remove("hidden");
    el("file-edit-text").focus();

    fileEditState = { hostId, path: remotePath, original: text, saving: false };
  }

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
      attempts.textContent = state?.detail ? ` (${state.detail})` : "";
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
      text.textContent = state.attempts ? "reconnecting" : "connecting";
      attempts.textContent = state.attempts ? ` (#${state.attempts})` : "";
      updateTerminalActions();
    }
  }

  let statePollTimer = null;
  let statePollInFlight = false;
  const trustPrompted = new Set();
  const passwordPrompted = new Set();
  let lastConnectedKey = null;

  function nextStatePollDelay() {
    if (!activeHostId) return 2500;
    if (activeState === "reconnecting") return 650;
    if (activeState === "connected") return 3000;
    return 3500;
  }

  function scheduleStatePoll(delayMs = nextStatePollDelay()) {
    if (statePollTimer) clearTimeout(statePollTimer);
    statePollTimer = setTimeout(pollState, delayMs);
  }

  function pollState() {
    if (statePollInFlight) return scheduleStatePoll(500);

    if (!activeHostId || !activeTermTabId) {
      updateStatus(null);
      return scheduleStatePoll();
    }

    const pollHostId = activeHostId;
    const pollTabId = activeTermTabId;

    statePollInFlight = true;
    rpc({ type: "state", hostId: pollHostId, tabId: pollTabId })
      .then((s) => {
        if (pollHostId !== activeHostId || pollTabId !== activeTermTabId) return;
        const hostId = pollHostId;
        const tabId = pollTabId;

        try {
          const state = ensureHostTerminalState(hostId);
          state.lastState.set(tabId, s);
        } catch {}

        if (s.state === "connected") {
          trustPrompted.delete(hostId);
          passwordPrompted.delete(hostId);
          updateStatus(s);

          const key = `${hostId}:${tabId}`;
          if (lastConnectedKey !== key) {
            lastConnectedKey = key;
            // Best-effort: re-fit and send a resize after the connection is live.
            fitTerminalAndGetSize()
              .then(() =>
                rpc({
                  type: "resize",
                  hostId,
                  tabId,
                  cols: term?.cols || 80,
                  rows: term?.rows || 24,
                })
              )
              .catch(() => {});
          }
          return;
        }

        // Host key trust needed
        if (
          (s.errCode === "unknown_host_key" || s.errCode === "host_key_mismatch") &&
          s.hostPort &&
          s.fingerprint &&
          !trustPrompted.has(hostId)
        ) {
          trustPrompted.add(hostId);
          const host = findHostById(hostId);
          if (host) {
            showTrustDialogAsync(hostId, s.hostPort, s.fingerprint)
              .then(() => {
                trustPrompted.delete(hostId);
                connectHost(host);
              })
              .catch(() => {
                // user canceled; keep disconnected
              });
          }
        }

        // Password auth needed
        if (s.errCode === "password_required" && !passwordPrompted.has(hostId)) {
          const host = findHostById(hostId);
          const driver = host?.driver || "ssh";
          if (host && driver === "ssh" && host.auth?.method === "password") {
            passwordPrompted.add(hostId);
            const pw = prompt(`Password for ${host.user}@${host.host}:`);
            if (pw) {
              host.auth.password = pw;
              saveConfig();
              connectHost(host);
            } else {
              passwordPrompted.delete(hostId);
            }
          }
        }

        if (lastConnectedKey === `${hostId}:${tabId}` && s.state !== "connected") {
          lastConnectedKey = null;
        }

        updateStatus(s);
      })
      .catch(() => {})
      .finally(() => {
        statePollInFlight = false;
        scheduleStatePoll();
      });
  }

  scheduleStatePoll(300);

  /* ===================== Rendering ===================== */

  let hostMenuTarget = null;
  let fileMenuTarget = null; // { hostId, path, isDir, cwd }

  function hideHostMenu() {
    const menu = el("host-menu");
    menu.classList.add("hidden");
    hostMenuTarget = null;
  }

  function hideFileMenu() {
    const menu = el("file-menu");
    menu.classList.add("hidden");
    fileMenuTarget = null;
  }

  function hideAllMenus() {
    hideHostMenu();
    hideFileMenu();
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

  function showFileMenuAt(x, y) {
    const menu = el("file-menu");
    const margin = 10;
    const w = menu.offsetWidth || 220;
    const h = menu.offsetHeight || 260;
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

    showHostMenuAt(e.clientX, e.clientY);

    // Update connect button state asynchronously (avoid menu-open lag).
    rpc({ type: "state", hostId: host.id, tabId: 1 })
      .then((state) => {
        if (state.state === "connected") {
          connectBtn.classList.add("hidden");
        } else if (state.state === "reconnecting") {
          connectBtn.disabled = true;
          connectBtn.textContent = "Reconnecting…";
        } else {
          connectBtn.disabled = false;
          connectBtn.textContent = "Connect";
        }
      })
      .catch(() => {
        // If state fails, keep connect enabled as best effort
        connectBtn.disabled = false;
        connectBtn.textContent = "Connect";
      });
  }

  function updateFileMenuForTarget(t) {
    const hasSel = !!t?.path;
    const isDir = !!t?.isDir;

    el("file-menu-open").disabled = !hasSel || !isDir;
    el("file-menu-edit").disabled = !hasSel || isDir;
    el("file-menu-download").disabled = !hasSel || isDir;
    el("file-menu-rename").disabled = !hasSel;
    el("file-menu-delete").disabled = !hasSel;
    el("file-menu-copy-path").disabled = !hasSel;
  }

  function openFileMenu(e, target) {
    e.preventDefault();
    e.stopPropagation();
    fileMenuTarget = target || null;
    updateFileMenuForTarget(fileMenuTarget);
    showFileMenuAt(e.clientX, e.clientY);
  }

  function deleteHostFromConfig(host) {
    if (!host) return;
    if (host.id === activeHostId) {
      notifyWarn("Disconnect before deleting this host.");
      return;
    }
    if (!confirm(`Delete host "${host.name}"?`)) return;

    const net = config.networks.find((n) => n.id === activeNetworkId);
    if (!net) return;
    const target = net.hosts.find((h) => h.id === host.id);
    if (target) target.deleted = true;
    saveConfig();
  }

  function duplicateHostInConfig(host) {
    if (!host) return;
    const net = config.networks.find((n) => n.id === activeNetworkId);
    if (!net) return;

    const clone = JSON.parse(JSON.stringify(host));
    clone.id = nextHostId();
    clone.uid = "";
    clone.updatedAt = 0;
    clone.updatedBy = "";
    clone.version = null;
    clone.conflict = false;
    clone.deleted = false;

    const baseName = `${host.name} (duplicated)`;
    const used = new Set((net.hosts || []).map((h) => String(h?.name || "")));
    let name = baseName;
    let i = 2;
    while (used.has(name)) {
      name = `${baseName} ${i}`;
      i++;
    }
    clone.name = name;

    net.hosts.push(clone);
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

  function isPersonalView() {
    return !activeTeamId;
  }

  function isNetworkVisible(net) {
    if (!net || net.deleted) return false;
    if (isPersonalView()) {
      return !net.teamId;
    }
    return net.teamId === activeTeamId;
  }

  function isHostVisible(host) {
    if (!host || host.deleted) return false;
    if (isPersonalView()) {
      return host.scope !== "team" && !host.teamId;
    }
    return host.scope === "team" && host.teamId === activeTeamId;
  }

  function visibleNetworks() {
    return (config?.networks || []).filter((net) => isNetworkVisible(net));
  }

  function visibleHosts(net) {
    return (net?.hosts || []).filter((h) => isHostVisible(h));
  }

  function renderTeamSelect() {
    const sel = el("team-select");
    if (!sel) return;

    sel.innerHTML = "";
    const personal = document.createElement("option");
    personal.value = "";
    personal.textContent = "Personal";
    sel.appendChild(personal);

    const teams = visibleTeams();
    teams.forEach((t) => {
      const opt = document.createElement("option");
      opt.value = t.id;
      opt.textContent = t.name || "Untitled Team";
      sel.appendChild(opt);
    });

    if (activeTeamId && !teams.some((t) => t.id === activeTeamId)) {
      activeTeamId = "";
    }
    sel.value = activeTeamId ?? "";
  }

  function renderNetworks() {
    const sel = el("network-select");
    sel.innerHTML = "";
    const frag = document.createDocumentFragment();

    const nets = visibleNetworks();
    nets.forEach((net) => {
      const opt = document.createElement("option");
      opt.value = net.id;
      opt.textContent = net.name;
      frag.appendChild(opt);
    });
    sel.appendChild(frag);

    if (!activeNetworkId && nets.length) {
      activeNetworkId = nets[0].id;
    }
    if (activeNetworkId && !nets.some((n) => n.id === activeNetworkId)) {
      activeNetworkId = nets.length ? nets[0].id : null;
    }

    sel.value = activeNetworkId ?? "";
  }

  function renderHosts() {
    const container = el("hosts");
    container.innerHTML = "";
    const frag = document.createDocumentFragment();

    const net = config.networks.find((n) => n.id === activeNetworkId);
    if (!net) return;

    const hosts = visibleHosts(net);
    if (activeHostId && !hosts.some((h) => h.id === activeHostId)) {
      activeHostId = null;
      activeTermTabId = null;
      activeState = "disconnected";
      activeHostHasSFTP = false;
      el("title").textContent = "Select a host";
      updateStatus(null);
    }

    hosts.forEach((h) => {
      const div = document.createElement("div");
      div.className = "node";
      if (h.id === activeHostId) div.classList.add("active");

      const sftpOn = !!(h.sftp?.enabled || h.sftpEnabled);
      const sftpTag = sftpOn ? " · sftp" : "";
      const scopeTag = h.scope === "team" ? " · team" : " · private";

      div.innerHTML = `
        <div class="node-name">${esc(h.name)}</div>
        <div class="node-meta">
          ${esc(h.user)}@${esc(h.host)}:${esc(h.port ?? 22)}
          · ${esc(h.driver || "ssh")}
          · ${esc(h.auth?.method || "password")}
          ${scopeTag}
          ${sftpTag}
        </div>
      `;

      div.onclick = () => connectHost(h);
      div.ondblclick = () => openEditor("host", "edit", h);
      div.oncontextmenu = (e) => openHostMenu(e, h);

      frag.appendChild(div);
    });
    container.appendChild(frag);
  }

  /* ===================== Scripts ===================== */

  function isScriptVisible(script) {
    if (!script || script.deleted) return false;
    if (isPersonalView()) {
      return script.scope !== "team" && !script.teamId;
    }
    return script.scope === "team" && script.teamId === activeTeamId;
  }

  function renderScripts() {
    const container = el("scripts");
    if (!container) return;
    container.innerHTML = "";
    const frag = document.createDocumentFragment();

    const scripts = (config?.scripts || []).filter(isScriptVisible);
    scripts.forEach((script) => {
      const item = document.createElement("div");
      item.className = "script-item";

      const name = document.createElement("div");
      name.className = "script-name";
      name.textContent = script.name || "Untitled script";

      const meta = document.createElement("div");
      meta.className = "script-meta";
      meta.textContent = script.description || script.command || "";

      const actions = document.createElement("div");
      actions.className = "script-actions";

      const runBtn = document.createElement("button");
      runBtn.className = "btn small secondary";
      runBtn.textContent = "Run";
      runBtn.onclick = (e) => {
        e.stopPropagation();
        runScript(script);
      };

      const editBtn = document.createElement("button");
      editBtn.className = "btn small secondary";
      editBtn.textContent = "Edit";
      editBtn.onclick = (e) => {
        e.stopPropagation();
        openScriptEditor("edit", script);
      };

      actions.appendChild(runBtn);
      actions.appendChild(editBtn);

      item.appendChild(name);
      if (meta.textContent) item.appendChild(meta);
      item.appendChild(actions);

      item.ondblclick = () => openScriptEditor("edit", script);

      frag.appendChild(item);
    });

    container.appendChild(frag);
  }

  function runScript(script) {
    if (!script?.command) return;
    if (!activeHostId || !activeTermTabId) {
      notifyWarn("Select a host before running a script.");
      return;
    }
    queueInput(script.command + "\r");
  }

  /* ===================== Connection ===================== */

  async function connectHost(host) {
    const state = ensureHostTerminalState(host.id);
    const tabId = state.activeTabId || 1;
    return connectHostTab(host, tabId);
  }

  async function connectHostTab(host, tabId) {
    try {
      activeHostId = host.id;
      activeState = "reconnecting";
      el("title").textContent = `${host.name} (${host.user}@${host.host})`;
      activateTerminalForHostTab(host.id, tabId);
      updateTabsForActiveHost(host);
      renderHosts();
      updateStatus({ state: "reconnecting", attempts: 0 });

      const size = await fitTerminalAndGetSize();

      const driver = host.driver || "ssh";
      const req = {
        type: "select",
        hostId: host.id,
        tabId,
        cols: size.cols,
        rows: size.rows,
        // 🔑 send stored password immediately if available
        passwordB64:
          driver === "ssh" &&
          host.auth?.method === "password" &&
          host.auth.password
            ? b64enc(host.auth.password)
            : "",
      };

      rpc(req)
        .then(() => scheduleStatePoll(150))
        .catch((err) => notifyError(err.detail || err.error || "Connection failed"));
    } catch (e) {
      console.error("CONNECT ERROR", e);
      notifyError("Unexpected connection error.");
    }
  }

  async function disconnectActiveHost() {
    if (!activeHostId || !activeTermTabId) return;
    try {
      await rpc({ type: "disconnect", hostId: activeHostId, tabId: activeTermTabId });
      activeState = "disconnected";
      updateStatus({ state: "disconnected" });
      const { entry } = getTerminalEntry(activeHostId, activeTermTabId);
      entry.term.reset();
      entry.fitAddon.fit();
      entry.term.write("\r\n\x1b[31m[disconnected]\x1b[0m\r\n");

      // Keep file UI state but force terminal tab (SFTP session is closed on backend).
      setActiveTab("terminal");
    } catch (e) {
      notifyError(e.detail || e.error || "Failed to disconnect");
    }
  }

  /* ===================== Editor modal ===================== */

  let editorMode = null;
  let editorType = null;
  let editorTarget = null;
  let scriptEditorMode = null;
  let scriptEditorTarget = null;
  let pendingTeamName = null;
  let networkCopyTargets = new Set();

  function fillTeamSelect(select, selectedId, includeEmpty) {
    if (!select) return;
    select.innerHTML = "";
    if (includeEmpty) {
      const opt = document.createElement("option");
      opt.value = "";
      opt.textContent = "Select team";
      select.appendChild(opt);
    }
    const teams = (config?.teams || []).filter((t) => !t.deleted);
    teams.forEach((team) => {
      const opt = document.createElement("option");
      opt.value = team.id;
      opt.textContent = team.name || "Untitled Team";
      select.appendChild(opt);
    });
    if (selectedId && !teams.some((t) => t.id === selectedId)) {
      const opt = document.createElement("option");
      opt.value = selectedId;
      opt.textContent = `Unknown team (${selectedId.slice(0, 8)})`;
      select.appendChild(opt);
    }
    select.value = selectedId || "";
  }

  function renderNetworkCopyTeams(target) {
    const row = el("net-copy-row");
    const list = el("net-copy-teams");
    if (!row || !list) return;

    networkCopyTargets = new Set();
    list.innerHTML = "";

    if (editorType !== "network" || editorMode !== "edit" || !target) {
      row.classList.add("hidden");
      return;
    }

    const teams = (config?.teams || []).filter(
      (t) => !t.deleted && isTeamAdmin(t)
    );

    if (!teams.length) {
      row.classList.add("hidden");
      return;
    }

    row.classList.remove("hidden");

    teams.forEach((team) => {
      const label = document.createElement("label");
      label.className = "checkbox";

      const input = document.createElement("input");
      input.type = "checkbox";
      input.value = team.id;
      input.addEventListener("change", () => {
        if (input.checked) {
          networkCopyTargets.add(team.id);
        } else {
          networkCopyTargets.delete(team.id);
        }
      });

      const text = document.createElement("span");
      text.textContent = team.name || "Untitled Team";

      label.appendChild(input);
      label.appendChild(text);
      list.appendChild(label);
    });
  }

  function normalizeHostSFTP(host) {
    if (!host) return;

    const legacyEnabled = !!host.sftpEnabled;
    if (!host.sftp) {
      if (legacyEnabled) {
        host.sftp = {
          enabled: true,
          credentials: "connection",
          user: "",
          password: "",
        };
      }
      return;
    }

    if (host.sftp.enabled === false) {
      host.sftp = undefined;
      host.sftpEnabled = false;
      return;
    }

    host.sftp.enabled = true;
    const hasCustom = !!(host.sftp.user || host.sftp.password);
    if (host.sftp.credentials !== "custom" && host.sftp.credentials !== "connection") {
      host.sftp.credentials = hasCustom ? "custom" : "connection";
    }
    if (host.sftp.credentials === "connection") {
      host.sftp.user = "";
      host.sftp.password = "";
    }
    host.sftpEnabled = true;
  }

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
      const netScope = target?.teamId ? "team" : activeTeamId ? "team" : "private";
      const teamId = target?.teamId || (activeTeamId || "");
      el("net-scope").value = netScope;
      fillTeamSelect(el("net-team"), teamId, true);
      applyNetworkScopeVisibility();
      renderNetworkCopyTeams(target);
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

    const hostScope = target?.scope || (activeTeamId ? "team" : "private");
    el("host-scope").value = hostScope;
    const hostTeamId = target?.teamId || (activeTeamId || "");
    fillTeamSelect(el("host-team"), hostTeamId, true);

    // Connection driver
    el("host-driver").value = driver;

    // Auth method
    el("host-auth").value = auth.method || "password";

    // Password field (ssh + telecom)
    el("host-password").value = auth.password || "";

    normalizeHostSFTP(target);

    // ---- SFTP ----
    const sftpEnabled = !!(target?.sftp?.enabled || target?.sftpEnabled);
    el("sftp-enabled").checked = sftpEnabled;

    const sftpCredMode = target?.sftp?.credentials || "connection";
    el("sftp-cred-mode").value = sftpCredMode === "custom" ? "custom" : "connection";

    el("sftp-user").value = target?.sftp?.user || "";
    el("sftp-password").value = target?.sftp?.password || "";

    // Telecom fields
    el("telecom-path").value = target?.telecom?.path || "";
    el("telecom-protocol").value = target?.telecom?.protocol || "ssh";
    el("telecom-command").value = target?.telecom?.command || "";

    applyHostDriverVisibility();
    applyHostScopeVisibility();
    applySFTPVisibility();

    validateEditor();
    el("editor-modal").classList.remove("hidden");
  }

  el("host-auth").addEventListener("change", () => {
    validateEditor();
  });

  function applyNetworkScopeVisibility() {
    const scope = el("net-scope")?.value || "private";
    el("net-team-row")?.classList.toggle("hidden", scope !== "team");
    if (scope === "team" && !el("net-team")?.value && activeTeamId) {
      el("net-team").value = activeTeamId;
    }
  }

  function applyHostScopeVisibility() {
    const scope = el("host-scope")?.value || "private";
    el("host-team-row")?.classList.toggle("hidden", scope !== "team");
    if (scope === "team" && !el("host-team")?.value && activeTeamId) {
      el("host-team").value = activeTeamId;
    }
  }

  function applyHostDriverVisibility() {
    const driver = el("host-driver")?.value || "ssh";
    document.querySelectorAll("#editor-form [data-driver]").forEach((n) => {
      n.classList.toggle("hidden", n.dataset.driver !== driver);
    });
    applySFTPVisibility();
    validateEditor();
  }

  el("host-driver").addEventListener("change", applyHostDriverVisibility);
  el("host-scope").addEventListener("change", () => {
    applyHostScopeVisibility();
    validateEditor();
  });
  el("net-scope").addEventListener("change", () => {
    applyNetworkScopeVisibility();
    validateEditor();
  });

  function applySFTPVisibility() {
    if (editorType !== "host") return;
    const enabled = !!el("sftp-enabled")?.checked;
    el("sftp-cred-group")?.classList.toggle("hidden", !enabled);

    const mode = el("sftp-cred-mode")?.value || "connection";
    const showCustom = enabled && mode === "custom";
    el("sftp-custom-row")?.classList.toggle("hidden", !showCustom);
  }

  function closeEditor() {
    el("editor-modal").classList.add("hidden");
    editorMode = editorType = editorTarget = null;
    networkCopyTargets = new Set();
  }

  function validateEditor() {
    let ok = true;

    if (editorType === "network") {
      const scope = el("net-scope")?.value || "private";
      ok = !!el("net-name").value.trim();
      if (ok && scope === "team") {
        ok = !!el("net-team").value;
      }
    }

    if (editorType === "host") {
      const driver = el("host-driver").value || "ssh";
      ok =
        el("host-name").value.trim() &&
        el("host-host").value.trim() &&
        el("host-user").value.trim() &&
        Number(el("host-port").value) > 0 &&
        driver &&
        (driver !== "telecom" ||
          (el("telecom-path").value.trim() && el("telecom-protocol").value));

      if (ok && el("sftp-enabled")?.checked) {
        const mode = el("sftp-cred-mode")?.value || "connection";
        if (mode === "custom") {
          ok = !!el("sftp-user").value.trim() && !!el("sftp-password").value;
        }
      }
      if (ok && el("host-scope")?.value === "team") {
        ok = !!el("host-team").value;
      }
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
    "net-team",
    "host-team",
    "telecom-path",
    "telecom-protocol",
    "telecom-command",
    "sftp-user",
    "sftp-password",
  ].forEach((id) => el(id)?.addEventListener("input", validateEditor));

  ["sftp-enabled", "sftp-cred-mode", "net-scope", "host-scope"].forEach((id) =>
    el(id)?.addEventListener("change", () => {
      applySFTPVisibility();
      validateEditor();
    })
  );

  el("editor-save").onclick = () => {
    if (editorType === "network") {
      const netScope = el("net-scope").value || "private";
      const netTeam = netScope === "team" ? el("net-team").value : "";
      if (editorMode === "create") {
        config.networks.push({
          id: nextNetworkId(),
          name: el("net-name").value.trim(),
          teamId: netTeam,
          hosts: [],
        });
      } else {
        editorTarget.name = el("net-name").value.trim();
        editorTarget.teamId = netTeam;
        if (networkCopyTargets.size) {
          const source = editorTarget;
          const teams = Array.from(networkCopyTargets).filter((teamId) => {
            const team = getTeamById(teamId);
            return team && isTeamAdmin(team);
          });

          teams.forEach((teamId) => {
            const clone = {
              id: nextNetworkId(),
              name: source.name,
              teamId,
              uid: "",
              hosts: [],
              updatedAt: 0,
              updatedBy: "",
              version: null,
              conflict: false,
              deleted: false,
            };
            config.networks.push(clone);

            (source.hosts || [])
              .filter((h) => !h.deleted)
              .forEach((h) => {
                const hostClone = JSON.parse(JSON.stringify(h));
                hostClone.id = nextHostId();
                hostClone.uid = "";
                hostClone.scope = "team";
                hostClone.teamId = teamId;
                hostClone.updatedAt = 0;
                hostClone.updatedBy = "";
                hostClone.version = null;
                hostClone.conflict = false;
                hostClone.deleted = false;
                clone.hosts.push(hostClone);
              });
          });
        }
      }
    }

    if (editorType === "host") {
      const net = config.networks.find((n) => n.id === activeNetworkId);
      if (!net) return;

      const driver = el("host-driver").value || "ssh";

      const sftpEnabled = !!el("sftp-enabled").checked;
      const sftpMode = el("sftp-cred-mode").value || "connection";

      const hostScope = el("host-scope")?.value || "private";
      const hostTeamId = hostScope === "team" ? el("host-team").value : "";

      const data = {
        name: el("host-name").value.trim(),
        host: el("host-host").value.trim(),
        user: el("host-user").value.trim(),
        port: Number(el("host-port").value),
        driver,
        scope: hostScope,
        teamId: hostTeamId,
        auth: {
          method: el("host-auth").value,
          password: el("host-password").value || "",
        },
        sftpEnabled: sftpEnabled,
        sftp: sftpEnabled
          ? {
              enabled: true,
              credentials: sftpMode === "custom" ? "custom" : "connection",
              user: sftpMode === "custom" ? el("sftp-user").value.trim() : "",
              password: sftpMode === "custom" ? el("sftp-password").value : "",
            }
          : undefined,
        telecom:
          driver === "telecom"
            ? {
                path: el("telecom-path").value.trim(),
                protocol: el("telecom-protocol").value || "ssh",
                command: el("telecom-command").value || "",
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
        notifyWarn("Disconnect before deleting this host.");
        return;
      }

      if (!confirm(`Delete host "${editorTarget.name}"?`)) return;

      const net = config.networks.find((n) => n.id === activeNetworkId);
      const target = net.hosts.find((h) => h.id === editorTarget.id);
      if (target) target.deleted = true;
    }

    if (editorType === "network") {
      if ((editorTarget.hosts || []).some((h) => !h.deleted)) {
        notifyWarn("Delete all hosts in this network first.");
        return;
      }

      if (!confirm(`Delete network "${editorTarget.name}"?`)) return;

      editorTarget.deleted = true;

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

  /* ===================== Script editor ===================== */

  function openScriptEditor(mode, target = null) {
    scriptEditorMode = mode;
    scriptEditorTarget = target;

    el("script-title").textContent = mode === "create" ? "Add Script" : "Edit Script";
    el("script-name").value = target?.name || "";
    el("script-command").value = target?.command || "";
    el("script-description").value = target?.description || "";

    const scope = target?.scope || (activeTeamId ? "team" : "private");
    el("script-scope").value = scope;
    fillTeamSelect(el("script-team"), target?.teamId || activeTeamId || "", true);
    applyScriptScopeVisibility();
    validateScriptEditor();

    el("script-delete").classList.toggle("hidden", mode !== "edit");
    el("script-modal").classList.remove("hidden");
  }

  function closeScriptEditor() {
    el("script-modal").classList.add("hidden");
    scriptEditorMode = null;
    scriptEditorTarget = null;
  }

  function applyScriptScopeVisibility() {
    const scope = el("script-scope")?.value || "private";
    el("script-team-row")?.classList.toggle("hidden", scope !== "team");
  }

  function validateScriptEditor() {
    let ok =
      el("script-name").value.trim() &&
      el("script-command").value.trim();

    if (ok && el("script-scope")?.value === "team") {
      ok = !!el("script-team").value;
    }
    el("script-save").disabled = !ok;
  }

  ["script-name", "script-command", "script-description"].forEach((id) =>
    el(id)?.addEventListener("input", validateScriptEditor)
  );
  ["script-scope", "script-team"].forEach((id) =>
    el(id)?.addEventListener("change", () => {
      applyScriptScopeVisibility();
      validateScriptEditor();
    })
  );

  el("script-save").onclick = () => {
    const scope = el("script-scope").value || "private";
    const teamId = scope === "team" ? el("script-team").value : "";

    const data = {
      name: el("script-name").value.trim(),
      command: el("script-command").value.trim(),
      description: el("script-description").value.trim(),
      scope,
      teamId,
    };

    if (scriptEditorMode === "create") {
      config.scripts = config.scripts || [];
      config.scripts.push({
        id: "",
        ...data,
      });
    } else if (scriptEditorTarget) {
      Object.assign(scriptEditorTarget, data);
    }

    closeScriptEditor();
    saveConfig();
  };

  el("script-delete").onclick = () => {
    if (!scriptEditorTarget) return;
    if (!confirm(`Delete script "${scriptEditorTarget.name}"?`)) return;
    scriptEditorTarget.deleted = true;
    closeScriptEditor();
    saveConfig();
  };

  el("script-cancel").onclick = closeScriptEditor;

  /* ===================== Teams modal ===================== */

  let teamsPresenceTimer = null;

  function openTeamsModal() {
    teamsModalOpen = true;
    el("teams-modal").classList.remove("hidden");
    profileDirty = false;
    el("profile-name").value = config?.user?.name || "";
    el("profile-email").value = config?.user?.email || "";
    profileEditing = !isValidEmail(el("profile-email").value);
    if (!activeTeamDetailId) {
      activeTeamDetailId = (config?.teams || []).find((t) => !t.deleted)?.id || null;
    }
    refreshTeamRepoPaths();
    refreshTeamPresence();
    renderTeamsModal();
    renderProfileSection();
    if (!teamsPresenceTimer) {
      teamsPresenceTimer = setInterval(refreshTeamPresence, 4000);
    }
  }

  function closeTeamsModal() {
    teamsModalOpen = false;
    profileDirty = false;
    el("teams-modal").classList.add("hidden");
    if (teamsPresenceTimer) {
      clearInterval(teamsPresenceTimer);
      teamsPresenceTimer = null;
    }
  }

  function refreshTeamPresence() {
    rpc({ type: "teams_presence" })
      .then((res) => {
        teamPresence = res || { peers: [], user: null };
        if (teamsModalOpen) {
          renderTeamsModal();
          refreshTeamRepoPaths();
        }
      })
      .catch(() => {});
  }

  function refreshTeamRepoPaths() {
    rpc({ type: "team_repo_paths" })
      .then((res) => {
        teamRepoPaths = res.paths || {};
        if (teamsModalOpen) renderTeamsModal();
      })
      .catch(() => {});
  }

  function isMemberActive(email) {
    const norm = normalizeEmail(email);
    if (!norm) return false;
    if (norm === userEmail()) return true;
    const now = Date.now() / 1000;
    return (teamPresence.peers || []).some(
      (p) => normalizeEmail(p.email) === norm && now - (p.lastSeen || 0) < 20
    );
  }

  function renderTeamsModal() {
    if (!teamsModalOpen) return;
    renderTeamsList();
    renderTeamDetail();
    renderProfileSection();
  }

  function renderTeamsList() {
    const container = el("teams-list");
    container.innerHTML = "";
    const frag = document.createDocumentFragment();
    (config?.teams || [])
      .filter((t) => !t.deleted)
      .forEach((team) => {
        const div = document.createElement("div");
        div.className = "teams-list-item";
        if (team.id === activeTeamDetailId) div.classList.add("active");

        const activeCount = (team.members || []).filter((m) => isMemberActive(m.email)).length;
        const badge = isTeamAdmin(team) ? '<span class="team-badge">admin</span>' : "";
        div.innerHTML = `
          <div class="team-row">
            <span>${esc(team.name || "Untitled Team")}</span>
            ${badge}
          </div>
          <div class="team-member-status">${activeCount} active</div>
        `;

        div.onclick = () => {
          activeTeamDetailId = team.id;
          renderTeamsModal();
        };

        frag.appendChild(div);
      });
    container.appendChild(frag);
  }

  function renderTeamDetail() {
    const team = getTeamById(activeTeamDetailId || "");
    const isAdmin = !!team && isTeamAdmin(team);
    const isMember = !!team && isUserInTeam(team);
    const currentEmail = userEmail();
    el("team-name").value = team?.name || "";
    el("team-name-display").textContent = team?.name || "";
    el("team-id").textContent = team?.id || "";
    const repoPath = (team?.id && teamRepoPaths[team.id]) || "";
    el("team-repo-path").textContent = repoPath || "Not available";

    el("team-name").disabled = !isAdmin;
    el("team-name").readOnly = !isAdmin;
    el("team-name").classList.toggle("hidden", !isAdmin);
    el("team-name-display").classList.toggle("hidden", isAdmin);

    const requestRow = el("team-request-row");
    const requestStatus = el("team-request-status");
    const requestBtn = el("team-request-access");
    const requestsRow = el("team-requests-row");
    const requestsContainer = el("team-requests");

    if (!team) {
      requestRow?.classList.add("hidden");
      requestsRow?.classList.add("hidden");
    }

    if (requestStatus) {
      requestStatus.textContent = "";
      requestStatus.className = "team-request-status";
    }
    if (requestBtn) {
      requestBtn.classList.remove("hidden");
      requestBtn.disabled = true;
    }

    if (team && !isMember) {
      const req = findTeamRequest(team, currentEmail);
      const status = req?.status || "";
      let statusText = "";
      if (req) {
        if (status === "approved") {
          statusText = "Request approved. Click Request access to rejoin.";
        } else if (status === "declined") {
          statusText = "Request declined. You can request access again.";
        } else {
          statusText = "Request pending";
        }
      }

      if (requestStatus) {
        const hint = isValidEmail(currentEmail)
          ? "Request access to join this team."
          : "Set a valid email in your profile to request access.";
        requestStatus.textContent = statusText || hint;
        if (status) {
          requestStatus.classList.add(status);
        }
      }

      const canRequest =
        isValidEmail(currentEmail) && (!req || req.status !== "pending");
      if (requestBtn) {
        requestBtn.disabled = !canRequest;
      }

      requestRow?.classList.remove("hidden");
    } else {
      requestRow?.classList.add("hidden");
    }

    if (requestsRow) {
      if (!team || !isAdmin) {
        requestsRow.classList.add("hidden");
      } else {
        requestsRow.classList.remove("hidden");
      }
    }
    if (requestsContainer) {
      requestsContainer.innerHTML = "";
    }

    if (team && isAdmin && requestsContainer) {
      const pending = (team.requests || []).filter((r) => r.status === "pending");
      if (!pending.length) {
        const empty = document.createElement("div");
        empty.className = "team-member-status";
        empty.textContent = "No pending requests.";
        requestsContainer.appendChild(empty);
      } else {
        pending.forEach((req) => {
          const row = document.createElement("div");
          row.className = "team-request-row";

          const info = document.createElement("div");
          info.className = "team-request-info";
          const label = document.createElement("div");
          label.textContent = req.name || req.email || "Unknown";
          const meta = document.createElement("div");
          meta.className = "team-member-status";
          meta.textContent = req.email || "";
          info.appendChild(label);
          info.appendChild(meta);

          const actions = document.createElement("div");
          actions.className = "team-request-actions";

          const approve = document.createElement("button");
          approve.className = "btn small";
          approve.textContent = "Approve";
          approve.onclick = () => {
            if (!team) return;
            const now = Math.floor(Date.now() / 1000);
            const target = (team.requests || []).find((r) => r.id === req.id);
            if (target) {
              target.status = "approved";
              target.resolvedAt = now;
              target.resolvedBy = currentEmail || "";
            }
            team.members = team.members || [];
            if (!team.members.some((m) => normalizeEmail(m.email) === normalizeEmail(req.email))) {
              team.members.push({
                email: req.email,
                name: req.name || "",
                role: "user",
              });
            }
            saveConfig();
          };

          const decline = document.createElement("button");
          decline.className = "btn small secondary";
          decline.textContent = "Decline";
          decline.onclick = () => {
            if (!team) return;
            const now = Math.floor(Date.now() / 1000);
            const target = (team.requests || []).find((r) => r.id === req.id);
            if (target) {
              target.status = "declined";
              target.resolvedAt = now;
              target.resolvedBy = currentEmail || "";
            }
            saveConfig();
          };

          actions.appendChild(approve);
          actions.appendChild(decline);
          row.appendChild(info);
          row.appendChild(actions);
          requestsContainer.appendChild(row);
        });
      }
    }

    const members = el("team-members");
    members.innerHTML = "";
    (team?.members || []).forEach((member) => {
      const row = document.createElement("div");
      row.className = "team-member-row";
      const status = isMemberActive(member.email) ? "active" : "offline";
      const info = document.createElement("div");
      info.innerHTML = `
        <div>${esc(member.name || member.email || "Unknown")}</div>
        <div class="team-member-status ${status}">${status}</div>
      `;
      row.appendChild(info);

      const roleWrap = document.createElement("div");
      if (isAdmin) {
        const select = document.createElement("select");
        select.className = "btn small secondary";
        const optAdmin = document.createElement("option");
        optAdmin.value = "admin";
        optAdmin.textContent = "admin";
        const optUser = document.createElement("option");
        optUser.value = "user";
        optUser.textContent = "user";
        select.appendChild(optAdmin);
        select.appendChild(optUser);
        select.value = member.role || "user";
        select.onchange = () => {
          if (!team) return;
          if (select.value !== "admin") {
            const adminCount = (team.members || []).filter(
              (m) => m.role === "admin"
            ).length;
            if (adminCount <= 1 && member.role === "admin") {
              notifyWarn("Each team must have at least one admin.");
              select.value = "admin";
              return;
            }
          }
          member.role = select.value;
          saveConfig();
        };
        roleWrap.appendChild(select);
      } else {
        const label = document.createElement("div");
        label.className = "team-member-status";
        label.textContent = member.role || "user";
        roleWrap.appendChild(label);
      }
      row.appendChild(roleWrap);

      const remove = document.createElement("button");
      remove.className = "btn small secondary";
      remove.textContent = "Remove";
      remove.disabled = !isAdmin;
      remove.classList.toggle("hidden", !isAdmin);
      remove.onclick = () => {
        if (!team || !isAdmin) return;
        if (member.role === "admin") {
          const adminCount = (team.members || []).filter(
            (m) => m.role === "admin"
          ).length;
          if (adminCount <= 1) {
            notifyWarn("Each team must have at least one admin.");
            return;
          }
        }
        team.members = (team.members || []).filter(
          (m) => normalizeEmail(m.email) !== normalizeEmail(member.email)
        );
        saveConfig();
      };
      row.appendChild(remove);
      members.appendChild(row);
    });

    const addMemberRow = el("team-add-member")?.closest(".team-add-member");
    if (addMemberRow) addMemberRow.classList.toggle("hidden", !isAdmin);

    el("team-save").disabled = !team || !isAdmin;
    el("team-delete").disabled = !team || !isAdmin;
    el("team-add-member").disabled = !team || !isAdmin;
    el("team-save").classList.toggle("hidden", !isAdmin);
    el("team-delete").classList.toggle("hidden", !isAdmin);
    el("team-copy-path").disabled = !repoPath;

    const leaveBtn = el("team-leave");
    if (leaveBtn) {
      const isMemberBtn = !!team && isMember;
      leaveBtn.classList.toggle("hidden", !isMemberBtn);
      if (isMemberBtn) {
        leaveBtn.disabled = false;
      }
    }
  }

  function renderProfileSection() {
    const name = config?.user?.name || "";
    const email = config?.user?.email || "";
    const hasValidEmail = isValidEmail(email);
    const showDisplay = hasValidEmail && !profileEditing;

    el("profile-name-text").textContent = name || "—";
    el("profile-email-text").textContent = email || "—";

    el("profile-display")?.classList.toggle("hidden", !showDisplay);
    el("profile-form")?.classList.toggle("hidden", showDisplay);
    el("profile-label")?.classList.toggle("hidden", showDisplay);

    const emailInput = el("profile-email");
    if (emailInput) {
      emailInput.classList.toggle("invalid", emailInput.value && !isValidEmail(emailInput.value));
    }

    const saveBtn = el("profile-save");
    if (saveBtn) {
      saveBtn.disabled = !isValidEmail(emailInput?.value || "");
    }
  }

  el("teams-close").onclick = closeTeamsModal;
  el("btn-teams").onclick = openTeamsModal;
  el("team-copy-path").onclick = () => {
    const path = el("team-repo-path").textContent || "";
    if (path && path !== "Not available") {
      writeClipboardText(path).catch(() => {});
    }
  };

  el("profile-save").onclick = () => {
    if (!isValidEmail(el("profile-email").value)) {
      notifyWarn("Enter a valid email address.");
      return;
    }
    config.user = config.user || {};
    config.user.name = el("profile-name").value.trim();
    config.user.email = el("profile-email").value.trim();
    profileDirty = false;
    profileEditing = false;
    saveConfig();
    renderProfileSection();
  };

  ["profile-name", "profile-email"].forEach((id) =>
    el(id)?.addEventListener("input", () => {
      profileDirty = true;
      renderProfileSection();
    })
  );

  el("profile-edit").onclick = () => {
    profileEditing = true;
    renderProfileSection();
  };

  el("btn-team-create").onclick = () => {
    if (!userEmail()) {
      notifyWarn("Set your profile email before creating a team.");
      return;
    }
    const name = prompt("Team name?");
    if (!name) return;
    const members = [];
    const email = userEmail();
    if (email) {
      members.push({ email, name: config?.user?.name || "", role: "admin" });
    }
    config.teams = config.teams || [];
    config.teams.push({ id: "", name: name.trim(), members });
    pendingTeamName = name.trim();
    saveConfig();
  };

  el("team-save").onclick = () => {
    const team = getTeamById(activeTeamDetailId || "");
    if (!team || !isTeamAdmin(team)) return;
    team.name = el("team-name").value.trim();
    saveConfig();
  };

  el("team-delete").onclick = () => {
    const team = getTeamById(activeTeamDetailId || "");
    if (!team || !isTeamAdmin(team)) return;
    if (!confirm(`Delete team "${team.name}"?`)) return;
    team.deleted = true;
    activeTeamDetailId = null;
    saveConfig();
  };

  el("team-add-member").onclick = () => {
    const team = getTeamById(activeTeamDetailId || "");
    if (!team || !isTeamAdmin(team)) return;
    const name = el("team-member-name").value.trim();
    const email = el("team-member-email").value.trim();
    if (!email) return;
    if ((team.members || []).some((m) => normalizeEmail(m.email) === normalizeEmail(email))) {
      return;
    }
    team.members = team.members || [];
    team.members.push({ name, email, role: "user" });
    el("team-member-name").value = "";
    el("team-member-email").value = "";
    saveConfig();
  };

  el("team-leave").onclick = async () => {
    const team = getTeamById(activeTeamDetailId || "");
    if (!team || !isUserInTeam(team)) return;
    if (isTeamAdmin(team)) {
      const adminCount = (team.members || []).filter(
        (m) => m.role === "admin"
      ).length;
      if (adminCount <= 1) {
        const ok = await confirmDialog(
          `You are the last admin. Leaving will delete "${team.name || "team"}". Continue?`,
          { okText: "Delete team", danger: true }
        );
        if (!ok) {
          return;
        }
        team.deleted = true;
        activeTeamDetailId = null;
        saveConfig();
        return;
      }
    }
    const ok = await confirmDialog(`Leave team "${team.name || "team"}"?`, {
      okText: "Leave",
      danger: true,
    });
    if (!ok) return;
    team.members = (team.members || []).filter(
      (m) => normalizeEmail(m.email) !== userEmail()
    );
    if (team.requests) {
      team.requests = team.requests.filter(
        (r) => normalizeEmail(r.email) !== userEmail()
      );
    }
    saveConfig();
  };

  el("team-request-access").onclick = () => {
    const team = getTeamById(activeTeamDetailId || "");
    if (!team || isUserInTeam(team)) return;
    const email = userEmail();
    if (!isValidEmail(email)) {
      notifyWarn("Set a valid email in your profile before requesting access.");
      return;
    }
    team.requests = team.requests || [];
    const existing = findTeamRequest(team, email);
    const now = Math.floor(Date.now() / 1000);
    if (existing) {
      if (existing.status === "pending") {
        return;
      }
      existing.status = "pending";
      existing.requestedAt = now;
      existing.resolvedAt = 0;
      existing.resolvedBy = "";
      existing.name = config?.user?.name || existing.name || "";
    } else {
      team.requests.push({
        id: newLocalId(),
        email: email,
        name: config?.user?.name || "",
        status: "pending",
        requestedAt: now,
      });
    }
    saveConfig();
  };

  el("confirm-cancel").onclick = () => closeConfirm(false);
  el("confirm-ok").onclick = () => closeConfirm(true);

  /* ===================== Config ===================== */

  function saveConfig() {
    rpc({ type: "config_save", config })
      .then(loadConfig)
      .catch((e) => notifyError(e.detail || e.error || "Failed to save config"));
  }

  function loadConfig() {
    rpc({ type: "config_get" })
      .then((res) => {
        config = res.config;
        checkRequestNotifications(config);
        if (pendingTeamName) {
          const found = (config.teams || []).find(
            (t) => (t.name || "") === pendingTeamName
          );
          if (found) activeTeamDetailId = found.id;
          pendingTeamName = null;
        }
        renderTeamSelect();
        renderNetworks();
        renderHosts();
        renderScripts();
        if (teamsModalOpen) renderTeamsModal();
      })
      .catch((e) =>
        notifyError("Failed to load config: " + (e.detail || e.error))
      );
  }

  window.__applyConfig = (cfg) => {
    config = cfg;
    checkRequestNotifications(config);
    renderTeamSelect();
    renderNetworks();
    renderHosts();
    renderScripts();
    if (teamsModalOpen) renderTeamsModal();
  };

  function resetTerminals() {
    hostTerminals.forEach((state) => {
      try {
        state?.tabs?.forEach?.((entry) => {
          try {
            entry.term?.dispose?.();
          } catch {}
          try {
            entry.pane?.remove?.();
          } catch {}
        });
      } catch {}
    });
    hostTerminals.clear();

    hostFiles.forEach((entry) => {
      try {
        entry.pane?.remove?.();
      } catch {
        // ignore
      }
    });
    hostFiles.clear();
    hostTabs.clear();

    activeHostId = null;
    activeTermTabId = null;
    activeState = "disconnected";
    activeHostHasSFTP = false;
    activeTab = "terminal";
    activateTerminalForHostTab(null, null);
    el("title").textContent = "Select a host";
    updateStatus(null);
    setActiveTab("terminal");
  }

  /* ===================== Bind UI ===================== */

  function updateTerminalActions() {
    const hasTerm = !!term;
    const hasHost = !!activeHostId;
    const hasTab = !!activeTermTabId;
    const isConnected = activeState === "connected";
    const isTerminalTab = activeTab === "terminal";

    el("btn-disconnect").disabled = !hasHost || !hasTab;
    el("btn-new-term-tab").disabled = !hasHost || !isTerminalTab;
    el("btn-copy").disabled = !hasTerm || !isTerminalTab;
    el("btn-clear").disabled = !hasTerm || !isTerminalTab;
    el("btn-paste").disabled = !hasTerm || !isConnected || !isTerminalTab;
    el("term-search").disabled = !hasTerm || !isTerminalTab;
    el("btn-find-prev").disabled = !hasTerm || !isTerminalTab;
    el("btn-find-next").disabled = !hasTerm || !isTerminalTab;
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
    const teamSel = el("team-select");

    if (teamSel) {
      teamSel.onchange = (e) => {
        activeTeamId = e.target.value || "";
        activeNetworkId = null;
        activeHostId = null;
        activeTermTabId = null;
        activeState = "disconnected";
        activeHostHasSFTP = false;
        activateTerminalForHostTab(null, null);
        setActiveTab("terminal");
        el("title").textContent = "Select a host";
        renderTeamSelect();
        renderNetworks();
        renderHosts();
        renderScripts();
        updateStatus(null);
      };
    }

    sel.onchange = (e) => {
      activeNetworkId = Number(e.target.value) || null;
      activeHostId = null;
      activeTermTabId = null;
      activeState = "disconnected";
      activeHostHasSFTP = false;
      activateTerminalForHostTab(null, null);
      setActiveTab("terminal");
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
        notifyWarn("Select a network first.");
        return;
      }
      openEditor("host", "create");
    };

    el("btn-add-script").onclick = () => openScriptEditor("create");

    el("btn-export").onclick = () =>
      rpc({ type: "config_export" }).then((r) =>
        notifySuccess(`Config exported to:\n${r.path}`)
      );

    el("btn-import").onclick = async () => {
      if (
        !confirm(
          "Import will overwrite your current config.\nA backup will be created automatically.\n\nContinue?"
        )
      )
        return;

      try {
        const r = await rpc({ type: "config_import_pick" });
        if (r.canceled) return;

        resetTerminals();
        config = r.config;
        renderTeamSelect();
        renderNetworks();
        renderHosts();
        renderScripts();

        const msg = [
          `Imported:\n${r.importPath || ""}`.trim(),
          r.backupPath ? `\nBackup:\n${r.backupPath}` : "",
        ].join("");
        if (msg.trim()) notifyInfo(msg.trim(), { ttl: 7000 });
      } catch (e) {
        notifyError(e.detail || e.error || "Import failed");
      }
    };

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
    el("btn-new-term-tab").onclick = () => {
      if (!activeHostId) return;
      const host = findHostById(activeHostId);
      if (host) addTerminalTab(host);
    };

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

    el("tab-terminal").onclick = () => setActiveTab("terminal");
    el("tab-files").onclick = () => setActiveTab("files");

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
    el("host-menu-duplicate").onclick = () => {
      const h = hostMenuTarget;
      hideHostMenu();
      if (h) duplicateHostInConfig(h);
    };
    el("host-menu-delete").onclick = () => {
      const h = hostMenuTarget;
      hideHostMenu();
      if (h) deleteHostFromConfig(h);
    };

    // Files (SFTP) context menu
    el("file-menu-open").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path || !t.isDir) return;
      navigateTo(t.hostId, t.path).catch((e) =>
        notifyError(e.detail || e.error || "Open failed")
      );
    };
    el("file-menu-edit").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path || t.isDir) return;
      openFileEditor(t.hostId, t.path).catch((e) =>
        notifyError(e.detail || e.error || "Edit failed")
      );
    };
    el("file-menu-download").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path || t.isDir) return;
      const entry = ensureFilePane(t.hostId);
      entry.selectedPath = t.path;
      downloadSelected(t.hostId).catch((e) =>
        notifyError(e.detail || e.error || "Download failed")
      );
    };
    el("file-menu-upload").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      const hostId = t?.hostId || activeHostId;
      if (!hostId) return;
      ensureFilePane(hostId).fileInput.click();
    };
    el("file-menu-mkdir").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      const hostId = t?.hostId || activeHostId;
      if (!hostId) return;
      createFolder(hostId).catch((e) =>
        notifyError(e.detail || e.error || "Create folder failed")
      );
    };
    el("file-menu-rename").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path) return;
      const entry = ensureFilePane(t.hostId);
      entry.selectedPath = t.path;
      renameSelected(t.hostId).catch((e) =>
        notifyError(e.detail || e.error || "Rename failed")
      );
    };
    el("file-menu-delete").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path) return;
      const entry = ensureFilePane(t.hostId);
      entry.selectedPath = t.path;
      deleteSelected(t.hostId).catch((e) =>
        notifyError(e.detail || e.error || "Delete failed")
      );
    };
    el("file-menu-copy-path").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.path) return;
      writeClipboardText(t.path).catch(() => {});
    };
    el("file-menu-refresh").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      const hostId = t?.hostId || activeHostId;
      if (!hostId) return;
      refreshFiles(hostId).catch(() => {});
    };

    document.addEventListener("click", () => hideAllMenus());
    document.addEventListener("contextmenu", () => hideAllMenus());
    window.addEventListener("blur", () => hideAllMenus());

    setupFileEditorModal();

    // Telecom path picker (prefer native dialog so we get a real absolute path)
    el("telecom-browse").onclick = async () => {
      try {
        const r = await rpc({ type: "telecom_pick" });
        if (r.path) {
          el("telecom-path").value = r.path;
          validateEditor();
          return;
        }
      } catch {
        // fall back to file input
      }

      el("telecom-path-picker").click();
    };

    // Fallback: in some WebViews File objects expose .path
    el("telecom-path-picker").addEventListener("change", () => {
      const f = el("telecom-path-picker").files?.[0];
      const p = f?.path || "";
      if (p) el("telecom-path").value = p;
      validateEditor();
    });

    // About modal: close on background click / escape
    el("about-modal").addEventListener("click", (e) => {
      if (e.target === el("about-modal")) hideAbout();
    });
    el("teams-modal").addEventListener("click", (e) => {
      if (e.target === el("teams-modal")) closeTeamsModal();
    });
    el("script-modal").addEventListener("click", (e) => {
      if (e.target === el("script-modal")) closeScriptEditor();
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        hideAbout();
        closeTeamsModal();
        closeScriptEditor();
      }
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    bindUI();
    loadConfig();
    updateTerminalActions();
  });
})();
