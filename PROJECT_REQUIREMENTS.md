# pTerminal – Project Requirements & Design Constraints

This repository scaffolds the pTerminal application per the locked architecture:
Go + WebView + xterm.js + native Go SSH, with embedded assets and persistent per-node sessions.

Key constraints:
- No native terminal emulators
- No X11 embedding tricks
- No GTK/Qt/Electron/Node/Python GUI frameworks
- No SSH X11 forwarding requirement
- Single self-contained binary
