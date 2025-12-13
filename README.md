# pTerminal (Go)

A lightweight Linux GUI app providing **persistent multi-node SSH terminal sessions** with a modern HTML/JS UI rendered via WebView.

## Status

- ✅ WebView window using `github.com/webview/webview_go` with inlined HTML/CSS/JS via `go:embed`
- ✅ Embedded **xterm.js** terminal with binary-safe streaming between Go and JS
- ✅ Native Go SSH sessions using `golang.org/x/crypto/ssh`, one persistent session per host
- ✅ Automatic reconnect logic with per-host status (connected / reconnecting / disconnected)
- ✅ JSON configuration file (`~/.config/pterminal/pterminal.json`) with networks and hosts, editable from the UI
- ✅ SSH host key verification, including UX for unknown / mismatched host keys
- ⚙️ Planned: SSH key + agent flows in the UI, SFTP browser, richer import/export and session UX


## Goals & Constraints

- Go + WebView (no GTK/Qt/Electron/Node/Python GUI frameworks)
- Terminal emulator: **xterm.js** (assets shipped and embedded into the binary)
- SSH: `golang.org/x/crypto/ssh` (no external `ssh` binary)
- Single self-contained binary (HTML/CSS/JS embedded via `go:embed`)
- One persistent SSH session per host; switching hosts does not reconnect

## Getting started

### 1) Fetch frontend vendor assets (xterm.js)

Xterm assets are not committed to the repo; they are fetched on demand with a small npm helper. You need a working `node` + `npm` for this step:

```bash
make assets
```

This downloads `xterm.js`, `xterm.css`, and the `xterm-addon-fit` bundle into `internal/ui/assets/vendor/`, which is then embedded into the Go binary.

### 2) Build & run

```bash
make build
./bin/pterminal
```

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
          "auth": {
            "method": "password"
          },
          "hostKey": {
            "mode": "known_hosts"
          }
        }
      ]
    }
  ]
}
```

### 4) Authentication & host keys

- **Auth methods** (model-level):
  - `password`
  - `key` (wired in config model, UI wiring planned)
  - `agent` (wired in config model and ssh client)
- **Host key modes**:
  - `known_hosts` – strict checking against `~/.ssh/known_hosts`
  - `insecure` – skip host key verification (for prototyping only)

The UI exposes a confirmation dialog for unknown / changed host keys and can persist a trusted key via the Go backend.

## Development

- `make assets` – fetch/update xterm.js assets into `internal/ui/assets/vendor/`
- `make build` – build the `pterminal` binary into `bin/`
- `make run` – build and run the app
- `make fmt` – format Go code
- `make vet` – run `go vet`

The hard constraints for this project are:

- No embedding or shelling out to native terminals
- No X11 `-into` embedding tricks
- No GTK/Qt/Electron/Node/Python GUI frameworks
- No tmux dependency (optional integration later)
