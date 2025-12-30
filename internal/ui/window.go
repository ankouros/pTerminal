package ui

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ankouros/pterminal/internal/buildinfo"
	"github.com/ankouros/pterminal/internal/config"
	"github.com/ankouros/pterminal/internal/model"
	"github.com/ankouros/pterminal/internal/p2p"
	"github.com/ankouros/pterminal/internal/session"
	"github.com/ankouros/pterminal/internal/sftpclient"
	"github.com/ankouros/pterminal/internal/sshclient"
	"github.com/ankouros/pterminal/internal/teamrepo"
	"github.com/ankouros/pterminal/internal/terminal"
	"github.com/ankouros/pterminal/internal/update"
	webview "github.com/webview/webview_go"
	"golang.org/x/crypto/ssh"
)

//go:embed assets/*
var assets embed.FS

func validateWebView(wv webview.WebView) error {
	if wv == nil {
		return errors.New("webview init failed (nil)")
	}

	// webview_go does not surface errors from C.webview_create; when it fails it
	// can return a webview instance with a nil internal handle, which later
	// segfaults on any call (e.g. SetTitle). Detect this via reflection.
	defer func() {
		_ = recover()
	}()

	rv := reflect.ValueOf(wv)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}
	ev := rv.Elem()
	if ev.Kind() != reflect.Struct {
		return nil
	}
	f := ev.FieldByName("w")
	if !f.IsValid() {
		return nil
	}
	if f.Kind() == reflect.UnsafePointer && f.Pointer() == 0 {
		display := os.Getenv("DISPLAY")
		wayland := os.Getenv("WAYLAND_DISPLAY")
		if display == "" && wayland == "" {
			return errors.New("GUI init failed: DISPLAY/WAYLAND_DISPLAY not set")
		}
		return fmt.Errorf("GUI init failed (webview handle is nil); if running in Docker ensure display sockets + auth are passed (DISPLAY/Wayland/XAUTHORITY)")
	}
	return nil
}

func softwareRenderEnabled() bool {
	if os.Getenv("PTERMINAL_SOFTWARE_RENDER") == "1" {
		return true
	}
	if os.Getenv("PTERMINAL_DISABLE_GPU") == "1" {
		return true
	}
	if os.Getenv("LIBGL_ALWAYS_SOFTWARE") != "" {
		return true
	}
	if os.Getenv("WEBKIT_DISABLE_COMPOSITING_MODE") != "" {
		return true
	}
	return false
}

func enableSoftwareRendering() {
	if os.Getenv("LIBGL_ALWAYS_SOFTWARE") == "" {
		_ = os.Setenv("LIBGL_ALWAYS_SOFTWARE", "1")
	}
	if os.Getenv("WEBKIT_DISABLE_COMPOSITING_MODE") == "" {
		_ = os.Setenv("WEBKIT_DISABLE_COMPOSITING_MODE", "1")
	}
}

func shouldRetryWithSoftware(err error) bool {
	if err == nil {
		return false
	}
	if softwareRenderEnabled() {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "DISPLAY") || strings.Contains(msg, "Wayland") || strings.Contains(msg, "WAYLAND") {
		return false
	}
	return true
}

func renderNodeReadable() bool {
	nodes, _ := filepath.Glob("/dev/dri/renderD*")
	if len(nodes) == 0 {
		return true
	}
	for _, node := range nodes {
		f, err := os.Open(node)
		if err == nil {
			f.Close()
			return true
		}
		if os.IsPermission(err) {
			return false
		}
	}
	return true
}

func newWebViewWithFallback() (webview.WebView, error) {
	if softwareRenderEnabled() {
		enableSoftwareRendering()
	} else if !renderNodeReadable() {
		log.Printf("GPU render node is not accessible; enabling software rendering")
		enableSoftwareRendering()
	}

	wv := webview.New(true)
	if err := validateWebView(wv); err == nil {
		return wv, nil
	} else if shouldRetryWithSoftware(err) {
		log.Printf("webview init failed: %v; retrying with software rendering", err)
		enableSoftwareRendering()
		wv2 := webview.New(true)
		if err2 := validateWebView(wv2); err2 == nil {
			return wv2, nil
		} else {
			return nil, fmt.Errorf("webview init failed: %w (software rendering retry: %v)", err, err2)
		}
	} else {
		return nil, err
	}
}

