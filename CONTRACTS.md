# pTerminal Contracts

Source of truth: `/home/aggelos/samakia-specs/repo-contracts/pterminal.md`
Sync target: `/home/aggelos/pTerminal/CONTRACTS.md` (run `make sync-contracts` in pTerminal).

These contracts define non-negotiable expectations for quality, UX, security, and operations.
Any change that violates a contract must be redesigned before merging.

## Quality Contract

- Go code must be `gofmt` clean and pass `go vet`.
- New non-trivial behavior must include tests or a documented reason to defer.
- Changes must keep config migrations backward compatible.
- Build/release scripts must remain reproducible and deterministic.

## UX Contract

- UI actions must not block the WebView UI thread.
- Credentials are memory-only; no passwords or passphrases are persisted to disk.
- Error states should be recoverable (clear messaging, retry paths where safe).
- Host key prompts must clearly surface fingerprints and trust scope.
- About/update popups must display the current version/commit/build metadata and release status with the same styling/interaction quality as other pTerminal dialogs.

## Security Contract

- LAN sync requires authentication + encryption by default.
- Path traversal into/outside team repositories is forbidden.
- Secrets must be redacted from exports, logs, and config persistence.
- Host key verification must respect the configured mode (`known_hosts` or `insecure`).

## Reliability Contract

- Session disconnects must clean up resources and avoid goroutine leaks.
- SFTP and SSH operations must be bounded by context timeouts.
- Config updates must be atomic and durable (tmp + fsync + rename).

## Performance Contract

- Terminal output buffering must enforce size caps and be backpressure-safe.
- Asset embedding should not add runtime IO dependencies.

## Documentation Contract

- Any feature change must update `ROADMAP.md` and relevant docs in `docs/`.
- Any behavior change that affects UX or security must update `issues-tracker.md`.
