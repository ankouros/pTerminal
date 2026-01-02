# Issues Tracker

Track open issues, proposals, and implementation status for this repo.

| ID | Type | Severity | Status | Summary | Areas |
| --- | --- | --- | --- | --- | --- |
| PT-001 | security | critical | fixed | P2P file sync accepts absolute/escape paths, enabling read/write/delete outside team repo. | `internal/p2p/service.go`, `internal/p2p/paths.go` |
| PT-002 | bug | high | fixed | `HostKeyInsecure` config is ignored; host key checks always use `known_hosts`. | `internal/sshclient/sshclient.go`, `internal/model/node.go` |
| PT-003 | security | high | fixed | P2P sync is unauthenticated/unencrypted; any LAN peer can inject config or files. | `internal/p2p/service.go`, `internal/p2p/secure.go`, `docs/lan-teams.md` |
| PT-004 | security | high | fixed | Plaintext passwords stored in config (`AuthConfig.Password`, `SFTPConfig.Password`); exports leak secrets. | `internal/config/config.go`, `internal/config/import.go`, `internal/ui/assets/app.js`, `README.md` |
| PT-005 | reliability | medium | fixed | Team manifest file order is nondeterministic; can cause repeated rewrites/sync noise. | `internal/teamrepo/manifest.go` |
| PT-006 | quality | medium | fixed | README claims tests exist, but no `*_test.go` or CI for `go test`. | `internal/config/config_test.go`, `internal/session/manager_test.go`, `internal/sftpclient/manager_test.go` |
| PT-007 | maintainability | low | fixed | `internal/config/nodes.go` is unused legacy code. | `internal/config/nodes.go` |
| PT-008 | feature | low | fixed | SSH key auth lacks passphrase support (only `ParsePrivateKey`). | `internal/sshclient/sshclient.go`, `internal/ui/assets/app.js` |
| PT-009 | reliability | low | fixed | App shutdown does not explicitly disconnect SSH sessions. | `internal/ui/window.go` |
| PT-010 | feature | low | fixed | Added keyboard-interactive SSH auth option with prompt support. | `internal/model/node.go`, `internal/sshclient/sshclient.go`, `internal/ui/assets/app.js`, `internal/ui/assets/index.html`, `docs/usage.md`, `README.md` |
| PT-011 | feature | low | fixed | Added CLI version metadata. | `cmd/pterminal/main.go`, `internal/buildinfo/buildinfo.go`, `Makefile`, `README.md`, `docs/usage.md` |
| PT-012 | feature | medium | fixed | Tray icon remains active when the window hides, exposes show/hide/about/exit actions, and exit requests show a confirmation dialog instead of terminating the app. | `internal/ui/window.go`, `internal/ui/tray_linux.go`, `docs/usage.md`, `README.md` |
| PT-011 | feature | low | fixed | Added version metadata and `--version` flag for CLI binaries. | `cmd/pterminal/main.go`, `internal/buildinfo/buildinfo.go`, `Makefile`, `README.md`, `docs/usage.md` |
| PT-011 | maintainability | low | fixed | Repo contracts were not synced from `samakia-specs`; added sync script and docs. | `CONTRACTS.md`, `scripts/sync-contracts.sh`, `docs/contracts.md`, `README.md`, `ROADMAP.md` |
| PT-012 | maintainability | low | fixed | Contracts were not actively tied to the specs repo; added `samakia-specs` submodule + sync workflow. | `.gitmodules`, `specs/samakia-specs`, `scripts/sync-contracts.sh`, `README.md`, `ROADMAP.md`, `docs/onboarding.md` |
| PT-013 | quality | medium | fixed | Added an acceptance test suite that exercises password/key/agent/keyboard-interactive SSH auth flows to catch regressions before release. | `internal/sshclient/sshclient.go`, `internal/sshclient/sshclient_auth_acceptance_test.go`, `README.md`, `docs/usage.md`, `docs/onboarding.md`, `ROADMAP.md` |
| PT-014 | feature | medium | fixed | GitHub release awareness plus “Check updates” / “Install update” buttons now download and install the latest portable bundle directly from GitHub releases. | `internal/update`, `internal/ui/window.go`, `internal/ui/assets/index.html`, `internal/ui/assets/app.js`, `internal/ui/assets/app.css`, `README.md`, `docs/usage.md` |
| PT-015 | UX | low | fixed | All modals/popups now mirror the tray icon’s gradient/blur/glow styling so they feel like they derive from the system bar icon menu. | `internal/ui/assets/app.css` |
| PT-016 | reliability | low | fixed | `INSTALL.sh` now chowns `bin/` artifacts back to the invoking user so future `make` invocations run without permission errors. | `INSTALL.sh`, `README.md` |
| PT-017 | maintainability | low | fixed | Aligned shared ecosystem contract baseline with samakia-fabric via samakia-specs. | `CONTRACTS.md`, `docs/contracts.md`, `AGENTS.md`, `README.md`, `SECURITY.md`, `CHANGELOG.md` |
| PT-018 | feature | medium | fixed | Samakia integration scaffolding: host roles, verification quick actions, and design/acceptance flow targets. | `internal/model/node.go`, `internal/ui/assets/app.js`, `scripts/entry-checks/samakia-design-check.sh`, `ROADMAP.md` |
| PT-019 | feature | medium | fixed | Implement Samakia inventory import helpers (Fabric/Platform). | `internal/config/samakia_import.go`, `internal/ui/window.go`, `internal/ui/assets/app.js`, `docs/concepts/samakia-inventory-import.md` |
| PT-020 | feature | low | fixed | Add Samakia import match modes and Markdown report export. | `internal/config/samakia_import.go`, `internal/config/samakia_report.go`, `internal/ui/window.go`, `internal/ui/assets/*`, `docs/usage.md` |
| PT-021 | maintainability | low | fixed | Add testing suite harness, entry checks, and report artifacts under `TESTS/`. | `scripts/tests/run-suite.sh`, `scripts/entry-checks/samakia-testing-check.sh`, `Makefile`, `docs/testing-suite.md` |
| PT-022 | bug | high | fixed | WebView UI fails to load due to escaped string delimiters in `app.js`. | `internal/ui/assets/app.js` |
| PT-023 | bug | medium | fixed | New network may be hidden after save if team filter does not match scope; align active team filter to new scope. | `internal/ui/assets/app.js` |
| PT-024 | bug | medium | fixed | Missing SSH key path now falls back to standard keys and shows a UI popup when none are found. | `internal/sshclient/sshclient.go`, `internal/ui/window.go`, `internal/ui/assets/app.js` |
| PT-025 | bug | medium | fixed | Update check/install now pushes state updates to the UI to avoid hanging status. | `internal/ui/window.go`, `internal/ui/assets/app.js` |
| PT-026 | bug | medium | fixed | Update installs now stage a `.next` binary and apply it on restart to avoid in-place replacement hangs. | `internal/ui/window.go`, `internal/update/stage.go`, `internal/app/app.go` |
| PT-027 | UX | medium | fixed | Update install now shows download/install progress, hides the check button once a release is available, and prompts for restart after staging. | `internal/ui/window.go`, `internal/ui/assets/app.js`, `README.md`, `docs/usage.md` |
| PT-028 | bug | medium | fixed | Restarting after updates no longer triggers UI freezes or JS notify errors. | `internal/ui/window.go`, `internal/ui/assets/app.js`, `CHANGELOG.md` |
| PT-029 | bug | medium | fixed | Update restart now relaunches the app instead of only closing. | `internal/ui/window.go`, `internal/ui/assets/app.js` |
| PT-030 | UX | low | fixed | Update check messaging only appears while a check is running. | `internal/ui/assets/app.js` |
