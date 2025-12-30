# Usage Guide

This guide covers the end-user workflow for pTerminal.

## First Launch

- Run the binary (`./bin/pterminal`).
- A config file is created at `~/.config/pterminal/pterminal.json`.
- Use the in-app editor or edit the JSON directly.

## Adding Hosts

- Create a Network, then add Hosts.
- Supported drivers:
  - `ssh` (default)
  - `telecom` (local PTY process)
- Auth methods:
  - `password`
  - `key`
  - `agent`
  - `keyboard-interactive`

The CLI test suite (`go test ./internal/sshclient`) now runs `internal/sshclient/sshclient_auth_acceptance_test.go`, which exercises every supported SSH auth method to guard against regressions.

## Version Information

- Run `./bin/pterminal --version` (or `pterminal --version` if the binary is on your `$PATH`) to print the embedded version, git commit, and build timestamp without opening the UI.

## Tray Icon

- On Linux, pTerminal adds a tray icon (near the clock) with Show/Hide/Exit menu items.
- Use the tray menu to hide the window without quitting or to bring it back when hidden.
- "Exit pTerminal" opens a confirmation dialog before shutting down the app.
- Closing the main window only hides it to the tray so the icon/menu remain active; use the tray controls to reopen the UI or exit.
- The tray menu now also includes an "About pTerminal" entry that pops up the embedded version, git commit, and build timestamp.

## Host Keys

- `known_hosts`: strict verification against `~/.ssh/known_hosts`.
- `insecure`: skip verification (dev/testing only).
- Unknown or mismatched host keys trigger a trust prompt.

## Passwords and Passphrases

- Credentials are kept in memory only.
- You will be prompted when needed (SSH password, key passphrase, SFTP custom password).
- Exports and config files never store secrets.

## Files (SFTP)

- Enable SFTP per host to use the Files tab.
- Features: directory listing, upload/download, rename, delete, inline edit.
- Downloads are saved to `~/Downloads`.

## Teams and LAN Sync

- LAN sync requires `PTERMINAL_P2P_SECRET` set to the same value on all peers.
- Team repositories live in `~/.config/pterminal/teams/<teamId>/`.
- Conflicts are written as `*.conflict-<deviceId>-<timestamp>`.

## Import / Export

- Export writes a timestamped config to `~/Downloads`.
- Import replaces the active config and writes a backup in `~/.config/pterminal/`.

## Telecom Driver

- Set `telecom.path` to the local executable.
- Optional fields: `protocol`, `command`, `args`, `workDir`, `env`.
- Telecom runs locally and is rendered inside the terminal.
