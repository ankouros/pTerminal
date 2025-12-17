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
  let activeTab = "terminal"; // terminal | files
  let activeHostHasSFTP = false;

  const hostTerms = new Map(); // hostId -> { pane, term, fitAddon, clipboardAddon, searchAddon, ... }
  let activePane = null;

  const hostFiles = new Map(); // hostId -> { pane, cwd, selectedPath, entries, fileInput }
  const hostTabs = new Map(); // hostId -> "terminal" | "files"

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

  function scheduleOutputFlush(entry) {
    if (!entry || entry.outputFlushScheduled) return;
    entry.outputFlushScheduled = true;

    setTimeout(() => {
      entry.outputFlushScheduled = false;
      const q = entry.outputQueue;
      if (!q || q.length === 0) return;

      const parts = new Array(q.length);
      for (let i = 0; i < q.length; i++) parts[i] = b64dec(q[i]);
      q.length = 0;

      entry.term.write(parts.join(""));
    }, 0);
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
      outputQueue: [],
      outputFlushScheduled: false,
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

    if (tab === "files" && activeHostId) {
      ensureFilePane(activeHostId);
      refreshFiles(activeHostId).catch(() => {});
    } else if (tab === "terminal") {
      requestAnimationFrame(() => fitAddon?.fit?.());
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
            alert(e.detail || e.error || "Failed to trust host");
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
      entries: [],
      searchQuery: "",
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
      renderFileList(entry);
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

    const q = (entry.searchQuery || "").trim().toLowerCase();
    const items = q
      ? entry.entries.filter((e) => (e.name || "").toLowerCase().includes(q))
      : entry.entries;

    items.forEach((it) => {
      const tr = document.createElement("tr");
      tr.className = "file-row";
      tr.draggable = true;
      tr.dataset.path = it.path;
      tr.dataset.isdir = it.isDir ? "1" : "0";

      if (it.path === entry.selectedPath) tr.classList.add("selected");

      tr.innerHTML = `
        <td>${esc(it.isDir ? it.name + "/" : it.name)}</td>
        <td class="file-muted">${it.isDir ? "" : esc(formatBytes(it.size))}</td>
        <td class="file-muted">${esc(formatTime(it.modUnix))}</td>
      `;

      tr.addEventListener("click", () => {
        entry.selectedPath = it.path;
        renderFileList(entry);
      });

      tr.addEventListener("contextmenu", (e) => {
        entry.selectedPath = it.path;
        renderFileList(entry);
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
          entry.selectedPath = it.path;
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
          .catch((err) => alert(err.detail || err.error || "Move failed"));
      });

      tbody.appendChild(tr);
    });
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

    // If selection not in listing, clear it.
    if (entry.selectedPath && !entry.entries.some((e) => e.path === entry.selectedPath)) {
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
    if (r.localPath) alert(`Downloaded to:\n${r.localPath}`);
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
          alert(e.detail || e.error || "Save failed");
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
    const text = b64dec(r.dataB64 || "");

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
      alert("Disconnect before deleting this host.");
      return;
    }
    if (!confirm(`Delete host "${host.name}"?`)) return;

    const net = config.networks.find((n) => n.id === activeNetworkId);
    if (!net) return;
    net.hosts = net.hosts.filter((h) => h.id !== host.id);
    saveConfig();
  }

  function duplicateHostInConfig(host) {
    if (!host) return;
    const net = config.networks.find((n) => n.id === activeNetworkId);
    if (!net) return;

    const clone = JSON.parse(JSON.stringify(host));
    clone.id = nextHostId();

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

      const sftpOn = !!(h.sftp?.enabled || h.sftpEnabled);
      const sftpTag = sftpOn ? " · sftp" : "";

      div.innerHTML = `
        <div class="node-name">${esc(h.name)}</div>
        <div class="node-meta">
          ${esc(h.user)}@${esc(h.host)}:${esc(h.port ?? 22)}
          · ${esc(h.driver || "ssh")}
          · ${esc(h.auth?.method || "password")}
          ${sftpTag}
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
      updateTabsForActiveHost(host);
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
          await showTrustDialogAsync(host.id, err.hostPort, err.fingerprint);
          return await connectHost(host);
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

      // Keep file UI state but force terminal tab (SFTP session is closed on backend).
      setActiveTab("terminal");
    } catch (e) {
      alert(e.detail || e.error || "Failed to disconnect");
    }
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

    // Password field (ssh + telecom)
    el("host-password").value = auth.password || "";

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
    applySFTPVisibility();

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
    applySFTPVisibility();
    validateEditor();
  }

  el("host-driver").addEventListener("change", applyHostDriverVisibility);

  function applySFTPVisibility() {
    const enabled = !!el("sftp-enabled")?.checked;
    el("sftp-cred-group")?.classList.toggle("hidden", !enabled);

    const mode = el("sftp-cred-mode")?.value || "connection";
    const showCustom = enabled && mode === "custom";
    el("sftp-custom-row")?.classList.toggle("hidden", !showCustom);
  }

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
        (driver !== "telecom" ||
          (el("telecom-path").value.trim() && el("telecom-protocol").value));

      if (ok && el("sftp-enabled")?.checked) {
        const mode = el("sftp-cred-mode")?.value || "connection";
        if (mode === "custom") {
          ok = !!el("sftp-user").value.trim() && !!el("sftp-password").value;
        }
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
    "telecom-path",
    "telecom-protocol",
    "telecom-command",
    "sftp-user",
    "sftp-password",
  ].forEach((id) => el(id)?.addEventListener("input", validateEditor));

  ["sftp-enabled", "sftp-cred-mode"].forEach((id) =>
    el(id)?.addEventListener("change", () => {
      applySFTPVisibility();
      validateEditor();
    })
  );

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

      const sftpEnabled = !!el("sftp-enabled").checked;
      const sftpMode = el("sftp-cred-mode").value || "connection";

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

  function resetTerminals() {
    hostTerms.forEach((entry) => {
      try {
        entry.term?.dispose?.();
      } catch {
        // ignore
      }
      try {
        entry.pane?.remove?.();
      } catch {
        // ignore
      }
    });
    hostTerms.clear();

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
    activeState = "disconnected";
    activeHostHasSFTP = false;
    activeTab = "terminal";
    activateTerminalForHost(null);
    el("title").textContent = "Select a host";
    updateStatus(null);
    setActiveTab("terminal");
  }

  /* ===================== Bind UI ===================== */

  function updateTerminalActions() {
    const hasTerm = !!term;
    const hasHost = !!activeHostId;
    const isConnected = activeState === "connected";
    const isTerminalTab = activeTab === "terminal";

    el("btn-disconnect").disabled = !hasHost;
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

    sel.onchange = (e) => {
      activeNetworkId = Number(e.target.value) || null;
      activeHostId = null;
      activeState = "disconnected";
      activeHostHasSFTP = false;
      activateTerminalForHost(null);
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
        alert("Select a network first.");
        return;
      }
      openEditor("host", "create");
    };

    el("btn-export").onclick = () =>
      rpc({ type: "config_export" }).then((r) =>
        alert(`Config exported to:\n${r.path}`)
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
        renderNetworks();
        renderHosts();

        const msg = [
          `Imported:\n${r.importPath || ""}`.trim(),
          r.backupPath ? `\nBackup:\n${r.backupPath}` : "",
        ].join("");
        if (msg.trim()) alert(msg.trim());
      } catch (e) {
        alert(e.detail || e.error || "Import failed");
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
        alert(e.detail || e.error || "Open failed")
      );
    };
    el("file-menu-edit").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path || t.isDir) return;
      openFileEditor(t.hostId, t.path).catch((e) =>
        alert(e.detail || e.error || "Edit failed")
      );
    };
    el("file-menu-download").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path || t.isDir) return;
      const entry = ensureFilePane(t.hostId);
      entry.selectedPath = t.path;
      downloadSelected(t.hostId).catch((e) =>
        alert(e.detail || e.error || "Download failed")
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
        alert(e.detail || e.error || "Create folder failed")
      );
    };
    el("file-menu-rename").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path) return;
      const entry = ensureFilePane(t.hostId);
      entry.selectedPath = t.path;
      renameSelected(t.hostId).catch((e) =>
        alert(e.detail || e.error || "Rename failed")
      );
    };
    el("file-menu-delete").onclick = () => {
      const t = fileMenuTarget;
      hideFileMenu();
      if (!t?.hostId || !t.path) return;
      const entry = ensureFilePane(t.hostId);
      entry.selectedPath = t.path;
      deleteSelected(t.hostId).catch((e) =>
        alert(e.detail || e.error || "Delete failed")
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
