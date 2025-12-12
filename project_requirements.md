# pTerminal – Project Requirements & Design Constraints

## 1. Project Overview

**pTerminal** is a lightweight, cross-distribution Linux application that provides a **persistent, multi-node SSH terminal UI** with a modern graphical interface.

The application is intended for infrastructure operators and developers who need to maintain long-lived terminal sessions across multiple remote nodes, with instant switching and zero dependency on system terminal emulators or fragile X11 embedding.

---

## 2. Core Goals

1. **Reliability**
   - Must work consistently on:
     - X11
     - Wayland
     - SSH sessions (e.g. `DISPLAY=:10.0`)
     - VMs and containers
   - Must not rely on X11 window embedding or external terminals.

2. **Portability**
   - Must run on most Linux distributions without distro-specific packages.
   - No runtime dependency on:
     - GTK / Qt toolkits
     - system terminal emulators
     - Xpra, VNC, tmux, etc.

3. **Minimal Dependencies**
   - Prefer a **single self-contained binary**.
   - All UI assets must be shipped inside the project/binary.
   - No external services required at runtime.

4. **Modern UI**
   - Appealing, responsive GUI.
   - Proper resize behavior.
   - Dark-theme friendly.
   - Keyboard and mouse friendly.

5. **Persistent SSH Sessions**
   - Each node maintains its own long-lived SSH session.
   - Switching nodes must not reconnect or reset state.
   - Command history and shell state must persist per node.

---

## 3. Explicit Non-Goals (Hard Constraints)

The application **must NOT**:

- Embed native terminals (`xterm`, `gnome-terminal`, etc.).
- Depend on X11 `-into` embedding or XIDs.
- Require SSH X11 forwarding to function.
- Depend on system-installed GUI frameworks.
- Depend on Snap / Flatpak / distro packaging at runtime.
- Require tmux (optional later, not required).

---

## 4. Technology Choices (Locked)

### Language
- **Go**

### GUI
- **WebView** (HTML/CSS/JS rendered in a native window)
- No Electron
- No Node.js
- No Python GUI frameworks

### Terminal Emulator
- **xterm.js**
- Runs entirely inside the application UI.
- Handles:
  - UTF-8
  - Scrollback
  - Clipboard
  - Keyboard input
  - Resize events

### SSH
- Native Go SSH implementation:
  - `golang.org/x/crypto/ssh`
- One SSH session per node.
- No reliance on external `ssh` binary.

---

## 5. High-Level Architecture

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

## 6. Functional Requirements

### 6.1 Node Management
- Nodes defined by:
  - `id`
  - `name`
  - `host`
  - `port`
  - `user`
  - `auth method`
- Nodes displayed in a **left-hand vertical list**, ordered by `id`.

### 6.2 Terminal View
- Right pane displays the terminal for the selected node.
- Terminal must:
  - Occupy **100% of available space**
  - Resize dynamically with the window
  - Preserve state when switching nodes

### 6.3 Session Handling
- SSH session is created on first selection.
- Session remains alive even when not visible.
- Switching nodes only switches the attached PTY stream.
- No reconnect unless explicitly requested (future feature).

### 6.4 Input / Output Handling
- Terminal I/O must be **binary-safe**.
- No string escaping hacks.
- Data must support:
  - ANSI escape sequences
  - Unicode
  - Full shell interactivity

### 6.5 Resize Handling
- UI resize events must propagate to:
  - xterm.js terminal
  - SSH PTY window size

---

## 7. UI / UX Requirements

- Layout:
  - Left: fixed-width node list
  - Right: flexible terminal pane
- Appearance:
  - Dark theme by default
  - Monospace font
- Interaction:
  - Click to switch nodes
  - Keyboard focus follows terminal
  - Clipboard copy/paste works

---

## 8. Packaging & Distribution

- Output artifact:
  - Single executable binary
- Build:
  - Standard Go toolchain
- Assets:
  - HTML / CSS / JS embedded into the binary (`go:embed`)
- Installation:
  - Copy binary → run
  - No installer required

---

## 9. Security Considerations

- SSH host key verification must be supported (configurable).
- Authentication methods (phased):
  1. Password (prototype only)
  2. SSH keys
  3. SSH agent support
- No credentials hard-coded in final version.

---

## 10. Future Extensions (Out of Scope for MVP)

- Session reconnect UI
- Status indicators (latency, connected/disconnected)
- Tabs or split terminals
- Config file support
- Windows / macOS support
- tmux integration (optional)

---

## 11. Definition of Success

The project is considered successful when:

- It runs reliably on multiple Linux distributions.
- It works identically on local desktops and SSH sessions.
- No X11 embedding issues occur.
- Switching between nodes is instant and stateful.
- The entire app is delivered as a single binary with no runtime dependencies.

---

**End of requirements.**