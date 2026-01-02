# Architecture Decision Records (ADR) — pTerminal

This document records key architectural and product decisions for pTerminal.

- Preserve design intent
- Explain trade-offs
- Prevent regressions
- Enable aligned future changes

This document is authoritative.

---

## ADR-0001 — Go + WebView for a Single-Binary Desktop App

**Status:** Accepted  
**Date:** 2026-01-02

### Decision

pTerminal is implemented in Go with a WebView UI (`webview_go`) to deliver a
single-binary desktop application.

### Rationale

- Small, portable runtime footprint
- Native performance for SSH/SFTP
- Simple distribution for Linux workstations

### Consequences

- UI logic must respect the WebView thread constraints
- WebKitGTK/GTK dependencies are required on Linux

---

## ADR-0002 — xterm.js as the Terminal Renderer

**Status:** Accepted  
**Date:** 2026-01-02

### Decision

All terminal rendering uses xterm.js embedded in the WebView.

### Rationale

- Mature terminal emulation with addon ecosystem
- Consistent UX across host types (SSH and telecom)

### Consequences

- Terminal output is base64 bridged between Go and JS
- UI performance must be protected with buffering caps

---

## ADR-0003 — Credentials Are Memory-Only

**Status:** Accepted  
**Date:** 2026-01-02

### Decision

Passwords and passphrases are never persisted to disk. Config exports are
redacted by default.

### Rationale

- Aligns with Samakia security baseline
- Reduces secret exposure on shared workstations

### Consequences

- Users re-enter secrets as needed
- Export/import flows must keep redaction intact

---

## ADR-0004 — pTerminal as the Official Samakia Access Tool

**Status:** Accepted  
**Date:** 2026-01-02

### Decision

pTerminal is the official tool for connecting to and verifying nodes in Samakia
Fabric and Samakia Platform environments, aligned with `samakia-specs`.

### Rationale

- Standardizes access tooling across dev and production
- Centralizes SSH/SFTP UX, guardrails, and audit workflows

### Consequences

- Host roles (`fabric`, `platform`) must be supported
- Roadmap and docs must track Samakia integration milestones
