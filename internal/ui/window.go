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

	// per-host password cache (memory only)
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
	Cols   int `json:"cols,omitempty"`
	Rows   int `json:"rows,omitempty"`

	DataB64     string `json:"dataB64,omitempty"`
	PasswordB64 string `json:"passwordB64,omitempty"`
	Config      any    `json:"config,omitempty"`
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
	w := &Window{
		wv:           webview.New(true),
		mgr:          mgr,
		pendingTrust: make(map[int]pendingKey),
		pwCache:      make(map[int]string),
	}

	w.wv.SetTitle("pTerminal")
	w.wv.SetSize(1200, 800, webview.HintNone)

	w.wv.Bind("rpc", func(payload string) string {
		var req rpcReq
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return fail("bad_request", nil)
		}

		switch req.Type {

		case "config_get":
			cfg, _, err := config.EnsureConfig()
			if err != nil {
				return fail("config_load_failed", nil)
			}
			w.mgr.SetConfig(cfg)
			return ok(rpcResp{"config": cfg})

		case "config_save":
			raw, _ := json.Marshal(req.Config)
			current := w.mgr.Config()
			if err := json.Unmarshal(raw, &current); err != nil {
				return fail("config_save_failed", nil)
			}
			if err := config.Save(current); err != nil {
				return fail("config_save_failed", nil)
			}
			w.mgr.SetConfig(current)
			return ok(nil)

		case "config_export":
			path, err := config.ExportToDownloads()
			if err != nil {
				return fail("export_failed", nil)
			}
			return ok(rpcResp{"path": path})

		case "select":
			// Cache password if provided (from config or prompt)
			if req.PasswordB64 != "" {
				if pw, err := b64dec(req.PasswordB64); err == nil && pw != "" {
					// never overwrite a valid cached password with empty data
					w.pwCache[req.HostID] = pw
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			sess, err := w.mgr.Ensure(ctx, req.HostID, req.Cols, req.Rows, func(id int) (string, error) {
				if pw := w.pwCache[id]; pw != "" {
					return pw, nil
				}
				// only request password if NONE was ever provided
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

				var mismatch sshclient.ErrHostKeyMismatch
				if errors.As(err, &mismatch) {
					w.pendingTrust[req.HostID] = pendingKey{
						hostPort:    mismatch.HostPort,
						fingerprint: mismatch.Fingerprint,
						key:         mismatch.Key,
					}
					return fail("host_key_mismatch", rpcResp{
						"hostPort":    mismatch.HostPort,
						"fingerprint": mismatch.Fingerprint,
					})
				}

				if err.Error() == "password_required" {
					return fail("password_required", nil)
				}

				return fail("connect_failed", rpcResp{"detail": err.Error()})
			}

			// ✅ attach output ONLY after successful session
			go w.attachOutput(req.HostID, sess)

			return ok(nil)

		case "trust_host":
			p := w.pendingTrust[req.HostID]
			if p.key == nil {
				return fail("no_pending_trust", nil)
			}
			if err := sshclient.TrustHostKey(p.hostPort, p.key); err != nil {
				return fail("trust_failed", nil)
			}
			delete(w.pendingTrust, req.HostID)
			return ok(nil)

		case "input":
			if req.HostID == 0 || req.DataB64 == "" {
				return fail("bad_input", nil)
			}

			if err := w.mgr.Write(req.HostID, req.DataB64); err != nil {
				return fail("write_failed", rpcResp{
					"detail": err.Error(),
				})
			}

			return ok(nil)

		case "resize":
			return ok(nil)

		case "state":
			info := w.mgr.SessionInfo(req.HostID)
			state := "disconnected"
			if info.State == session.StateConnected {
				state = "connected"
			} else if info.State == session.StateReconnecting {
				state = "reconnecting"
			}
			return ok(rpcResp{
				"state":    state,
				"attempts": info.Attempts,
			})

		case "about":
			return ok(rpcResp{"text": "pTerminal – SSH Terminal Manager"})

		default:
			return fail("unknown_rpc", nil)
		}
	})

	html, _ := w.buildInlinedHTML()
	w.wv.SetHtml(html)
	return w, nil
}

func (w *Window) attachOutput(hostID int, sess *sshclient.NodeSession) {
	for chunk := range sess.Output {
		w.pushPTY(hostID, chunk)
	}
}

func (w *Window) pushPTY(hostID int, data []byte) {
	b64 := base64.StdEncoding.EncodeToString(data)
	js := fmt.Sprintf("window.dispatchPTY(%d,%q)", hostID, b64)
	w.wv.Dispatch(func() { w.wv.Eval(js) })
}

func (w *Window) buildInlinedHTML() (string, error) {
	index, _ := assets.ReadFile("assets/index.html")
	appCSS, _ := assets.ReadFile("assets/app.css")
	xtermCSS, _ := assets.ReadFile("assets/vendor/xterm.css")
	appJS, _ := assets.ReadFile("assets/app.js")
	xtermJS, _ := assets.ReadFile("assets/vendor/xterm.js")
	fitJS, _ := assets.ReadFile("assets/vendor/xterm-addon-fit.js")

	s := string(index)
	s = strings.ReplaceAll(s, `<link rel="stylesheet" href="vendor/xterm.css" />`, "<style>"+string(xtermCSS)+"</style>")
	s = strings.ReplaceAll(s, `<link rel="stylesheet" href="app.css" />`, "<style>"+string(appCSS)+"</style>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm.js"></script>`, "<script>"+string(xtermJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-fit.js"></script>`, "<script>"+string(fitJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="app.js"></script>`, "<script>"+string(appJS)+"</script>")
	return s, nil
}

func (w *Window) Run()   { w.wv.Run() }
func (w *Window) Close() { w.wv.Destroy() }

func b64dec(b string) (string, error) {
	d, err := base64.StdEncoding.DecodeString(b)
	return string(d), err
}
