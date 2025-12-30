# pTerminal (Go)

Persistent multi-node SSH terminals + SFTP in a single Linux WebView app written entirely in Go.

## Contents

- [Features](#features)
- [Architecture & constraints](#architecture--constraints)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Terminal & Files tabs](#terminal--files-tabs)
- [Authentication & host keys](#authentication--host-keys)
- [Import / export](#import--export)
- [Releases & distribution](#releases--distribution)
- [Development workflow](#development-workflow)
- [LAN smoke test](#lan-smoke-test)
- [Troubleshooting & tips](#troubleshooting--tips)

## Features

- Modern HTML/CSS/JS UI rendered inside `github.com/webview/webview_go` with assets embedded via `go:embed`.
- Embedded **xterm.js** terminal per host with binary-safe base64 streaming between Go and JS.
- Persistent **native Go SSH** sessions (`golang.org/x/crypto/ssh`) with reconnect logic; no external `ssh` binary.
- Optional IOshell driver (local PTY) for telecom hosts that demand it, still rendered in xterm.js.
- Built-in **SFTP file manager** (Files tab) with search, context menu, drag & drop upload, download to `~/Downloads`, and inline edit/save.
- JSON config stored in `~/.config/pterminal/pterminal.json`, editable via UI or text editor; import/export helpers included.
- Host key verification UX (unknown/mismatched dialog, trust storage) and per-host auth method selection.
- Ships as a single binary with embedded assets plus Flatpak/portable bundles when needed; no external GUI frameworks required.

## Architecture & constraints

- Go backend orchestrates SSH, sessions, config, and RPC bridge; UI lives entirely inside WebView.
- Linux WebView runtime is **WebKitGTK/GTK** (bundled via Flatpak/portable release when system copy is unavailable).
- Terminal emulator: **xterm.js** only (no native terminals, tmux, or X11 `-into` tricks).
- SSH implementation: `golang.org/x/crypto/ssh` with custom session persistence; PTY data is base64 encoded both ways.
- SFTP backend is native Go + UI tab (no external `sftp` process).
- Node/npm usage is limited to `make assets` (fetching xterm vendor files under `internal/ui/assets/vendor/`).

## Quick start

### 0. Prerequisites (Linux)

- Go 1.22+
- System packages for `webview_go` / WebKitGTK + GTK3 (example on Debian/Ubuntu: `libwebkit2gtk-4.1-dev libgtk-3-dev pkg-config`).
- Optional: `node` + `npm` when refreshing xterm.js assets.

### 1. Fetch/update frontend vendor assets

```bash
make assets
```

Downloads `xterm.js`, CSS, and addons into `internal/ui/assets/vendor/` where they are embedded.

### 1.5. Install dependencies (optional)

```bash
sudo ./INSTALL.sh
```

Runs `apt` against the official Ubuntu/Debian repositories to install Go, GTK/WebKit, Node/npm, and git, then tests, cleans, builds, and publishes a desktop icon so everything is ready to run.

### 2. Build & run

```bash
make build
./bin/pterminal
# or
make run
```

On Linux the app also creates a system tray icon next to the clock; use it to show/hide the window or exit with confirmation.
- Closing the main window merely hides it to the tray so the icon and its menu stay available for reopening or exiting.

Run `./bin/pterminal --version` (or `pterminal --version`) to see the embedded version, git commit, and build timestamp before launching the UI.

The Makefile automatically redirects Go module/cache paths to `$XDG_CACHE_HOME/pterminal` (or `$HOME/.cache/pterminal`) so enterprise module systems with read-only defaults still work. Missing `pkg-config` `.pc` files are shimmed in the same cache if your distro leaves gaps (common on SLES).

### 3. Configuration

On first launch a config file is created at `~/.config/pterminal/pterminal.json`. Use the UI editors or edit the JSON manually, then reload via the UI.

Minimal example:

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

## Terminal & Files tabs

- Hosts with `sftp.enabled` show `Terminal` and `Files` tabs; other hosts show only Terminal.
- Files tab behaves like a lightweight two-pane manager:
  - Search within the current directory
  - Right-click context menu with file/folder-aware actions
  - Drag & drop upload into the listing
  - Download the selection to `~/Downloads`
  - Inline text editor with Ctrl+S save
- Per-host tab state (cwd, search, selection) persists while the app runs so switching hosts does not reset your view.

## Authentication & host keys

- Auth methods defined in config: `password`, `key`, `agent`, `keyboard-interactive`.
- Host key modes: `known_hosts` (strict check) or `insecure` (development/testing only).
- Unknown/changed host keys trigger a dialog in the UI; trusted keys are persisted via the Go backend.
- SSH/SFTP passwords and key passphrases are kept in memory only and are not persisted to disk.
- Acceptance coverage lives inside `internal/sshclient/sshclient_auth_acceptance_test.go`, which validates password, key, SSH agent, and keyboard-interactive logins on every `go test` sweep so regressions in the authentication stack are caught early.

## Import / export

- **Export** writes a timestamped config JSON into `~/Downloads`.
- **Import** lets you choose a JSON file and replaces the current config:
  - A backup of the previous file is written to `~/.config/pterminal/`.
  - Active sessions disconnect because host IDs may change during normalization.

## Releases & distribution

- `make release` builds an optimized bundle into `release/` (binary, desktop file, icon, helper scripts, dependency probe).
- `make portable` assembles a larger `release/portable/` folder with best-effort bundled shared libs + `LD_LIBRARY_PATH` launcher (Linux-only).
- Flatpak: `make flatpak` uses `flatpak-builder` (see `.github/workflows/flatpak.yml`).
- Portable tarballs for GitHub Releases are generated via `.github/workflows/release-portable.yml` (Ubuntu 24.04 base).
- Docker image: use `scripts/docker/` helpers, running with host X11/Wayland for GUI display.

If you run Docker manually on X11 and get “Authorization required” errors, pass XAUTHORITY:

```bash
xhost +local:docker
docker run --rm \
  -e DISPLAY \
  -e XAUTHORITY=/tmp/.Xauthority \
  -v /tmp/.X11-unix:/tmp/.X11-unix \
  -v "$HOME/.Xauthority:/tmp/.Xauthority:ro" \
  -v "$HOME/.config/pterminal:/home/pterminal/.config/pterminal" \
  -v "$HOME/Downloads:/home/pterminal/Downloads" \
  ankouros/pterminal:latest
```

Wayland example (recommended on GNOME/Wayland):

```bash
docker run --rm \
  -e XDG_RUNTIME_DIR=/tmp/xdg-runtime \
  -e WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-wayland-0} \
  -e DBUS_SESSION_BUS_ADDRESS=unix:path=/tmp/xdg-runtime/bus \
  -v "$XDG_RUNTIME_DIR:/tmp/xdg-runtime" \
  -v "$HOME/.config/pterminal:/home/pterminal/.config/pterminal" \
  -v "$HOME/Downloads:/home/pterminal/Downloads" \
  ankouros/pterminal:latest
```

If you run manually, pass your UID:GID so config files are not owned by root:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e DISPLAY \
  -v /tmp/.X11-unix:/tmp/.X11-unix \
  -v "$HOME/.config/pterminal:/home/pterminal/.config/pterminal" \
  -v "$HOME/Downloads:/home/pterminal/Downloads" \
  ankouros/pterminal:latest
```

## Development workflow

- `make specs-update` – init/update the `samakia-specs` submodule
- `make sync-contracts` – sync `CONTRACTS.md` from `samakia-specs`
- `make assets` – refresh xterm.js + addons in `internal/ui/assets/vendor/`
- `make build` – compile to `bin/pterminal`
- `make run` – build and run
- `make fmt` / `make vet` – gofmt + `go vet`
- `go test ./...` – run package tests (config/session/sftp have coverage)
- `go test ./internal/sshclient` – exercises the SSH auth acceptance test for password/key/agent/keyboard-interactive flows
- `make release` / `make portable` / `make flatpak` – produce distributable bundles

Before sending patches, run `make fmt` and `make vet` (mirrors CI). For UI/asset edits, ensure the `go:embed` lists in `internal/ui/assets` stay in sync.
## LAN smoke test

Quick checklist for two instances on the same LAN:

1. Launch pTerminal on two machines (or two user sessions on the same machine).
   - Ensure `PTERMINAL_P2P_SECRET` is set to the same value on both instances (required for sync).
2. In **Teams**, set your profile email on each instance (must match a member entry).
3. Create a team on instance A, add instance B's email as a member.
4. In the team dropdown, pick the new team and create a network + host with scope = Team.
5. Verify instance B sees the team, team members, and the shared network/host.
6. In the team repo folder (`~/.config/pterminal/teams/<teamId>/`), create a file and confirm it syncs to the other instance.

## Troubleshooting & tips

- **WebView fails to start**: verify WebKitGTK/GTK3 dev packages are installed; Flatpak build is the quickest way to get a known-good runtime.
- **xterm assets missing**: rerun `make assets` (requires npm) so `internal/ui/assets/vendor/` is repopulated before `make build`.
- **Config import duplicates**: host IDs must remain globally unique; duplicate IDs are rejected early. Use the UI import dialog so normalization handles re-numbering.
- **Frozen UI**: long-running SSH/SFTP actions must run outside the UI thread. Check `internal/ui` RPC handlers for blocking calls.
- **Readonly Go module cache**: rely on the Makefile-provided cache path rather than overriding env vars manually.

Questions or bugs? Open an issue with repro steps, SSH/SFTP expectations, and distro/runtime details so we can help quickly.
