package ui

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ankouros/pterminal/internal/config"
	"github.com/ankouros/pterminal/internal/session"
	"github.com/ankouros/pterminal/internal/sshclient"
	webview "github.com/webview/webview_go"
	"golang.org/x/crypto/ssh"
)

//go:embed assets/*
var assets embed.FS

type Window struct {
	wv  webview.WebView
	mgr *session.Manager

	// pending host-key trust data
	pendingTrust map[int]pendingKey

	// per-host password cache in-memory only (NOT persisted)
	pwCache map[int]string
}

type pendingKey struct {
	hostPort    string
	fingerprint string
	key         ssh.PublicKey
}

type rpcReq struct {
	Type string `json:"type"`

	HostID int `json:"hostId,omitempty"`

	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`

	DataB64     string `json:"dataB64,omitempty"`     // input payload
	PasswordB64 string `json:"passwordB64,omitempty"` // optional password for connect
	Config      any    `json:"config,omitempty"`      // config_save payload
}

type rpcResp map[string]any

func ok(extra rpcResp) string {
	if extra == nil {
		extra = rpcResp{}
	}
	extra["ok"] = true
	b, _ := json.Marshal(extra)
	return string(b)
}

func fail(code string, extra rpcResp) string {
	if extra == nil {
		extra = rpcResp{}
	}
	extra["ok"] = false
	extra["error"] = code
	b, _ := json.Marshal(extra)
	return string(b)
}

func NewWindow(mgr *session.Manager) (*Window, error) {
	if mgr == nil {
		return nil, fmt.Errorf("ui: session manager is nil")
	}

	w := &Window{
		wv:           webview.New(true),
		mgr:          mgr,
		pendingTrust: make(map[int]pendingKey),
		pwCache:      make(map[int]string),
	}
	if w.wv == nil {
		return nil, fmt.Errorf("ui: failed to create webview")
	}

	w.wv.SetTitle("pTerminal")
	w.wv.SetSize(1200, 800, webview.HintNone)

	// Single RPC entrypoint expected by app.js: window.rpc(JSON.stringify(req)) -> JSON string
	w.wv.Bind("rpc", func(payload string) string {
		var req rpcReq
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return fail("bad_request", rpcResp{"detail": err.Error()})
		}

		switch req.Type {
		case "config_get":
			cfg, _, err := config.EnsureConfig()
			if err != nil {
				return fail("config_load_failed", rpcResp{"detail": err.Error()})
			}
			// keep manager in sync
			w.mgr.SetConfig(cfg)
			return ok(rpcResp{"config": cfg})

		case "config_save":
			// req.Config is generic; re-marshal to strong type by roundtrip
			raw, err := json.Marshal(req.Config)
			if err != nil {
				return fail("config_save_failed", rpcResp{"detail": err.Error()})
			}
			var cfgAny map[string]any
			_ = json.Unmarshal(raw, &cfgAny)

			// decode into your model via config.Load/Save path:
			// easiest: unmarshal into the same struct used by manager.Config()
			current := w.mgr.Config()
			if err := json.Unmarshal(raw, &current); err != nil {
				return fail("config_save_failed", rpcResp{"detail": err.Error()})
			}

			if err := config.Save(current); err != nil {
				return fail("config_save_failed", rpcResp{"detail": err.Error()})
			}
			w.mgr.SetConfig(current)
			return ok(nil)

		case "config_export":
			path, err := config.ExportToDownloads()
			if err != nil {
				return fail("export_failed", rpcResp{"detail": err.Error()})
			}
			return ok(rpcResp{"path": path})

		case "select":
			// optional password provided by UI (base64 UTF-8)
			if req.PasswordB64 != "" {
				pw, err := b64dec(req.PasswordB64)
				if err != nil {
					return fail("bad_password_b64", rpcResp{"detail": err.Error()})
				}
				w.pwCache[req.HostID] = pw
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			_, err := w.mgr.Ensure(ctx, req.HostID, req.Cols, req.Rows, func(hostID int) (string, error) {
				if pw, ok := w.pwCache[hostID]; ok && pw != "" {
					return pw, nil
				}
				// do NOT persist; UI should supply it (prompt).
				return "", errors.New("password_required")
			})
			if err != nil {
				var unk sshclient.ErrUnknownHostKey
				if errors.As(err, &unk) {
					w.pendingTrust[req.HostID] = pendingKey{
						hostPort:    unk.HostPort,
						fingerprint: unk.Fingerprint,
						key:         unk.Key,
					}
					return fail("unknown_host_key", rpcResp{
						"hostPort":    unk.HostPort,
						"fingerprint": unk.Fingerprint,
					})
				}

				// surface password required explicitly so JS can prompt
				if err.Error() == "password_required" {
					return fail("password_required", nil)
				}

				return fail("connect_failed", rpcResp{"detail": err.Error()})
			}

			// start output pump for this host
			go w.attachOutput(req.HostID)

			return ok(nil)

		case "input":
			if err := w.mgr.Write(req.HostID, req.DataB64); err != nil {
				return fail("write_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "resize":
			if err := w.mgr.Resize(req.HostID, req.Cols, req.Rows); err != nil {
				return fail("resize_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "state":
			info := w.mgr.SessionInfo(req.HostID)
			stateStr := "disconnected"
			switch info.State {
			case session.StateConnected:
				stateStr = "connected"
			case session.StateReconnecting:
				stateStr = "reconnecting"
			default:
				stateStr = "disconnected"
			}
			return ok(rpcResp{
				"state":    stateStr,
				"attempts": info.Attempts,
				"lastErr":  info.LastErr,
			})

		case "trust_host":
			p, okTrust := w.pendingTrust[req.HostID]
			if !okTrust || p.key == nil || p.hostPort == "" {
				return fail("no_pending_trust", nil)
			}
			if err := sshclient.TrustHostKey(p.hostPort, p.key); err != nil {
				return fail("trust_failed", rpcResp{"detail": err.Error()})
			}
			delete(w.pendingTrust, req.HostID)
			return ok(nil)

		case "about":
			return ok(rpcResp{
				"text": "pTerminal\nGo + webview + xterm.js\nConfig: ~/.config/pterminal/pterminal.json\nExport: ~/Downloads/pterminal-config-*.json",
			})

		default:
			return fail("unknown_rpc", rpcResp{"type": req.Type})
		}
	})

	// Load fully-inlined HTML (critical for WebView)
	html, err := w.buildInlinedHTML()
	if err != nil {
		return nil, err
	}
	w.wv.SetHtml(html)

	return w, nil
}

func (w *Window) attachOutput(hostID int) {
	// Drain buffered output (if any) first
	for _, chunk := range w.mgr.DrainBuffered(hostID) {
		w.pushPTY(hostID, chunk)
	}

	// Then stream live output by watching session output channel via manager's buffering hook:
	// Current manager buffers output when session reads happen.
	// The simplest robust pattern here is to poll-drain frequently (low CPU).
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()

	for range t.C {
		b := w.mgr.DrainBuffered(hostID)
		for _, chunk := range b {
			w.pushPTY(hostID, chunk)
		}
		// if no session exists anymore, we still keep ticking; status polling handles UI.
	}
}

func (w *Window) pushPTY(hostID int, data []byte) {
	b64 := base64.StdEncoding.EncodeToString(data)
	js := fmt.Sprintf(
		"window.dispatchPTY && window.dispatchPTY(%d, %q);",
		hostID,
		b64,
	)

	w.wv.Dispatch(func() {
		w.wv.Eval(js)
	})
}

func (w *Window) buildInlinedHTML() (string, error) {
	index, err := assets.ReadFile("assets/index.html")
	if err != nil {
		return "", err
	}
	appCSS, _ := assets.ReadFile("assets/app.css")
	xtermCSS, _ := assets.ReadFile("assets/vendor/xterm.css")
	appJS, _ := assets.ReadFile("assets/app.js")
	xtermJS, _ := assets.ReadFile("assets/vendor/xterm.js")
	fitJS, _ := assets.ReadFile("assets/vendor/xterm-addon-fit.js")

	s := string(index)

	// Replace external CSS links with inlined styles
	s = strings.ReplaceAll(s,
		`<link rel="stylesheet" href="vendor/xterm.css" />`,
		"<style>\n"+string(xtermCSS)+"\n</style>",
	)
	s = strings.ReplaceAll(s,
		`<link rel="stylesheet" href="app.css" />`,
		"<style>\n"+string(appCSS)+"\n</style>",
	)

	// Replace external scripts with inline scripts
	s = strings.ReplaceAll(s,
		`<script src="vendor/xterm.js"></script>`,
		"<script>\n"+string(xtermJS)+"\n</script>",
	)
	s = strings.ReplaceAll(s,
		`<script src="vendor/xterm-addon-fit.js"></script>`,
		"<script>\n"+string(fitJS)+"\n</script>",
	)
	s = strings.ReplaceAll(s,
		`<script src="app.js"></script>`,
		"<script>\n"+string(appJS)+"\n</script>",
	)

	return s, nil
}

func (w *Window) Run() { w.wv.Run() }

func (w *Window) Close() {
	if w.wv != nil {
		w.wv.Destroy()
	}
}

func b64dec(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
