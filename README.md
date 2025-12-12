# pTerminal

**pTerminal** is a lightweight, cross-distribution Linux application that provides a **persistent, multi-node SSH terminal UI** with a modern graphical interface.

It is designed for operators and developers who need reliable, long-lived SSH sessions without relying on system terminal emulators, X11 embedding, or distro-specific GUI toolkits.

---

## ✨ Key Features

- Persistent SSH sessions (one per node)
- Instant switching between nodes without reconnecting
- Modern, responsive GUI
- Built-in terminal emulator (xterm.js)
- Works on X11, Wayland, SSH sessions, VMs, containers
- Single self-contained Go binary
- No dependency on system terminals or X11 embedding

---

## 🧠 Design Philosophy

pTerminal is built around a simple but strict idea:

> **Never embed native terminals or depend on the host environment.**

Instead, it uses:
- **Go** for portability and static builds
- **WebView** for a lightweight native GUI
- **xterm.js** for a full-featured terminal emulator
- **Native Go SSH** for session management

This results in a robust application that behaves consistently across Linux distributions and environments.

---

## 🏗 Architecture Overview

```
┌────────────────────────────────────┐
│            Go Application           │
│                                    │
│  ┌──────────── WebView ──────────┐ │
│  │ HTML / CSS / JS                │ │
│  │  ┌───────────────┐             │ │
│  │  │  xterm.js     │◀───────┐    │ │
│  │  └───────────────┘        │    │ │
│  └──────────────▲────────────┘    │ │
│                 │ JS ↔ Go Bridge   │ │
│                 ▼                  │ │
│  ┌──────────────────────────────┐ │ │
│  │ SSH + PTY Session Manager     │ │ │
│  │  • one session per node       │ │ │
│  │  • persistent connections     │ │ │
│  └──────────────────────────────┘ │ │
└────────────────────────────────────┘
```

---

## 📦 Installation

### Prerequisites

- Go (official binary distribution recommended)

### Build

```bash
go build -o pterminal
```

### Run

```bash
./pterminal
```

No additional runtime dependencies are required.

---

## 🔐 SSH Authentication

Supported / planned authentication methods:

1. Password (prototype / testing only)
2. SSH keys
3. SSH agent support

Credentials must **never** be hard-coded in production.

---

## 🎯 Project Status

- ✅ Core architecture defined
- ✅ Minimal working prototype
- 🔧 Binary-safe PTY streaming (in progress)
- 🔧 Multi-node management (in progress)
- 🔧 Resize propagation (planned)

---

## 🚧 Non-Goals

pTerminal intentionally avoids:

- X11 `-into` embedding
- External terminal emulators
- GTK / Qt heavy frameworks
- Electron / Node.js
- tmux dependency

---

## 📄 Documentation

- See `PROJECT_REQUIREMENTS.md` for full design constraints and requirements.

---

## 📝 License

TBD
