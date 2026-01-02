# Samakia Inventory Import (Design)

This document captures the design for importing Samakia Fabric and Samakia
Platform inventory into pTerminal. Implementation is tracked in the roadmap.

## Goals

- Import host inventories without embedding secrets in config files.
- Tag imported hosts with the correct Samakia role (`fabric` or `platform`).
- Preserve the memory-only credential contract and host key verification modes.
- Keep imports deterministic and auditable (no hidden mutation).

## Inputs

### Samakia Fabric

Preferred input sources (read-only):

- `terraform output -json` for `lxc_inventory`.
- The Fabric dynamic inventory JSON output
  (`ansible-inventory -i fabric-core/ansible/inventory/terraform.py --list`).

### Samakia Platform

Preferred input sources (read-only):

- Platform environment manifests (node IPs + roles) captured as a JSON export.
- Kubernetes node listings exported to JSON and mapped to host entries.

## Proposed Import Flow

1. User selects an inventory JSON file (Fabric or Platform).
2. pTerminal parses and validates the schema.
3. Hosts are normalized and assigned:
   - `role: fabric` or `role: platform`
   - `driver: ssh`
   - `hostKey.mode: known_hosts` by default
4. The importer creates or updates a named Network and appends hosts.
5. Secrets are never persisted; auth methods default to `key` or `agent`.

## Schema Sketch (Fabric)

A Fabric import expects host entries with:

- `hostname`
- `ansible_host` or `ip`
- `user` (optional; default to `samakia`)
- `port` (optional; default to `22`)

## Security Notes

- No secrets in the import file.
- SSH keys and passwords remain memory-only.
- All imports must log their source path and timestamp (no secret content).

## Open Questions

- Canonical JSON schema for Platform inventories.
- How to surface import reports in team sync workflows.

## Implementation Status

- Initial import helper reads JSON files and creates/updates a named network.
- Supports host list JSON, Terraform `lxc_inventory` output, and Ansible inventory output.
- Imports are idempotent for Fabric/Platform hosts and mark missing imported entries as deleted.
- Hosts not tagged as `managedBy: samakia-import` are not removed.
- Import match mode is selectable: hostname (name-first), host address, or UID.
- UID matching expects each entry to provide a stable `uid` (or `id`/`vmid`).
- Import summaries include host-level change lists and can be exported as JSON/CSV/Markdown or copied to clipboard.
