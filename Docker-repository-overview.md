# pTerminal (Docker)

pTerminal is a lightweight Linux GUI app for persistent, multi-node SSH terminals with a fast WebView UI.
It runs xterm.js inside the app (no external terminal binaries) and keeps per-host sessions alive even when you switch views.

## Why it is useful
- One app for many SSH hosts with persistent scrollback per host.
- LAN discovery and peer sync so teams can keep hosts and scripts aligned.
- Built-in SFTP file browser and editor.
- Multi-tab terminals per host with runtime renaming.
- Runs as a single binary or via Docker on any Linux desktop.

## Key features
- Persistent per-host terminals with scrollback retention.
- Multiple terminal tabs per host with rename-on-the-fly.
- Native Go SSH and SFTP (no external ssh/sftp binaries).
- WebView UI with embedded HTML/CSS/JS and xterm.js.
- Host-level and network-level scopes (private vs team).
- Team scripts with shared commands and descriptions.
- Telecom driver support for specialized telecom hosts.
- Binary-safe streaming between Go and JS (base64 PTY data).

## Collaboration and teams
- Teams are managed by email with roles (admin and user).
- Team admins can add/remove members and manage roles.
- Join requests are persistent and approved/declined by admins.
- Users can belong to multiple teams; each team is isolated.
- Team repositories (docs/scripts/help files) sync across team members.
- Hosts and scripts can be private or team-shared per item.

## LAN discovery and sync
- Best-effort, unauthenticated discovery on the same LAN.
- Instances announce presence and sync team-scoped data.
- Request/response file sync keeps bandwidth low.
- Strong conflict handling with version vectors.
- Designed for trusted networks only.

## Files and SFTP
- Built-in file browser with uploads, downloads, rename, delete.
- Open and edit remote files from inside the app.
- SFTP credentials can reuse SSH or be custom per host.

## Docker usage
The image is a GUI app and needs access to your display server. Use the helper script:

```
./scripts/docker/run_image.sh
```

It auto-mounts your config and Downloads folders and passes Wayland/X11 settings.

Manual example:

```
docker run --rm \
  -e DISPLAY \
  -v /tmp/.X11-unix:/tmp/.X11-unix \
  -v "$HOME/.config/pterminal:/home/pterminal/.config/pterminal" \
  -v "$HOME/Downloads:/home/pterminal/Downloads" \
  ankouros/pterminal:latest
```

If you see “Authorization required” errors on X11, pass XAUTHORITY explicitly:

```
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

```
docker run --rm \
  -e XDG_RUNTIME_DIR=/tmp/xdg-runtime \
  -e WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-wayland-0} \
  -e DBUS_SESSION_BUS_ADDRESS=unix:path=/tmp/xdg-runtime/bus \
  -v "$XDG_RUNTIME_DIR:/tmp/xdg-runtime" \
  -v "$HOME/.config/pterminal:/home/pterminal/.config/pterminal" \
  -v "$HOME/Downloads:/home/pterminal/Downloads" \
  ankouros/pterminal:latest
```

## Persistence
- Config lives in `~/.config/pterminal` on the host and is mounted into the container.
- Downloads are stored in your host `~/Downloads` folder.

## Platform notes
- Linux desktop only (Wayland or X11).
- Docker runs with software rendering by default for compatibility.
  Set `PTERMINAL_GPU=1` to try GPU acceleration.

## Security model
- LAN discovery is unauthenticated; use on trusted networks.
- SSH and SFTP are native Go implementations.
- No external terminal processes or tmux are required.

## Links
- Source: https://github.com/ankouros/pterminal
