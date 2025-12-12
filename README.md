# pTerminal (Go)

A lightweight Linux GUI app providing **persistent multi-node SSH terminal sessions** with instant switching.

## Goals & Constraints

- Go + WebView (`github.com/webview/webview`)
- Terminal emulator: **xterm.js** (embedded assets)
- SSH: `golang.org/x/crypto/ssh`
- Single self-contained binary (assets embedded via `go:embed`)
- One persistent SSH session per node; switching nodes does not reconnect

See `PROJECT_REQUIREMENTS.md` and `AGENTS.md`.

## Quick start

### 1) Fetch frontend vendor assets (xterm.js)
This repo intentionally does **not** vendor xterm.js by default. Run:

```bash
make assets
```

This downloads `xterm.js`, `xterm.css`, and the `xterm-addon-fit` bundle into `assets/vendor/`.

### 2) Build & run
```bash
make build
./bin/pterminal
```

### 3) Configure nodes
By default the app reads nodes from:

- `~/.config/pterminal/nodes.json` (preferred)
- or falls back to `configs/nodes.sample.json`

Copy the sample:

```bash
mkdir -p ~/.config/pterminal
cp configs/nodes.sample.json ~/.config/pterminal/nodes.json
```

## Notes

- Password auth is supported for prototypes (password is requested in UI and kept in memory only).
- Host key verification supports:
  - strict checking against `~/.ssh/known_hosts`
  - or insecure (for prototyping) via config flag per node
