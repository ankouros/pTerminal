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
| PT-011 | maintainability | low | fixed | Repo contracts were not synced from `samakia-specs`; added sync script and docs. | `CONTRACTS.md`, `scripts/sync-contracts.sh`, `docs/contracts.md`, `README.md`, `ROADMAP.md` |
