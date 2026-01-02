# Testing Suite (Design)

This document defines the testing strategy for pTerminal and the reporting
workflow used to capture evidence of verification and acceptance runs.

## Goals

- Verify core behavior without persisting secrets.
- Provide repeatable, deterministic test runs with report artifacts.
- Exercise the Samakia integration flows (inventory import, roles, scripts).
- Create a clear manual test map for UI and SSH/SFTP scenarios.

## Scope

Automated checks focus on Go packages and configuration workflows. Manual checks
cover UI flows that require WebView interaction or live SSH targets.

### Automated Coverage

- Config normalization, import/export, and Samakia inventory import.
- SSH auth acceptance tests (password/key/agent/keyboard-interactive).
- SFTP client operations and session manager behavior.
- Static vetting of Go sources.

### Manual Coverage (Required)

- UI host editor (create/edit/delete, role tagging).
- Session flows: connect, reconnect, disconnect, and tab switching.
- Host key prompts (`known_hosts`, mismatch handling).
- Samakia import: hostname/host/UID match modes and summary modal actions.
- Script templates: add/run/edit verification scripts per role.
- SFTP: list, download, upload, rename, delete, and inline edit/save.
- Team sync and conflict rendering (LAN).

## Test Report Workflow

Reports are written under `TESTS/` using timestamped directories:

- `TESTS/run-YYYYMMDDTHHMMSSZ/summary.md`
- `TESTS/run-YYYYMMDDTHHMMSSZ/go-vet.log`
- `TESTS/run-YYYYMMDDTHHMMSSZ/go-test.log`
- `TESTS/run-YYYYMMDDTHHMMSSZ/go-test-json.log` (optional/full)

## Make Targets

- `make samakia.testing.check`: validate test suite entry points.
- `make tests.report`: run the automated suite and write reports to `TESTS/`.
- `make samakia.testing.verify`: entry check + test reports.
- `make samakia.testing.accept`: verification + acceptance marker.

## Manual Test Checklist

Use this list to record UI coverage after automated tests:

- Create a network and host, set role `fabric`, connect via SSH key.
- Trigger host menu quick actions, add and run verify script.
- Import a Fabric inventory (hostname match mode) and confirm summary modal.
- Import a Platform inventory (host match mode) and confirm updates/removals.
- Import a UID inventory and confirm UID matching + summary reports.
- Export Samakia import report in JSON/CSV/Markdown and copy to clipboard.
- Exercise SFTP: list, download, upload, rename, edit, and delete.
- Validate host key trust flow in `known_hosts` mode.
- Validate LAN sync discovery and conflict markers (if enabled).

## Weakness Hunting Notes

- Attempt imports with missing or duplicate identifiers.
- Verify unmanaged hosts are never mutated or removed.
- Ensure import reports contain no secrets.
- Confirm UI remains responsive during long-running SSH sessions.
