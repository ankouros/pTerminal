# Contracts

pTerminal contracts are defined in `samakia-specs` and synced into this repo.

## Source of Truth

- Canonical contract: `/home/aggelos/samakia-specs/repo-contracts/pterminal.md`
- Shared ecosystem contract: `/home/aggelos/samakia-specs/specs/base/ecosystem.yaml`
- Local mirror: `CONTRACTS.md`

## Sync

Run the sync target after updating the spec or when you pull new changes:

```bash
make sync-contracts
```

By default the sync reads from the `specs/samakia-specs` submodule. Override with:

```bash
SAMAKIA_SPECS_PATH=/path/to/samakia-specs make sync-contracts
```

If you need to initialize the submodule first:

```bash
make specs-update
```

## Updates

- Update the contract in `samakia-specs` first.
- Sync into this repo with `make sync-contracts`.
- Note any expectations changes in `issues-tracker.md` and `ROADMAP.md`.
- Evaluate contracts across samakia-fabric, samakia-platform, and pTerminal on every prompt.
