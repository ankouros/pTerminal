# Security Policy — pTerminal

Security is a core requirement of pTerminal.
This document defines the security model, assumptions, and disclosure process.

## Security Philosophy

- Least privilege by default; explicit trust over implicit trust.
- Key-based authentication preferred; avoid password storage.
- Rebuild over repair when compromise is suspected.
- No secrets in Git; configuration and exports must redact credentials.

## Threat Assumptions

- Credentials can leak.
- LAN environments are not trusted by default.
- Misconfiguration is possible.

## Required Controls

- Credentials (passwords, passphrases) remain memory-only and are never persisted.
- Host key verification must respect the configured mode (`known_hosts` or `insecure`).
- LAN sync requires authentication and encryption by default.
- Config exports must redact secrets and avoid unsafe paths.

## Incident Response

- Revoke or rotate affected credentials immediately.
- Remove compromised hosts from config and re-verify host keys.
- Document the incident in `issues-tracker.md` with impact and remediation.

## Responsible Disclosure

If you discover a vulnerability:
- Do not open a public issue.
- Contact the maintainers directly with reproduction steps and impact details.

## Scope

This policy covers:
- Code in this repository.
- Official release bundles and binaries.
