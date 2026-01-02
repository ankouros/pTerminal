# Samakia Acceptance Flow

This runbook describes the persistent Makefile targets used for Samakia-aligned
work in pTerminal.

## Targets

```bash
make samakia.design.check
make samakia.verify
make samakia.accept
make samakia.testing.check
make samakia.testing.verify
make samakia.testing.accept
```

## Behavior

- `samakia.design.check` validates required entry points and docs.
- `samakia.verify` runs formatting, vetting, and tests.
- `samakia.accept` runs design checks and verification in order.
- `samakia.testing.check` validates testing suite entry points.
- `samakia.testing.verify` runs the automated suite and writes reports under `TESTS/`.
- `samakia.testing.accept` runs testing suite verification in order.

## When to Run

- Run `samakia.design.check` before execution changes.
- Run `samakia.verify` before shipping code.
- Run `samakia.accept` before release or review.
- Run `samakia.testing.verify` when test report artifacts are required.
