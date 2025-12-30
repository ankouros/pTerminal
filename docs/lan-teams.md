# LAN Teams Discovery + Sync

This document describes the LAN-only peer discovery and team sync behavior for pTerminal.

## Scope

- LAN-only (no WAN relay, no external service).
- Authenticated + encrypted sync when `PTERMINAL_P2P_SECRET` is set.
- If no secret is set, sync is disabled unless `PTERMINAL_P2P_INSECURE=1` is explicitly enabled.

## Discovery

- UDP broadcast on port 43277.
- Each instance announces:
  - deviceId
  - user name/email
  - TCP sync port
  - team summaries (id + name)
- Peers are considered active while announcements are seen within ~20 seconds.
  - Announcements include an HMAC when a secret is configured.

## Sync Transport

- TCP sync is established directly between peers on the LAN.
- Each side sends a `sync` payload with:
  - teams
  - networks/hosts
  - scripts
  - team repository manifests
- Both sides exchange file payloads for team repositories using a request/response flow
  (only missing/newer files are sent).
  - When `PTERMINAL_P2P_SECRET` is set, payloads are encrypted and authenticated.

## Team Repositories

- Each team has a local repository folder:
  - `~/.config/pterminal/teams/<teamId>/`
- Hidden metadata is stored at:
  - `~/.config/pterminal/teams/<teamId>/.pterminal/manifest.json`
- The manifest tracks files and tombstones for deletions.

## Conflict Handling

- Config items (teams, networks, hosts, scripts) use version vectors.
- If concurrent updates are detected:
  - The local item is marked `conflict: true`.
  - A conflict copy may be created with a new id and `(conflict)` suffix.
- File sync is conservative:
  - If hashes differ, remote files are stored as `*.conflict-<deviceId>-<timestamp>`
  - Deletions apply only when hashes match (to avoid data loss)

## UI

- A **Teams** window lets users:
  - manage profile (name/email)
  - create teams
  - add/remove members
  - view active members
  - see local team repo paths
- Navbar includes a **Team** dropdown.
  - Selecting a team filters networks/hosts/scripts to that team only.
  - Personal view shows only private items.

## Notes

- All sync is best-effort; peers may join/leave at any time.
- The sync protocol is designed to avoid data loss over speed.
