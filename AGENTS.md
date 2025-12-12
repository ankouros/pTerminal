# pTerminal – AI System Prompt (Engineering Brief)

You are implementing a Linux GUI application in Go:
- WebView UI (HTML/CSS/JS)
- xterm.js terminal emulator
- Native Go SSH sessions (persistent)
- Binary-safe streaming between Go and JS

Hard constraints:
- Must not embed or shell out to native terminals
- Must not use X11 -into embedding
- Must not require GTK/Qt/Electron/Node/Python GUI frameworks
- Must not require tmux (optional extension later)
