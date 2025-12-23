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

## Coding & review guidelines

- Keep Go files gofmt-clean and favor small, composable packages so WebView/UI glue stays thin.
- Prefer passing `context.Context` through SSH/session layers so reconnect/teardown logic can cancel goroutines quickly.
- Guard RPC handlers: validate payloads, never trust the UI blindly, and log concise error context using the existing logger.
- Asset changes go through `internal/ui/assets`; keep embedded `go:embed` lists in sync and avoid loading files from disk dynamically.
- When touching WebView JS, remember it is bundled at build time; no runtime CDN access is allowed.

## Testing & validation checklist

- Always run `make fmt` and `make vet` before sending patches; CI mirrors these checks.
- Use `go test ./...` for targeted unit coverage where packages already define tests (session/config/sftp).
- `make run` exercises the full stack; keep an SSH test host handy to verify PTY + SFTP happy-path and disconnect/reconnect flows.
- When touching release bits (packaging, Flatpak manifest, etc.) run `make release` once to confirm assets embed correctly.

## Frequent pitfalls

- Long RPC handlers will freeze the WebView. Offload SSH/SFTP work onto goroutines and respond via channels.
- Never write raw PTY bytes onto the bridge; base64 encode/decode at the API boundary.
- Ensure `hostId` remains unique even when importing configs—reject duplicates early in `internal/config`.
- Disconnect actions must flip the session into a "manual reconnect" state; stop timers/watchers as part of the same update.
- SFTP tab state is per-host. Changes to list/search panes should round-trip that state through `internal/session` instead of global variables.
