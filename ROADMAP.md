# pTerminal Roadmap

This roadmap lists what exists today and what is planned next. Status reflects intent, not guarantees.

## Strategy (Design to Production)

pTerminal follows the Samakia Fabric design-production strategy:

- Design first, then implement. Document intent in `ARCHITECTURE.md` and `DECISIONS.md`.
- Ship small, reviewable diffs with explicit acceptance evidence.
- Production readiness requires `make samakia.verify` and `make samakia.accept`.

## Baseline (Shipped)

- Linux WebView app with embedded xterm.js UI.
- Contracts synced from the `samakia-specs` submodule with `make specs-update` + `make sync-contracts`.
- Shared ecosystem contract alignment with samakia-fabric via `samakia-specs`.
- Persistent SSH sessions with reconnect logic.
- SFTP file manager (list, upload/download, edit).
- Team discovery + sync on LAN with authenticated/encrypted transport.
- Acceptance tests cover password, SSH key, agent, and keyboard-interactive flows (`internal/sshclient/sshclient_auth_acceptance_test.go`).
- Tray icon/menu for window visibility and exit confirmation.
- About dialog and update UI surface version/commit/build metadata and release status.
- CLI/version metadata (`pterminal --version`).
- GitHub release awareness + “Check updates”/“Install update” controls.
- Keyboard-interactive SSH auth with prompt integration (memory-only credentials).
- Config import/export with normalization and conflict handling.
- Telecom (local PTY) driver for specialized environments.
- Samakia development flow targets (`make samakia.design.check`, `make samakia.verify`, `make samakia.accept`).
- Testing suite harness with report artifacts under `TESTS/`.
- WebView UI asset parsing regression fixed (app JS loads cleanly).
- Network creation now aligns the active team filter to keep new networks visible.
- SSH key auth now auto-checks standard key paths and prompts when none are found.
- Update status now pushes to the UI during checks/installs to avoid hanging state.
- Update installs now stage `.next` binaries and apply on restart for safer self-updates.
- Update install flow now shows download/install progress, hides the check button once a release is available, and prompts for restart after staging.
- Restarting after an update now terminates cleanly without UI thread errors.
- Update restart now relaunches the app automatically.
- Update check messaging only appears while a check is running.
- Release docs updated for minor version update UX changes.
- Added v1.1.0 release notes under `docs/releases/`.

## Phase 1 — Samakia Integration Design (Design-Only)

Status: In progress

- Document Samakia integration goals and constraints. (done)
- Add architecture and ADR baselines aligned with Samakia Fabric structure. (done)
- Define host role taxonomy for Fabric/Platform nodes. (done)

## Phase 2 — Samakia Integration Implementation

Status: In progress

- Expose host role tagging in the UI and config. (done)
- Add role-specific verification script templates. (done)
- Design Samakia inventory import helpers. (done)
- Implement Samakia inventory import helpers. (done)
- Add import match-mode selection and Markdown report exports. (done)
- Expand tests for role-driven UX and config normalization. (planned)

## Phase 3 — Production Readiness for Samakia

Status: Planned

- Verification runbooks for Fabric/Platform nodes.
- Evidence capture for role-specific verification flows.
- Stability targets for long-running sessions and multi-node verification.

## Backlog (Product Enhancements)

- UI support for SSH key + agent auth selection and status.
- Better host key UX (key history, trust overrides per host).
- Expanded test coverage for P2P merge/conflict logic.
- Telemetry-free diagnostics pack for bug reports (opt-in).
- Multi-tab terminal UX polish (rename, reorder, persistence).
- Script runner enhancements (per-team permissions, output capture).
- Team repo conflict resolution UI.
- Improved file search and filtering in SFTP pane.
- Cross-platform support (macOS/Windows).
- Plugin-style drivers for additional connection types.
- SSH config import (known_hosts + ~/.ssh/config).
- Offline-first team sync with manual merge tools.

## Release Discipline

- Backward compatible config migrations only.
- Security fixes are prioritized over feature work.
