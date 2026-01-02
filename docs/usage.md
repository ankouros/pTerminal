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

## Samakia Host Roles

- Use the Host Role field to tag nodes as `fabric` or `platform` when connecting to Samakia Fabric or Samakia Platform.
- Roles label hosts in the UI and anchor upcoming verification flows for Samakia environments.
- Leave the role as `generic` for non-Samakia nodes.

## Samakia Verification Quick Actions

- Right-click a Samakia-tagged host to add or run a role-specific verification script.
- Verification scripts are read-only by default and execute in the active terminal.
- Scripts are saved in the Scripts panel and can be edited if needed.

## Samakia Inventory Import

- Use **Samakia Import** in the top bar to import Fabric/Platform inventories.
- Imports create or update a named network without overwriting existing config.
- Imported hosts default to SSH key auth and `known_hosts` verification.
- Imports update existing Samakia hosts in the target network and mark missing imported hosts as deleted.
- Hosts not previously imported are not removed.
- Choose a match mode: hostname (name-first), host address, or UID.
- UID matching requires each inventory entry to include a stable `uid` (or `id`/`vmid`).
- A summary modal shows counts, host-level changes, and provides JSON/CSV/Markdown export plus clipboard copy.

## Version Information

- Run `./bin/pterminal --version` (or `pterminal --version` if the binary is on your `$PATH`) to print the embedded version, git commit, and build timestamp without opening the UI.
- Updates are staged next to the current binary as `pterminal.next` and applied on restart.

## Tray Icon

- On Linux, pTerminal adds a tray icon (near the clock) with Show/Hide/Exit menu items.
- Use the tray menu to hide the window without quitting or to bring it back when hidden.
- "Exit pTerminal" opens a confirmation dialog before shutting down the app.
- Closing the main window only hides it to the tray so the icon/menu remain active; use the tray controls to reopen the UI or exit.
- The tray menu now also includes an "About pTerminal" entry that pops up the embedded version, git commit, and build timestamp.
- The About modal mirrors other pTerminal popups and now shows that same metadata alongside the latest GitHub release tag, release notes snippet, and links to check/install updates so you never have to guess what version is running.
- Popups now reuse the tray icon’s gradient, blur, and glow treatment so they feel visually anchored to the system bar icon.
- The navigation bar shows update status text and update controls; once a new release is available the check button hides, install shows download/install progress, and a restart prompt appears after staging the update (the restart relaunches pTerminal).

## Host Keys

- `known_hosts`: strict verification against `~/.ssh/known_hosts`.
- `insecure`: skip verification (dev/testing only).
- Unknown or mismatched host keys trigger a trust prompt.

## Passwords and Passphrases

- Credentials are kept in memory only.
- You will be prompted when needed (SSH password, key passphrase, SFTP custom password).
- Exports and config files never store secrets.
- If a key path is missing, pTerminal checks standard `~/.ssh` key files and prompts you to update the host if none are found.

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
