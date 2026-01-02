# Samakia Development Flow

This document defines the required tree steps for Samakia-related work in
pTerminal. All changes must follow this sequence:

## 1. Design

Design creates:

- Documentation (intent, scope, UX impact, and contracts).
- Entry-point checks (scripts that validate required docs/targets exist).
- Makefile targets that persist the design/verify/accept flow.

Required targets:

- `make samakia.design.check`
- `make samakia.verify`
- `make samakia.accept`
- `make samakia.testing.check` (testing suite entry points)
- `make samakia.testing.verify`
- `make samakia.testing.accept`

## 2. Execution

- Implement the smallest safe diff that matches the approved design.
- Keep changes scoped and reviewable.
- Update `issues-tracker.md` when new work is started or discovered.

## 3. Verification

- Run `make samakia.verify`.
- For full testing suite evidence, run `make samakia.testing.verify`.
- Capture evidence in notes without secrets.

## 4. Acceptance

- Run `make samakia.accept`.
- For full testing suite acceptance, run `make samakia.testing.accept`.
- Update `CHANGELOG.md` and `ROADMAP.md` if behavior or scope changes.
- See `docs/tutorials/samakia-acceptance.md` for the runbook.

## Checklist

Use `docs/change-checklist.md` for every change.
