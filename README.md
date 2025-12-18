# pTerminal (Go)

A lightweight Linux GUI app providing **persistent multi-node SSH terminal sessions** with a modern HTML/JS UI rendered via WebView.

## Status

- ✅ WebView window using `github.com/webview/webview_go` with inlined HTML/CSS/JS via `go:embed`
- ✅ Embedded **xterm.js** terminal with binary-safe streaming between Go and JS (base64 PTY)
- ✅ Native Go SSH sessions using `golang.org/x/crypto/ssh`, one persistent session per host + auto-reconnect
- ✅ Optional driver (PTY local process) for hosts that require it
- ✅ SFTP support (per-host enable + credential mode) with a **tabbed Files view**
- ✅ SFTP Files view: search, context menu actions, drag & drop upload, download to `~/Downloads`, in-place edit/save
- ✅ JSON configuration (`~/.config/pterminal/pterminal.json`) editable from the UI + Import/Export
- ✅ SSH host key verification + trust UX for unknown/mismatched keys
- ⚙️ Planned: SSH key + agent flows in UI, richer SFTP (recursive delete, permissions, etc.)

## Goals & Constraints

- Go + WebView UI (our UI is not built with GTK/Qt/Electron/Node/Python; on Linux `webview_go` uses **WebKitGTK/GTK** under the hood, which we ship via Flatpak and/or bundle best-effort for portable builds)
- Terminal emulator: **xterm.js** (assets shipped and embedded into the binary)
- SSH: `golang.org/x/crypto/ssh` (no external `ssh` binary)
- Single self-contained binary (HTML/CSS/JS embedded via `go:embed`)
- One persistent SSH session per host; switching hosts does not reconnect

## Getting started

### 0) Prereqs (Linux)

- Go 1.22+
- System libraries required by `webview_go` (WebKitGTK) and the native file picker (GTK3).
  - Package names vary by distro (example on Debian/Ubuntu: `libwebkit2gtk-4.1-dev`, `libgtk-3-dev`, `pkg-config`).

### 1) Fetch/update frontend vendor assets (xterm.js)

Xterm assets live under `internal/ui/assets/vendor/` and are embedded into the Go binary. To update them, you need `node` + `npm`:

```bash
make assets
```

This downloads `xterm.js`, `xterm.css`, and the required addons into `internal/ui/assets/vendor/`.

### 2) Build & run

```bash
make build
./bin/pterminal
```

If your distro uses an environment *module system* that ships Go with a **read-only** default module/cache path (common on SLES enterprise setups), our Makefile always forces writable caches under:

- `$XDG_CACHE_HOME/pterminal` (if set), otherwise `$HOME/.cache/pterminal`

No user exports are needed; just run the Makefile targets.

Or, during development:

```bash
make run
```

### 3) Configuration

On first start, pTerminal will create a default configuration at:

- `~/.config/pterminal/pterminal.json`

The file contains a list of **networks** and **hosts**. You can edit it either:

- Directly in the file (JSON), or
- Via the in-app “Add network” / “Add host” editor, which calls the `config_get` / `config_save` RPCs.

A minimal example:

```json
{
  "version": 1,
  "networks": [
    {
      "id": 1,
      "name": "Default",
      "hosts": [
        {
          "id": 1,
          "name": "example",
          "host": "192.168.11.90",
          "port": 22,
          "user": "root",
          "driver": "ssh",
          "auth": {
            "method": "password"
          },
          "hostKey": {
            "mode": "known_hosts"
          },
          "sftp": {
            "enabled": true,
            "credentials": "connection"
          }
        }
      ]
    }
  ]
}
```

### 4) Terminal + Files tabs

- Hosts with SFTP enabled show `Terminal` / `Files` tabs.
- The Files tab provides a lightweight SFTP file manager:
  - Search within current directory
  - Right-click context menu (file/folder aware)
  - Drag & drop upload into the file list
  - Download selected file to `~/Downloads`
  - In-place edit/save for text files (Ctrl+S)

### 4) Authentication & host keys

- **Auth methods** (model-level):
  - `password`
  - `key` (wired in config model, UI wiring planned)
  - `agent` (wired in config model and ssh client)
- **Host key modes**:
  - `known_hosts` – strict checking against `~/.ssh/known_hosts`
  - `insecure` – skip host key verification (for prototyping only)

The UI exposes a confirmation dialog for unknown / changed host keys and can persist a trusted key via the Go backend.

## Import / Export

- Export writes a timestamped config JSON into `~/Downloads`.
- Import lets you pick a JSON file and overwrites the active config:
  - A backup of the previous config is created in `~/.config/pterminal/`
  - All existing sessions are disconnected (host IDs may change during normalization)

## Releases (no root)

Because pTerminal uses WebView (WebKitGTK) + GTK3, a plain Linux binary may require system runtime libraries.

- `make release` produces a small bundle and a dependency checker.
- `make portable` produces a larger `portable/` folder that tries to bundle shared libs next to the executable and run with `LD_LIBRARY_PATH` (best-effort, Linux-only).
- GitHub Releases: see `.github/workflows/release-portable.yml` for portable tarballs built for Ubuntu 24.04 and openSUSE Leap 15.6 (and SLES12 SP5 if SCC credentials are configured in repo secrets).
- Flatpak (recommended for “out-of-the-box” Linux): `make flatpak` and `.github/workflows/flatpak.yml`.

## Development

- `make assets` – fetch/update xterm.js assets into `internal/ui/assets/vendor/`
- `make build` – build the `pterminal` binary into `bin/`
- `make run` – build and run the app
- `make release` – build an optimized binary bundle into `release/` (includes `.desktop` + icon + helper scripts)
- `make portable` – build a best-effort “portable folder” into `release/` (bundled shared libs next to the executable)
- `make flatpak` – build `dist/pterminal.flatpak` using `flatpak-builder` (Linux)
- `make fmt` – format Go code
- `make vet` – run `go vet`

The hard constraints for this project are:

- No embedding or shelling out to native terminals
- No X11 `-into` embedding tricks
- No dependency on GUI frameworks as a *system-installed requirement* (Flatpak/portable releases may bundle WebKitGTK/GTK runtime)
- No tmux dependency (optional integration later)
