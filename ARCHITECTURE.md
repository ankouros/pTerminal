# pTerminal Architecture

This document describes the current pTerminal architecture and how it aligns to
the Samakia ecosystem.

## Overview

pTerminal is a Go-based SSH client with a WebView UI. It provides persistent
terminal sessions, SFTP tooling, and LAN team sync in a single desktop app.

## Major Components

- **WebView UI**: HTML/CSS/JS rendered via `webview_go`. All UI state and
  actions flow through the RPC bridge to Go.
- **SSH/SFTP core**: `golang.org/x/crypto/ssh` sessions with reconnect logic,
  plus a Go-native SFTP client.
- **Config + persistence**: JSON config in `~/.config/pterminal/pterminal.json`,
  with team repos stored under `~/.config/pterminal/teams/<teamId>/`.
- **LAN sync**: Authenticated/encrypted P2P sync for teams and shared networks.

## Data Flow (High-Level)

1. UI dispatches actions through the RPC bridge.
2. Go handlers manage SSH/SFTP sessions and return results.
3. Terminal output is base64 encoded and rendered in xterm.js.
4. Config changes are normalized, redacted, and persisted atomically.

## Security Boundaries

- Credentials are memory-only; config and exports are redacted.
- Host key verification honors `known_hosts` or explicit `insecure` mode.
- LAN sync requires authentication and encryption by default.

## Samakia Alignment

- pTerminal is the official Samakia tool for connecting to Fabric/Platform nodes.
- Host roles (`fabric`, `platform`) tag Samakia nodes to support verification.
- Contracts and docs are synced from `samakia-specs` and updated with changes.

## Design and Production Strategy

pTerminal follows the Samakia Fabric design-production strategy:

- Design-first with explicit intent documented in ADRs.
- Incremental, reviewable changes with acceptance evidence.
- Production readiness validated by `make fmt`, `make vet`, and `go test ./...`.