type Window struct {
	wv   webview.WebView
	mgr  *session.Manager
	sftp *sftpclient.Manager
	p2p  *p2p.Service

	// pending host-key trust data
	pendingTrust map[int]pendingKey

	// per-host password cache (memory only)
	pwMu    sync.RWMutex
	pwCache map[int]string
	sftpPw  map[int]string

	activeHostID atomic.Int64
	activeTabID  atomic.Int64

	attachedMu sync.Mutex
	attached   map[attachKey]terminal.Session

	flushCancel context.CancelFunc
	flushCh     chan struct{}

	ioCancel context.CancelFunc
	inputCh  chan inputMsg
	resizeCh chan resizeMsg

	windowHidden atomic.Bool
	exePath      string

	update   updateState
	updateMu sync.Mutex
}

type pendingKey struct {
	kind        string
	hostPort    string
	fingerprint string
	key         ssh.PublicKey
}

type updateState struct {
	LatestTag    string
	ReleaseURL   string
	ReleaseNotes string
	AssetName    string
	AssetURL     string
	HasUpdate    bool
	Checking     bool
	Installing   bool
	Error        string
	LastCheck    time.Time
	InstalledTag string
}

func (w *Window) updatePayload() rpcResp {
	w.updateMu.Lock()
	defer w.updateMu.Unlock()

	info := rpcResp{
		"currentVersion": buildinfo.Version,
		"latest":         w.update.LatestTag,
		"releaseUrl":     w.update.ReleaseURL,
		"notes":          w.update.ReleaseNotes,
		"assetName":      w.update.AssetName,
		"assetUrl":       w.update.AssetURL,
		"hasUpdate":      w.update.HasUpdate,
		"checking":       w.update.Checking,
		"installing":     w.update.Installing,
		"error":          w.update.Error,
		"installedTag":   w.update.InstalledTag,
	}
	if !w.update.LastCheck.IsZero() {
		info["lastCheck"] = w.update.LastCheck.Format(time.RFC3339)
	}
	return info
}

func isNewVersion(latest, current string) bool {
	latest = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(latest)), "v")
	current = strings.TrimPrefix(strings.TrimSpace(strings.ToLower(current)), "v")
	if latest == "" {
		return false
	}
	if current == "" || current == "dev" {
		return true
	}
	return latest != current
}

func selectUpdateAsset(goos string, assets []update.Asset) (update.Asset, bool) {
	goos = strings.ToLower(goos)
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, goos) && strings.Contains(name, "portable") && (strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz")) {
			return asset, true
		}
	}
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "pterminal") && (strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz")) {
			return asset, true
		}
	}
	for _, asset := range assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, goos) {
			return asset, true
		}
	}
	return update.Asset{}, false
}

type inputMsg struct {
	hostID  int
	tabID   int
	dataB64 string
}

type resizeMsg struct {
	hostID int
	tabID  int
	cols   int
	rows   int
}

type attachKey struct {
	hostID int
	tabID  int
}

