# pTerminal – AI System Prompt (Engineering Brief)

You are implementing a Linux GUI application in Go:
- WebView UI (HTML/CSS/JS)
- xterm.js terminal emulator
- Native Go SSH sessions (persistent)
- Binary-safe streaming between Go and JS

Hard constraints:
- Must not embed or shell out to native terminals
- Must not use X11 -into embedding
- Must not require GTK/Qt/Electron/Node/Python GUI frameworks
- Must not require tmux (optional extension later)

These constraints apply to all code under this repository, including future contributions.

---

## Repository layout

- `cmd/pterminal` – main entrypoint, minimal CLI wiring
- `internal/app` – top-level application bootstrap and OS checks
- `internal/ui` – WebView window, JS bridge, and embedded HTML/CSS/JS (xterm.js)
- `internal/sshclient` – native Go SSH + PTY handling (no external `ssh` binary)
- `internal/session` – per-host session manager, reconnect logic, state tracking
- `internal/config` – config file loading, saving, migration (`~/.config/pterminal/pterminal.json`)
- `internal/model` – shared config and domain types
- `internal/ui/assets` – browser UI (index.html, app.css, app.js, xterm.js assets)
- `scripts` – build-time helpers only (e.g. fetching xterm.js via npm)

---

## Implementation guidelines

- GUI:
  - All UI must be implemented using WebView with HTML/CSS/JS.
  - xterm.js is the only terminal emulator; do not shell out to native terminals or embed them.
  - No GTK/Qt/Electron/Node/Python GUI frameworks may be introduced.
- SSH:
  - All SSH connections must use `golang.org/x/crypto/ssh` (no external `ssh` process).
  - Sessions are long-lived and per-host; switching hosts must not reconnect by default.
- Go ↔ JS bridge:
  - Use JSON messages for control flow and **base64-encoded** strings for terminal I/O to stay binary-safe.
  - Keep the RPC surface small and explicit (`type`-based messages as in `rpcReq` in `internal/ui/window.go`).
- Assets:
  - UI and vendor assets should be embedded via `go:embed`; the app must not depend on external files at runtime.
  - Node/npm are allowed **only** for build-time tooling (e.g. fetching xterm.js) under `scripts/`.

---

## Coding style

- Go:
  - Prefer small packages with clear responsibilities (`config`, `session`, `sshclient`, `ui`, etc.).
  - Avoid global mutable state; prefer passing explicit dependencies.
  - Run `make fmt` before committing; keep code `gofmt`-clean.
- JavaScript:
  - Use plain ES modules / vanilla JS; avoid frontend frameworks and bundlers.
  - Keep browser-side state co-located with the UI that uses it.
  - Preserve binary-safe handling of terminal data (base64 in both directions).

---

## Developer workflow

- Use `make assets` to fetch/update xterm.js into `internal/ui/assets/vendor/`.
- Use `make build` / `make run` during development.
- Use `make vet` to catch obvious issues.

When in doubt, prefer simpler code paths and check new changes against the constraints at the top of this file.
