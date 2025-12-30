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
