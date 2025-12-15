package ui

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ankouros/pterminal/internal/config"
	"github.com/ankouros/pterminal/internal/session"
	"github.com/ankouros/pterminal/internal/sftpclient"
	"github.com/ankouros/pterminal/internal/sshclient"
	"github.com/ankouros/pterminal/internal/terminal"
	webview "github.com/webview/webview_go"
	"golang.org/x/crypto/ssh"
)

//go:embed assets/*
var assets embed.FS

type Window struct {
	wv  webview.WebView
	mgr *session.Manager
	sftp *sftpclient.Manager

	// pending host-key trust data
	pendingTrust map[int]pendingKey

	// per-host password cache (memory only)
	pwCache map[int]string

	activeHostID atomic.Int64

	attachedMu sync.Mutex
	attached   map[int]terminal.Session

	flushCancel context.CancelFunc
	flushCh     chan struct{}
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

	Path string `json:"path,omitempty"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Dir  string `json:"dir,omitempty"`
	Name string `json:"name,omitempty"`

	UploadID string `json:"uploadId,omitempty"`
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
		sftp:         sftpclient.NewManager(mgr.Config()),
		pendingTrust: make(map[int]pendingKey),
		pwCache:      make(map[int]string),
		attached:     make(map[int]terminal.Session),
		flushCh:      make(chan struct{}, 1),
	}

	w.wv.SetTitle("pTerminal")
	w.wv.SetSize(1200, 800, webview.HintNone)
	w.wv.Dispatch(func() { w.setNativeIcon() })

	w.wv.Bind("rpc", func(payload string) string {
		var req rpcReq
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return fail("bad_request", nil)
		}

		// Opportunistic password cache (used by SSH + SFTP ops). JS can supply
		// PasswordB64 to avoid prompting repeatedly.
		if req.HostID != 0 && req.PasswordB64 != "" {
			if pw, err := b64dec(req.PasswordB64); err == nil && pw != "" {
				w.pwCache[req.HostID] = pw
			}
		}

		switch req.Type {

		case "config_get":
			cfg, _, err := config.EnsureConfig()
			if err != nil {
				return fail("config_load_failed", nil)
			}
			w.mgr.SetConfig(cfg)
			w.sftp.SetConfig(cfg)
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
			w.sftp.SetConfig(current)
			return ok(nil)

		case "config_export":
			path, err := config.ExportToDownloads()
			if err != nil {
				return fail("export_failed", nil)
			}
			return ok(rpcResp{"path": path})

		case "config_import_pick":
			path := w.pickConfigImportPath()
			if path == "" {
				return ok(rpcResp{"canceled": true})
			}

			cfg, backup, err := config.ImportFromFile(path)
			if err != nil {
				return fail("import_failed", rpcResp{"detail": err.Error()})
			}

			// Imported config might invalidate existing sessions/IDs; disconnect all.
			w.mgr.DisconnectAll()
			w.sftp.DisconnectAll()

			w.pendingTrust = make(map[int]pendingKey)
			w.pwCache = make(map[int]string)
			w.activeHostID.Store(0)

			w.mgr.SetConfig(cfg)
			w.sftp.SetConfig(cfg)
			return ok(rpcResp{
				"config":      cfg,
				"importPath":  path,
				"backupPath":  backup,
				"disconnected": true,
			})

		case "sftp_ls":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			entries, cwd, err := w.sftp.List(ctx, req.HostID, req.Path, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
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
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(rpcResp{"cwd": cwd, "entries": entries})

		case "sftp_mkdir":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := w.sftp.MkdirAll(ctx, req.HostID, req.Path, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_rm":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := w.sftp.Remove(ctx, req.HostID, req.Path, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_mv":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := w.sftp.Rename(ctx, req.HostID, req.From, req.To, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_download":
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := w.sftp.DownloadToDownloads(ctx, req.HostID, req.Path, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			})
			if err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(rpcResp{"localPath": out})

		case "sftp_upload_begin":
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			id, err := w.sftp.BeginUpload(ctx, req.HostID, req.Dir, req.Name, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			})
			if err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(rpcResp{"uploadId": id})

		case "sftp_upload_chunk":
			if req.UploadID == "" || req.DataB64 == "" {
				return fail("bad_request", nil)
			}
			data, err := base64.StdEncoding.DecodeString(req.DataB64)
			if err != nil {
				return fail("bad_request", nil)
			}
			if err := w.sftp.UploadChunk(req.UploadID, data); err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_upload_end":
			if req.UploadID == "" {
				return fail("bad_request", nil)
			}
			if err := w.sftp.EndUpload(req.UploadID); err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_read":
			if req.HostID == 0 || req.Path == "" {
				return fail("bad_request", nil)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			const max = 2 * 1024 * 1024
			b, err := w.sftp.ReadFile(ctx, req.HostID, req.Path, max, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			})
			if err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(rpcResp{"dataB64": base64.StdEncoding.EncodeToString(b)})

		case "sftp_write":
			if req.HostID == 0 || req.Path == "" || req.DataB64 == "" {
				return fail("bad_request", nil)
			}
			data, err := base64.StdEncoding.DecodeString(req.DataB64)
			if err != nil {
				return fail("bad_request", nil)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := w.sftp.WriteFile(ctx, req.HostID, req.Path, data, func(hostID int) (string, error) {
				if pw := w.pwCache[hostID]; pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

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

			w.activeHostID.Store(int64(req.HostID))
			w.ensureAttached(req.HostID, sess)
			_ = w.flushHost(req.HostID)

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
			if err := w.mgr.Resize(req.HostID, req.Cols, req.Rows); err != nil {
				return fail("resize_failed", rpcResp{"detail": err.Error()})
			}
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

		case "disconnect":
			if req.HostID == 0 {
				return fail("bad_request", nil)
			}
			if err := w.mgr.Disconnect(req.HostID); err != nil {
				return fail("disconnect_failed", rpcResp{"detail": err.Error()})
			}
			w.sftp.Disconnect(req.HostID)
			return ok(nil)

		case "ioshell_pick":
			// Bind handlers are invoked on the UI thread in this build; dispatching
			// back to the UI thread and waiting would deadlock.
			return ok(rpcResp{"path": w.pickIOShellExecutablePath()})

		case "about":
			return ok(rpcResp{"text": "pTerminal – SSH Terminal Manager"})

		default:
			return fail("unknown_rpc", nil)
		}
	})

	html, _ := w.buildInlinedHTML()
	w.wv.SetHtml(html)

	w.startPTYFlushLoop()
	return w, nil
}

func (w *Window) attachOutput(hostID int, sess terminal.Session) {
	for chunk := range sess.Output() {
		w.mgr.BufferOutput(hostID, chunk)
		if int(w.activeHostID.Load()) == hostID {
			w.kickFlush()
		}
	}
}

func (w *Window) ensureAttached(hostID int, sess terminal.Session) {
	w.attachedMu.Lock()
	defer w.attachedMu.Unlock()

	if w.attached[hostID] == sess {
		return
	}
	w.attached[hostID] = sess
	go w.attachOutput(hostID, sess)
}

func (w *Window) kickFlush() {
	select {
	case w.flushCh <- struct{}{}:
	default:
	}
}

func (w *Window) startPTYFlushLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	w.flushCancel = cancel

	go func() {
		// Event-driven flush: coalesce bursts and avoid a constant high-frequency ticker
		// that competes with WebView's UI thread.
		timer := time.NewTimer(time.Hour)
		if !timer.Stop() {
			<-timer.C
		}
		pending := false

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.flushCh:
				if pending {
					continue
				}
				pending = true
				timer.Reset(8 * time.Millisecond)
			case <-timer.C:
				pending = false
				hostID := int(w.activeHostID.Load())
				if hostID != 0 {
					_ = w.flushHost(hostID)
				}
			}
		}
	}()
}

func (w *Window) flushHost(hostID int) bool {
	const maxBytesPerEval = 256 * 1024

	chunks := w.mgr.DrainBuffered(hostID)
	if len(chunks) == 0 {
		return false
	}

	buf := make([]byte, 0, 32*1024)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		w.pushPTY(hostID, buf)
		buf = buf[:0]
	}

	for _, c := range chunks {
		if len(c) > maxBytesPerEval {
			flush()
			for start := 0; start < len(c); start += maxBytesPerEval {
				end := start + maxBytesPerEval
				if end > len(c) {
					end = len(c)
				}
				w.pushPTY(hostID, c[start:end])
			}
			continue
		}

		if len(buf)+len(c) > maxBytesPerEval {
			flush()
		}
		buf = append(buf, c...)
	}
	flush()
	return true
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
	logoSVG, _ := assets.ReadFile("assets/logo.svg")
	appJS, _ := assets.ReadFile("assets/app.js")
	xtermJS, _ := assets.ReadFile("assets/vendor/xterm.js")
	fitJS, _ := assets.ReadFile("assets/vendor/xterm-addon-fit.js")
	clipboardJS, _ := assets.ReadFile("assets/vendor/xterm-addon-clipboard.js")
	searchJS, _ := assets.ReadFile("assets/vendor/xterm-addon-search.js")
	webLinksJS, _ := assets.ReadFile("assets/vendor/xterm-addon-web-links.js")
	webglJS, _ := assets.ReadFile("assets/vendor/xterm-addon-webgl.js")
	serializeJS, _ := assets.ReadFile("assets/vendor/xterm-addon-serialize.js")
	unicode11JS, _ := assets.ReadFile("assets/vendor/xterm-addon-unicode11.js")
	ligaturesJS, _ := assets.ReadFile("assets/vendor/xterm-addon-ligatures.js")
	imageJS, _ := assets.ReadFile("assets/vendor/xterm-addon-image.js")

	s := string(index)
	s = strings.ReplaceAll(s, `<link rel="stylesheet" href="vendor/xterm.css" />`, "<style>"+string(xtermCSS)+"</style>")
	s = strings.ReplaceAll(s, `<link rel="stylesheet" href="app.css" />`, "<style>"+string(appCSS)+"</style>")
	if len(logoSVG) > 0 {
		logoURI := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(logoSVG)
		s = strings.ReplaceAll(s, "__PTERMINAL_LOGO__", logoURI)
	}
	s = strings.ReplaceAll(s, `<script src="vendor/xterm.js"></script>`, "<script>"+string(xtermJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-fit.js"></script>`, "<script>"+string(fitJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-clipboard.js"></script>`, "<script>"+string(clipboardJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-search.js"></script>`, "<script>"+string(searchJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-web-links.js"></script>`, "<script>"+string(webLinksJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-webgl.js"></script>`, "<script>"+string(webglJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-serialize.js"></script>`, "<script>"+string(serializeJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-unicode11.js"></script>`, "<script>"+string(unicode11JS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-ligatures.js"></script>`, "<script>"+string(ligaturesJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="vendor/xterm-addon-image.js"></script>`, "<script>"+string(imageJS)+"</script>")
	s = strings.ReplaceAll(s, `<script src="app.js"></script>`, "<script>"+string(appJS)+"</script>")
	return s, nil
}

func (w *Window) Run() { w.wv.Run() }
func (w *Window) Close() {
	if w.flushCancel != nil {
		w.flushCancel()
	}
	if w.sftp != nil {
		w.sftp.DisconnectAll()
	}
	w.wv.Destroy()
}

func b64dec(b string) (string, error) {
	d, err := base64.StdEncoding.DecodeString(b)
	return string(d), err
}