type rpcReq struct {
	Type string `json:"type"`

	HostID int `json:"hostId,omitempty"`
	TabID  int `json:"tabId,omitempty"`
	Cols   int `json:"cols,omitempty"`
	Rows   int `json:"rows,omitempty"`

	Force bool `json:"force,omitempty"`

	DataB64         string `json:"dataB64,omitempty"`
	PasswordB64     string `json:"passwordB64,omitempty"`
	SftpPasswordB64 string `json:"sftpPasswordB64,omitempty"`
	Config          any    `json:"config,omitempty"`

	Path string `json:"path,omitempty"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Dir  string `json:"dir,omitempty"`
	Name string `json:"name,omitempty"`

	UploadID string `json:"uploadId,omitempty"`
}

type rpcResp map[string]any

func (w *Window) getCachedPassword(hostID int) string {
	w.pwMu.RLock()
	defer w.pwMu.RUnlock()
	return w.pwCache[hostID]
}

func (w *Window) setCachedPassword(hostID int, pw string) {
	w.pwMu.Lock()
	w.pwCache[hostID] = pw
	w.pwMu.Unlock()
}

func (w *Window) getCachedSFTPPassword(hostID int) string {
	w.pwMu.RLock()
	defer w.pwMu.RUnlock()
	return w.sftpPw[hostID]
}

func (w *Window) setCachedSFTPPassword(hostID int, pw string) {
	w.pwMu.Lock()
	if pw == "" {
		delete(w.sftpPw, hostID)
	} else {
		w.sftpPw[hostID] = pw
	}
	w.pwMu.Unlock()
}

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

func NewWindow(mgr *session.Manager, p2pSvc *p2p.Service) (*Window, error) {
	wv, err := newWebViewWithFallback()
	if err != nil {
		return nil, err
	}

	w := &Window{
		wv:           wv,
		mgr:          mgr,
		sftp:         sftpclient.NewManager(mgr.Config()),
		p2p:          p2pSvc,
		pendingTrust: make(map[int]pendingKey),
		pwCache:      make(map[int]string),
		sftpPw:       make(map[int]string),
		attached:     make(map[attachKey]terminal.Session),
		flushCh:      make(chan struct{}, 1),
		inputCh:      make(chan inputMsg, 16384),
		resizeCh:     make(chan resizeMsg, 256),
	}
	if exe, err := os.Executable(); err == nil {
		w.exePath = exe
	} else {
		w.exePath = filepath.Join(".", "bin", "pterminal")
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
				w.setCachedPassword(req.HostID, pw)
			}
		}
		if req.HostID != 0 && req.SftpPasswordB64 != "" {
			if pw, err := b64dec(req.SftpPasswordB64); err == nil && pw != "" {
				w.setCachedSFTPPassword(req.HostID, pw)
				w.sftp.SetCustomPassword(req.HostID, pw)
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
			if w.p2p != nil {
				w.p2p.SetConfig(cfg)
			}
			return ok(rpcResp{"config": cfg})

		case "config_save":
			raw, _ := json.Marshal(req.Config)
			current := w.mgr.Config()
			var incoming model.AppConfig
			if err := json.Unmarshal(raw, &incoming); err != nil {
				return fail("config_save_failed", nil)
			}
			updated, _ := p2p.ApplyLocalEdits(current, incoming)
			_ = config.StripSecrets(&updated)
			if err := config.Save(updated); err != nil {
				return fail("config_save_failed", nil)
			}
			w.mgr.SetConfig(updated)
			w.sftp.SetConfig(updated)
			if w.p2p != nil {
				w.p2p.SetConfig(updated)
				w.p2p.SyncNow()
			}
			return ok(rpcResp{"config": updated})

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
			w.pwMu.Lock()
			w.pwCache = make(map[int]string)
			w.sftpPw = make(map[int]string)
			w.pwMu.Unlock()
			w.activeHostID.Store(0)
			w.activeTabID.Store(0)
			w.attachedMu.Lock()
			w.attached = make(map[attachKey]terminal.Session)
			w.attachedMu.Unlock()

			w.mgr.SetConfig(cfg)
			w.sftp.SetConfig(cfg)
			if w.p2p != nil {
				w.p2p.SetConfig(cfg)
				w.p2p.SyncNow()
			}
			return ok(rpcResp{
				"config":       cfg,
				"importPath":   path,
				"backupPath":   backup,
				"disconnected": true,
			})

		case "sftp_ls":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			entries, cwd, err := w.sftp.List(ctx, req.HostID, req.Path, func(hostID int) (string, error) {
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			})
			if err != nil {
				var unk sshclient.ErrUnknownHostKey
				if errors.As(err, &unk) {
					w.pendingTrust[req.HostID] = pendingKey{
						kind:        "unknown_host_key",
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
						kind:        "host_key_mismatch",
						hostPort:    mismatch.HostPort,
						fingerprint: mismatch.Fingerprint,
						key:         mismatch.Key,
					}
					return fail("host_key_mismatch", rpcResp{
						"hostPort":    mismatch.HostPort,
						"fingerprint": mismatch.Fingerprint,
					})
				}
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
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
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
				}
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_rm":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := w.sftp.Remove(ctx, req.HostID, req.Path, func(hostID int) (string, error) {
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
				}
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_mv":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := w.sftp.Rename(ctx, req.HostID, req.From, req.To, func(hostID int) (string, error) {
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
				}
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "sftp_download":
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			out, err := w.sftp.DownloadToDownloads(ctx, req.HostID, req.Path, func(hostID int) (string, error) {
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			})
			if err != nil {
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
				}
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(rpcResp{"localPath": out})

		case "sftp_upload_begin":
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			id, err := w.sftp.BeginUpload(ctx, req.HostID, req.Dir, req.Name, func(hostID int) (string, error) {
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			})
			if err != nil {
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
				}
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
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			})
			if err != nil {
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
				}
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
				if pw := w.getCachedSFTPPassword(hostID); pw != "" {
					return pw, nil
				}
				if pw := w.getCachedPassword(hostID); pw != "" {
					return pw, nil
				}
				return "", errors.New("password_required")
			}); err != nil {
				if errors.Is(err, sshclient.ErrPassphraseRequired) {
					return fail("passphrase_required", nil)
				}
				return fail("sftp_failed", rpcResp{"detail": err.Error()})
			}
			return ok(nil)

		case "select":
			hostID := req.HostID
			if hostID == 0 {
				return fail("bad_request", nil)
			}
			tabID := req.TabID
			if tabID <= 0 {
				tabID = 1
			}

			// Cache password if provided (from config or prompt)
			if req.PasswordB64 != "" {
				if pw, err := b64dec(req.PasswordB64); err == nil && pw != "" {
					// never overwrite a valid cached password with empty data
					w.setCachedPassword(hostID, pw)
				}
			}

			// Start connecting asynchronously to avoid blocking the WebView UI thread.
			sess, alreadyConnected, err := w.mgr.StartConnectAsync(hostID, tabID, req.Cols, req.Rows, func(id int) (string, error) {
				if pw := w.getCachedPassword(id); pw != "" {
					return pw, nil
				}
				// only request password if NONE was ever provided
				return "", errors.New("password_required")
			}, func(sess terminal.Session, err error) {
				if err != nil {
					var unk sshclient.ErrUnknownHostKey
					if errors.As(err, &unk) {
						w.wv.Dispatch(func() {
							w.pendingTrust[hostID] = pendingKey{
								kind:        "unknown_host_key",
								hostPort:    unk.HostPort,
								fingerprint: unk.Fingerprint,
								key:         unk.Key,
							}
						})
						return
					}

					var mismatch sshclient.ErrHostKeyMismatch
					if errors.As(err, &mismatch) {
						w.wv.Dispatch(func() {
							w.pendingTrust[hostID] = pendingKey{
								kind:        "host_key_mismatch",
								hostPort:    mismatch.HostPort,
								fingerprint: mismatch.Fingerprint,
								key:         mismatch.Key,
							}
						})
						return
					}
					return
				}

				if sess != nil {
					w.ensureAttached(hostID, tabID, sess)
					if int(w.activeHostID.Load()) == hostID && int(w.activeTabID.Load()) == tabID {
						w.kickFlush()
					}
				}
			})
			if err != nil {
				return fail("connect_failed", rpcResp{"detail": err.Error()})
			}

			w.activeHostID.Store(int64(hostID))
			w.activeTabID.Store(int64(tabID))
			if alreadyConnected && sess != nil {
				w.ensureAttached(hostID, tabID, sess)
				_ = w.flushHost(hostID, tabID)
			}

			return ok(rpcResp{"started": true})

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
			// Bind handlers are invoked on the UI thread in this build; never block
			// the UI thread on network/PTY writes. Enqueue to a worker instead.
			w.inputCh <- inputMsg{hostID: req.HostID, tabID: req.TabID, dataB64: req.DataB64}
			return ok(nil)

		case "resize":
			// Also avoid blocking the UI thread on SSH window-change requests.
			if req.HostID != 0 && req.Cols > 0 && req.Rows > 0 {
				select {
				case w.resizeCh <- resizeMsg{hostID: req.HostID, tabID: req.TabID, cols: req.Cols, rows: req.Rows}:
				default:
					// Best effort; a later resize will win.
				}
			}
			return ok(nil)

		case "state":
			info := w.mgr.SessionInfoTab(req.HostID, req.TabID)
			state := "disconnected"
			if info.State == session.StateConnected {
				state = "connected"
			} else if info.State == session.StateReconnecting {
				state = "reconnecting"
			}

			resp := rpcResp{
				"hostId":    req.HostID,
				"tabId":     req.TabID,
				"state":     state,
				"attempts":  info.Attempts,
				"detail":    info.LastErr,
				"connected": info.State == session.StateConnected,
			}

			// Host key trust hints (also used for reconnect failures)
			if p := w.pendingTrust[req.HostID]; p.key != nil {
				resp["errCode"] = p.kind
				resp["hostPort"] = p.hostPort
				resp["fingerprint"] = p.fingerprint
				return ok(resp)
			}

			if info.Err != nil {
				var unk sshclient.ErrUnknownHostKey
				if errors.As(info.Err, &unk) {
					w.pendingTrust[req.HostID] = pendingKey{
						kind:        "unknown_host_key",
						hostPort:    unk.HostPort,
						fingerprint: unk.Fingerprint,
						key:         unk.Key,
					}
					resp["errCode"] = "unknown_host_key"
					resp["hostPort"] = unk.HostPort
					resp["fingerprint"] = unk.Fingerprint
				}

				var mismatch sshclient.ErrHostKeyMismatch
				if errors.As(info.Err, &mismatch) {
					w.pendingTrust[req.HostID] = pendingKey{
						kind:        "host_key_mismatch",
						hostPort:    mismatch.HostPort,
						fingerprint: mismatch.Fingerprint,
						key:         mismatch.Key,
					}
					resp["errCode"] = "host_key_mismatch"
					resp["hostPort"] = mismatch.HostPort
					resp["fingerprint"] = mismatch.Fingerprint
				}

				if info.LastErr == "password_required" {
					resp["errCode"] = "password_required"
				}
				if errors.Is(info.Err, sshclient.ErrPassphraseRequired) {
					resp["errCode"] = "passphrase_required"
				}
			}

			return ok(resp)

		case "disconnect":
			if req.HostID == 0 {
				return fail("bad_request", nil)
			}
			if err := w.mgr.DisconnectTab(req.HostID, req.TabID); err != nil {
				return fail("disconnect_failed", rpcResp{"detail": err.Error()})
			}
			w.sftp.Disconnect(req.HostID)
			w.attachedMu.Lock()
			delete(w.attached, attachKey{hostID: req.HostID, tabID: req.TabID})
			w.attachedMu.Unlock()
			return ok(nil)

		case "telecom_pick":
			// Bind handlers are invoked on the UI thread in this build; dispatching
			// back to the UI thread and waiting would deadlock.
			return ok(rpcResp{"path": w.pickTelecomExecutablePath()})

		case "about":
			return ok(rpcResp{"text": "pTerminal – SSH Terminal Manager"})

		case "teams_presence":
			if w.p2p == nil {
				return ok(rpcResp{"peers": []p2p.PeerInfo{}, "user": w.mgr.Config().User})
			}
			presence := w.p2p.Presence()
			return ok(rpcResp{"peers": presence.Peers, "user": presence.User})

		case "team_repo_paths":
			cfg := w.mgr.Config()
			p, err := config.ConfigPath()
			if err != nil {
				return fail("config_load_failed", nil)
			}
			base := filepath.Dir(p)
			paths := map[string]string{}
			for _, t := range cfg.Teams {
				if t.ID == "" {
					continue
				}
				paths[t.ID] = teamrepo.TeamDir(base, t.ID)
			}
			return ok(rpcResp{"paths": paths})

		case "update_status":
			if req.Force {
				w.triggerUpdateCheck(true)
			}
			return ok(rpcResp{"update": w.updatePayload()})

		case "update_check":
			w.triggerUpdateCheck(true)
			return ok(rpcResp{"update": w.updatePayload()})

		case "update_install":
			go w.runUpdateInstall()
			return ok(rpcResp{"update": w.updatePayload()})

		default:
			return fail("unknown_rpc", nil)
		}
	})

	html, _ := w.buildInlinedHTML()
	w.wv.SetHtml(html)

	w.startIOLoops()
	w.startPTYFlushLoop()
	if softwareRenderEnabled() {
		w.wv.Dispatch(func() {
			w.wv.Eval("window.__notifySoftwareRender && window.__notifySoftwareRender();")
		})
	}
	w.startTray()
	w.triggerUpdateCheck(false)
	return w, nil
}

func (w *Window) ApplyConfig(cfg model.AppConfig) {
	w.mgr.SetConfig(cfg)
	w.sftp.SetConfig(cfg)
	if w.p2p != nil {
		w.p2p.SetConfig(cfg)
	}

	sw := softwareRenderEnabled()
	b, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	notifyJS := ""
	if sw {
		notifyJS = "window.__notifySoftwareRender && window.__notifySoftwareRender();"
	}
	applyJS := fmt.Sprintf(`(function(){var cfg=%s;var tries=0;var apply=function(){if(window.__applyConfig){window.__applyConfig(cfg);%s}else if(tries<200){tries++;setTimeout(apply,50);}};apply();})();`, string(b), notifyJS)
	w.wv.Dispatch(func() {
		w.wv.Eval(applyJS)
	})
}

func (w *Window) startIOLoops() {
	ctx, cancel := context.WithCancel(context.Background())
	w.ioCancel = cancel

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-w.inputCh:
				if !ok {
					return
				}
				_ = w.mgr.WriteTab(msg.hostID, msg.tabID, msg.dataB64)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-w.resizeCh:
				if !ok {
					return
				}
				_ = w.mgr.ResizeTab(msg.hostID, msg.tabID, msg.cols, msg.rows)
			}
		}
	}()
}

func (w *Window) attachOutput(hostID, tabID int, sess terminal.Session) {
	for chunk := range sess.Output() {
		w.mgr.BufferOutputTab(hostID, tabID, chunk)
		if int(w.activeHostID.Load()) == hostID && int(w.activeTabID.Load()) == tabID {
			w.kickFlush()
		}
	}
}

func (w *Window) ensureAttached(hostID, tabID int, sess terminal.Session) {
	w.attachedMu.Lock()
	defer w.attachedMu.Unlock()

	k := attachKey{hostID: hostID, tabID: tabID}
	if w.attached[k] == sess {
		return
	}
	w.attached[k] = sess
	go w.attachOutput(hostID, tabID, sess)
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
				tabID := int(w.activeTabID.Load())
				if hostID != 0 && tabID != 0 {
					_ = w.flushHost(hostID, tabID)
				}
			}
		}
	}()
}

func (w *Window) startTray() {
	trayInit(w)
	installWindowCloseHandler(w.wv)
	w.showFromTray()
}

func (w *Window) triggerUpdateCheck(force bool) {
	go w.runUpdateCheck(force)
}

func (w *Window) runUpdateCheck(force bool) {
	w.updateMu.Lock()
	if w.update.Checking && !force {
		w.updateMu.Unlock()
		return
	}
	w.update.Checking = true
	w.update.Error = ""
	w.updateMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rel, err := update.Latest(ctx)
	w.updateMu.Lock()
	defer w.updateMu.Unlock()
	w.update.Checking = false
	if err != nil {
		w.update.Error = err.Error()
		return
	}

	w.update.LatestTag = rel.Tag
	w.update.ReleaseURL = rel.HTMLURL
	w.update.ReleaseNotes = rel.Body

	if asset, ok := selectUpdateAsset(runtime.GOOS, rel.Assets); ok {
		w.update.AssetName = asset.Name
		w.update.AssetURL = asset.URL
	} else {
		w.update.AssetName = ""
		w.update.AssetURL = ""
	}

	w.update.HasUpdate = rel.Tag != "" && isNewVersion(rel.Tag, buildinfo.Version) && !rel.Draft && !rel.Prerelease
	if !w.update.HasUpdate {
		w.update.AssetName = ""
		w.update.AssetURL = ""
	}

	w.update.LastCheck = time.Now()
}

func (w *Window) runUpdateInstall() {
	w.updateMu.Lock()
	if w.update.Installing || w.update.AssetURL == "" {
		w.update.Error = "no update asset available"
		w.update.Installing = false
		w.updateMu.Unlock()
		return
	}
	w.update.Installing = true
	w.update.Error = ""
	assetURL := w.update.AssetURL
	w.updateMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	path, err := downloadAsset(ctx, assetURL)
	if err != nil {
		w.setUpdateError(err.Error())
		return
	}
	defer os.Remove(path)

	binaryPath, err := extractPortableBinary(path)
	if err != nil {
		w.setUpdateError(err.Error())
		return
	}
	defer os.Remove(binaryPath)

	dest := w.exePath
	if dest == "" {
		dest = filepath.Join(".", "bin", "pterminal")
	}

	if err := installBinary(binaryPath, dest); err != nil {
		w.setUpdateError(err.Error())
		return
	}

	w.setUpdateInstalled()
}

func (w *Window) setUpdateError(msg string) {
	w.updateMu.Lock()
	w.update.Error = msg
	w.update.Installing = false
	w.updateMu.Unlock()
	w.notifyUpdateError("Update failed: " + msg)
}

func (w *Window) setUpdateInstalled() {
	w.updateMu.Lock()
	w.update.Installing = false
	w.update.HasUpdate = false
	w.update.InstalledTag = w.update.LatestTag
	w.updateMu.Unlock()
	message := "Update installed (" + w.update.LatestTag + "). Restarting the app will run the new version."
	w.notifyUpdateSuccess(message)
}

func downloadAsset(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected download status: %s", resp.Status)
	}

	tmp, err := os.CreateTemp("", "pterminal-update-*"+filepath.Ext(url))
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return "", err
	}

	return tmp.Name(), nil
}

func extractPortableBinary(archivePath string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	tmpDir, err := os.MkdirTemp("", "pterminal-update-*")
	if err != nil {
		return "", err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "pterminal" {
			continue
		}
		dest := filepath.Join(tmpDir, "pterminal")
		out, err := os.Create(dest)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
		if err := os.Chmod(dest, 0o755); err != nil {
			return "", err
		}
		return dest, nil
	}

	return "", errors.New("pterminal binary not found in archive")
}

func installBinary(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := copyFile(src, dest); err != nil {
		return err
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func (w *Window) notifyUpdateSuccess(msg string) {
	w.runJSNotify("notifySuccess", msg)
}

func (w *Window) notifyUpdateError(msg string) {
	w.runJSNotify("notifyError", msg)
}

func (w *Window) runJSNotify(fn, msg string) {
	if w.wv == nil {
		return
	}
	quoted := strconv.Quote(msg)
	w.wv.Dispatch(func() {
		w.wv.Eval(fmt.Sprintf("%s(%s);", fn, quoted))
	})
}

func (w *Window) hideFromTray() {
	if w == nil {
		return
	}
	hideGtkWindow(w.wv)
	w.windowHidden.Store(true)
}

func (w *Window) showFromTray() {
	if w == nil {
		return
	}
	showGtkWindow(w.wv)
	w.windowHidden.Store(false)
	w.kickFlush()
}

func (w *Window) closeFromTray() {
	if !confirmCloseDialog() {
		return
	}
	w.Close()
}
func (w *Window) flushHost(hostID, tabID int) bool {
	const maxBytesPerEval = 96 * 1024
	const maxBytesPerCycle = 4 * maxBytesPerEval

	chunks, more := w.mgr.DrainBufferedUpToTab(hostID, tabID, maxBytesPerCycle)
	if len(chunks) == 0 {
		return false
	}

	buf := make([]byte, 0, 32*1024)
	b64s := make([]string, 0, len(chunks)+1)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		b64s = append(b64s, base64.StdEncoding.EncodeToString(buf))
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
				b64s = append(b64s, base64.StdEncoding.EncodeToString(c[start:end]))
			}
			continue
		}

		if len(buf)+len(c) > maxBytesPerEval {
			flush()
		}
		buf = append(buf, c...)
	}
	flush()

	w.pushPTYB64Batch(hostID, tabID, b64s)

	if more {
		w.kickFlush()
	}
	return true
}

func (w *Window) pushPTYB64Batch(hostID, tabID int, chunks []string) {
	if len(chunks) == 0 {
		return
	}

	hid := strconv.Itoa(hostID)
	tid := strconv.Itoa(tabID)
	var sb strings.Builder
	sb.WriteString("(function(){")
	for _, b64 := range chunks {
		if b64 == "" {
			continue
		}
		sb.WriteString("window.dispatchPTY(")
		sb.WriteString(hid)
		sb.WriteByte(',')
		sb.WriteString(tid)
		sb.WriteByte(',')
		sb.WriteString(strconv.Quote(b64))
		sb.WriteString(");")
	}
	sb.WriteString("})()")

	js := sb.String()
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
	s = strings.ReplaceAll(s, `<script src="app.js"></script>`, "<script>"+string(appJS)+"</script>")
	return s, nil
}

func (w *Window) Run() { w.wv.Run() }
func (w *Window) Close() {
	if w.flushCancel != nil {
		w.flushCancel()
	}
	if w.ioCancel != nil {
		w.ioCancel()
	}
	if w.inputCh != nil {
		close(w.inputCh)
	}
	if w.resizeCh != nil {
		close(w.resizeCh)
	}
	if w.mgr != nil {
		w.mgr.DisconnectAll()
	}
	if w.sftp != nil {
		w.sftp.DisconnectAll()
	}
	trayCleanup()
	w.wv.Destroy()
}

func b64dec(b string) (string, error) {
	d, err := base64.StdEncoding.DecodeString(b)
	return string(d), err
}
