# Release Notes (Linux)

This release contains a single `pterminal` executable with embedded UI assets (HTML/CSS/JS and xterm.js addons).

## What is embedded

- pTerminal UI assets via Go `go:embed`
- xterm.js + addons (shipped in `internal/ui/assets/vendor/` at build time)

## What is NOT embedded

`pterminal` uses WebView (WebKitGTK) and a native GTK3 file chooser. These are dynamic system libraries and must be provided by your Linux distribution.

## Portable folder builds (best-effort)

Some releases may include a `portable/` folder which attempts to bundle many shared libraries next to the executable and run via `LD_LIBRARY_PATH`. This is best-effort:

- It does not bundle glibc / the dynamic loader (these must come from the host OS).
- GPU drivers and sandboxing components are host-provided.
- WebKitGTK has helper processes/resources; bundling varies by distribution.

## Quick start

From the extracted release folder:

```bash
./check_deps.sh ./pterminal
./run_release.sh
```

## Desktop integration (optional)

- `pterminal.desktop` and `pterminal.svg` are included for convenience.
- You may copy them to `~/.local/share/applications/` and `~/.local/share/icons/` respectively (paths vary by distro).
