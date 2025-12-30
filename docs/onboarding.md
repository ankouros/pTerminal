# Developer Onboarding

This guide covers setting up the repo for local development.

## Prerequisites

- Go 1.22+
- Linux WebKitGTK + GTK3 dev packages
  - Debian/Ubuntu: `libwebkit2gtk-4.1-dev libgtk-3-dev pkg-config`
- Optional: Node + npm for `make assets`

## Clone and Build

```bash
make specs-update
make sync-contracts
make assets
make build
./bin/pterminal
```

## Optional automated install

Run `sudo ./INSTALL.sh`; it installs Go, GTK/WebKit, Node (which typically provides `npm`), and git using the official Ubuntu repositories listed in `/etc/apt/sources.list.d/ubuntu.sources`, skips already satisfied packages, and requires the standard Ubuntu keyring, then runs the test suite, rebuilds the executable, installs `/usr/local/bin/pterminal`, and refreshes the icon cache so the desktop icon registers.

## Run from Source

```bash
make run
```

## Tests and Checks

```bash
make fmt
make vet
go test ./...
go test ./internal/sshclient
```

## Key Environment Variables

- `PTERMINAL_P2P_SECRET`: shared secret for LAN team sync (required for sync).
- `PTERMINAL_P2P_INSECURE=1`: enable unauthenticated LAN sync (not recommended).
- `PTERMINAL_SOFTWARE_RENDER=1`: force software rendering in WebView.
- `PTERMINAL_DISABLE_GPU=1`: disable GPU rendering.

## Debug Tips

- If the WebView fails to start, confirm `DISPLAY`/`WAYLAND_DISPLAY` is set.
- For Docker/Flatpak, ensure GUI sockets and XAUTHORITY/Wayland envs are passed.
