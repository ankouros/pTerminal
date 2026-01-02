# Manual Test Checklist

Status: DEFERRED (requires interactive UI + SSH targets)

## Checklist

- [ ] Create a network and host, set role `fabric`, connect via SSH key.
- [ ] Trigger host menu quick actions, add and run verify script.
- [ ] Import a Fabric inventory (hostname match mode) and confirm summary modal.
- [ ] Import a Platform inventory (host match mode) and confirm updates/removals.
- [ ] Import a UID inventory and confirm UID matching + summary reports.
- [ ] Export Samakia import report in JSON/CSV/Markdown and copy to clipboard.
- [ ] Exercise SFTP: list, download, upload, rename, edit, and delete.
- [ ] Validate host key trust flow in `known_hosts` mode.
- [ ] Validate LAN sync discovery and conflict markers (if enabled).

## Notes

- Manual UI and SSH/SFTP verification requires a live desktop session and reachable test nodes.
- Re-run this checklist after completing the automated test suite with real environment access.
