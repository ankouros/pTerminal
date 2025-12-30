# pTerminal Roadmap

This roadmap lists what exists today and what is planned next. Status reflects intent, not guarantees.

## Current (Shipped)

- Linux WebView app with embedded xterm.js UI.
- Contracts synced from the `samakia-specs` submodule with `make specs-update` + `make sync-contracts`.
- Persistent SSH sessions with reconnect logic.
- SFTP file manager (list, upload/download, edit).
- Team discovery + sync on LAN with authenticated/encrypted transport.
- CLI/version metadata (`pterminal --version`).
- Keyboard-interactive SSH auth with prompt integration (memory-only credentials).
- Config import/export with normalization and conflict handling.
- Telecom (local PTY) driver for specialized environments.

## Near Term (Next)

- UI support for SSH key + agent auth selection and status.
- Better host key UX (key history, trust overrides per host).
- Expanded test coverage for P2P merge/conflict logic.
- Telemetry-free diagnostics pack for bug reports (opt-in).

## Mid Term (Planned)

- Multi-tab terminal UX polish (rename, reorder, persistence).
- Script runner enhancements (per-team permissions, output capture).
- Team repo conflict resolution UI.
- Improved file search and filtering in SFTP pane.

## Long Term (Backlog)

- Cross-platform support (macOS/Windows).
- Plugin-style drivers for additional connection types.
- SSH config import (known_hosts + ~/.ssh/config).
- Offline-first team sync with manual merge tools.

## Release Discipline

- Backward compatible config migrations only.
- Security fixes are prioritized over feature work.
