# Changelog

All notable changes to this repository are documented here.

## Unreleased

## v1.1.0 - 2026-01-02

- Added shared ecosystem contract alignment with samakia-specs and samakia-fabric.
- Added security policy and changelog entry points.
- Aligned AGENTS.md protocol with samakia-fabric expectations.
- Added concise Phase/Acceptance protocol to AGENTS.md.
- Added docs principles/glossary and moved release/docker docs under `docs/`.
- Added home-directory Codex memory pointer in AGENTS.md.
- Added Session Log reminder in AGENTS.md.
- Added Samakia integration scaffolding (host roles, architecture/ADR baselines, roadmap update).
- Added Samakia verification quick actions, design entry checks, and acceptance targets.
- Added Samakia inventory import helper and acceptance runbook.
- Added Samakia import summary modal and idempotent update/removal handling.
- Added Samakia import report export and managed-by safeguards for removals.
- Refined Samakia import matching (hostname-first) and added CSV export/copy.
- Added Samakia import match-mode selection and Markdown report export.
- Added testing suite targets and report artifacts under `TESTS/`.
- Fixed a WebView startup JS parse error caused by escaped string delimiters in `app.js`.
- Ensure newly created/updated networks stay visible by aligning the active team filter to the network scope.
- Auto-detect missing SSH key files, try standard key paths, and surface a UI popup when no key is found.
- Update status now pushes to the UI during checks/installs to avoid stuck “Installing…” states.
- Update installs now stage a `.next` binary and apply it on restart to avoid in-place replacement hangs.
- Update installs now show download/install progress, hide the check button once a new release is available, and prompt for restart after staging.
- Restarting after an update now terminates cleanly and avoids JS/GTK errors during shutdown.
- Update restart now relaunches the app automatically.
- Update check messaging only appears while a check is running.
- Updated release and usage documentation for the minor release update UX.
- Added v1.1.0 release notes under `docs/releases/`.
