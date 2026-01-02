# Samakia Integration

pTerminal is the official Samakia tool for connecting to and verifying nodes
that run Samakia Fabric and Samakia Platform.

## Goals

- Provide a single, audited SSH/SFTP client for Samakia development and production.
- Keep connection behavior aligned with `samakia-specs` and repo contracts.
- Preserve security defaults: key-based auth, `known_hosts` verification, and memory-only credentials.

## Host Roles

- Tag Samakia nodes as `fabric` or `platform` in the host editor.
- Roles surface in the host list and will anchor verification scripts and runbooks.
- Non-Samakia nodes should remain `generic`.

## Design and Production Strategy

pTerminal follows the Samakia Fabric design-production strategy:

- Design first: document intent (architecture + ADRs) before expanding behavior.
- Small, safe diffs: prefer incremental steps with explicit acceptance criteria.
- Production gates: run `make samakia.verify` and `make samakia.accept` before release.
- Docs are part of the system; update `ROADMAP.md`, `CHANGELOG.md`, and relevant docs with every change.

## Development Flow

- Follow the tree steps in `docs/concepts/samakia-development-flow.md`.
- Entry-point checks and acceptance are enforced via Makefile targets.

## Verification Quick Actions

- Use host menu quick actions to add or run Samakia verification scripts.
- Scripts are role-specific and read-only by default.

## Inventory Import

- Use the Samakia Import flow to add Fabric/Platform inventories to a network.
- Imports default to SSH key auth with `known_hosts` verification.
- Choose a match mode (hostname, host address, or UID) to align updates with inventory identity.
- Import summaries include host-level changes, JSON/CSV/Markdown exports, and clipboard copy.

## Next Integrations

- Idempotent inventory updates and host removal handling.
