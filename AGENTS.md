# pTerminal – Engineering Brief (Agent Instructions)

pTerminal is a Linux GUI application written in Go that provides persistent multi-node SSH terminals in a WebView UI.

## Locked architecture

- **Go** backend
- **WebView** UI (`github.com/webview/webview_go`) with embedded HTML/CSS/JS (`go:embed`)
- Linux WebView runtime is **WebKitGTK/GTK** (acceptable when shipped via Flatpak and/or bundled best-effort in portable releases)
- **xterm.js** as the terminal emulator (no native terminal processes)
- **Native Go SSH** (`golang.org/x/crypto/ssh`) with persistent per-host sessions
- **Binary-safe streaming** between Go and JS (base64 for PTY data)
- **Optional IOshell driver** for telecom hosts (PTY local process; still rendered in xterm.js)
- **SFTP backend** over SSH + in-app Files tab (no external `sftp` binary)

## Hard constraints (must not violate)

- Must not embed or shell out to native terminals (`xterm`, `gnome-terminal`, etc.)
- Must not use X11 `-into` embedding or similar tricks
- Must not require end users to install GUI frameworks system-wide (Flatpak/portable releases may bundle WebKitGTK/GTK runtime)
- Must not require tmux (optional later)
- Node/npm are allowed **only** for build-time asset fetching under `scripts/`

## Repository layout

- `cmd/pterminal` – main entrypoint
- `internal/app` – app bootstrap / OS checks
- `internal/ui` – WebView window, JS bridge, embedded assets and xterm integration
- `internal/ui/assets` – `index.html`, `app.css`, `app.js`, `logo.svg`, vendor xterm assets
- `internal/sshclient` – SSH dial, PTY, host key verification
- `internal/session` – session manager, reconnect/disconnect behavior
- `internal/sftpclient` – SFTP sessions + file operations (list/upload/download/edit)
- `internal/config` – config load/save/migration (`~/.config/pterminal/pterminal.json`)
- `internal/model` – config/domain types
- `scripts` – build-time helpers (xterm addon fetch via npm)

## Important invariants / UX expectations

- **Host IDs must be globally unique** across all networks (selection uses `hostId`).
- **Per-host terminals persist**: switching hosts retains scrollback until cleared or disconnected.
- “Disconnect” must **stop reconnect attempts** until the user explicitly connects again.
- Hosts with `sftp.enabled` show `Terminal`/`Files` tabs; per-host file view state (cwd/search/selection) should persist while the app runs.

## Go ↔ JS bridge rules

- Use explicit `type`-based RPC messages (see `rpcReq` in `internal/ui/window.go`).
- PTY data must remain binary-safe (base64 both directions).
- Avoid per-keystroke heavy RPC work; batch where possible to keep UI responsive.
- Keep RPC handlers non-blocking on the UI thread; avoid deadlocks (especially with native dialogs).

## Developer workflow

- `make assets` – fetch/update xterm.js + addons into `internal/ui/assets/vendor/`
- `make build` / `make run` – build and run
- `make release` – build optimized bundle into `release/`
- `make fmt` – `gofmt`
- `make vet` – `go vet`
