package sftpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ankouros/pterminal/internal/model"
	"github.com/ankouros/pterminal/internal/sshclient"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
	ModUnix int64  `json:"modUnix"`
}

type session struct {
	host model.Host

	ssh     *ssh.Client
	cleanup func()
	sftp    *sftp.Client
}

type upload struct {
	hostID int
	f      *sftp.File
}

type Manager struct {
	mu sync.Mutex

	cfg model.AppConfig

	sessions map[int]*session
	uploads  map[string]*upload
}

func NewManager(cfg model.AppConfig) *Manager {
	return &Manager{
		cfg:      cfg,
		sessions: make(map[int]*session),
		uploads:  make(map[string]*upload),
	}
}

func (m *Manager) SetConfig(cfg model.AppConfig) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
}

func (m *Manager) Disconnect(hostID int) {
	m.mu.Lock()
	s := m.sessions[hostID]
	delete(m.sessions, hostID)
	m.mu.Unlock()
	if s == nil {
		return
	}
	if s.sftp != nil {
		_ = s.sftp.Close()
	}
	if s.ssh != nil {
		_ = s.ssh.Close()
	}
	if s.cleanup != nil {
		s.cleanup()
	}
}

func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	ids := make([]int, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Disconnect(id)
	}
}

func (m *Manager) ensure(ctx context.Context, hostID int, passwordProvider func(hostID int) (string, error)) (*sftp.Client, error) {
	m.mu.Lock()
	if s := m.sessions[hostID]; s != nil && s.sftp != nil {
		c := s.sftp
		m.mu.Unlock()
		return c, nil
	}
	m.mu.Unlock()

	host, ok := m.findHost(hostID)
	if !ok {
		return nil, fmt.Errorf("host %d not found", hostID)
	}
	if !isSFTPEnabled(host) {
		return nil, errors.New("sftp is not enabled for this host")
	}

	sftpUser, pwFn, err := m.sftpAuth(host, passwordProvider)
	if err != nil {
		return nil, err
	}
	host.User = sftpUser
	host.Driver = model.DriverSSH
	if host.SFTP != nil && host.SFTP.Credentials == model.SFTPCredsCustom {
		host.Auth.Method = model.AuthPassword
	}

	client, cleanup, err := sshclient.DialClient(ctx, host, func() (string, error) {
		return pwFn()
	})
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}

	sf, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}

	m.mu.Lock()
	m.sessions[hostID] = &session{host: host, ssh: client, cleanup: cleanup, sftp: sf}
	m.mu.Unlock()

	return sf, nil
}

func (m *Manager) List(ctx context.Context, hostID int, dir string, passwordProvider func(hostID int) (string, error)) ([]Entry, string, error) {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return nil, "", err
	}

	if dir == "" {
		dir = "."
	}

	// If a file is passed, list its parent and select file on client side.
	p := cleanRemotePath(dir)
	fi, statErr := c.Stat(p)
	if statErr == nil && !fi.IsDir() {
		p = path.Dir(p)
	}

	entries, err := c.ReadDir(p)
	if err != nil {
		return nil, p, err
	}

	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		out = append(out, Entry{
			Name:    name,
			Path:    path.Join(p, name),
			IsDir:   e.IsDir(),
			Size:    e.Size(),
			Mode:    uint32(e.Mode()),
			ModUnix: e.ModTime().Unix(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	return out, p, nil
}

func (m *Manager) MkdirAll(ctx context.Context, hostID int, remotePath string, passwordProvider func(hostID int) (string, error)) error {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return err
	}
	return c.MkdirAll(cleanRemotePath(remotePath))
}

func (m *Manager) Remove(ctx context.Context, hostID int, remotePath string, passwordProvider func(hostID int) (string, error)) error {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return err
	}
	p := cleanRemotePath(remotePath)
	fi, err := c.Stat(p)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return c.RemoveDirectory(p)
	}
	return c.Remove(p)
}

func (m *Manager) Rename(ctx context.Context, hostID int, fromPath, toPath string, passwordProvider func(hostID int) (string, error)) error {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return err
	}
	return c.Rename(cleanRemotePath(fromPath), cleanRemotePath(toPath))
}

func (m *Manager) ReadFile(ctx context.Context, hostID int, remotePath string, maxBytes int64, passwordProvider func(hostID int) (string, error)) ([]byte, error) {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return nil, err
	}
	p := cleanRemotePath(remotePath)

	fi, err := c.Stat(p)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, errors.New("path is a directory")
	}
	if maxBytes > 0 && fi.Size() > maxBytes {
		return nil, fmt.Errorf("file too large (%d bytes, limit %d)", fi.Size(), maxBytes)
	}

	f, err := c.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if maxBytes <= 0 {
		maxBytes = fi.Size()
	}
	if maxBytes < 0 {
		maxBytes = 0
	}

	buf := make([]byte, 0, minInt64(256*1024, maxBytes))
	tmp := make([]byte, 32*1024)
	var total int64
	for {
		n, rerr := f.Read(tmp)
		if n > 0 {
			total += int64(n)
			if maxBytes > 0 && total > maxBytes {
				return nil, fmt.Errorf("file exceeded limit %d bytes", maxBytes)
			}
			buf = append(buf, tmp[:n]...)
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return nil, rerr
		}
	}

	return buf, nil
}

func (m *Manager) WriteFile(ctx context.Context, hostID int, remotePath string, data []byte, passwordProvider func(hostID int) (string, error)) error {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return err
	}
	p := cleanRemotePath(remotePath)

	fi, err := c.Stat(p)
	if err == nil && fi.IsDir() {
		return errors.New("path is a directory")
	}

	f, err := c.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func (m *Manager) DownloadToDownloads(ctx context.Context, hostID int, remotePath string, passwordProvider func(hostID int) (string, error)) (string, error) {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return "", err
	}

	rp := cleanRemotePath(remotePath)
	rf, err := c.Open(rp)
	if err != nil {
		return "", err
	}
	defer rf.Close()

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	downloads := filepath.Join(home, "Downloads")
	_ = os.MkdirAll(downloads, 0o755)

	base := path.Base(rp)
	base = strings.ReplaceAll(base, "/", "_")
	base = strings.ReplaceAll(base, string(filepath.Separator), "_")
	if base == "" || base == "." {
		base = "download"
	}

	out := filepath.Join(downloads, fmt.Sprintf("pterminal-%d-%s-%s", hostID, time.Now().Format("20060102-150405"), base))
	lf, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer lf.Close()

	if _, err := io.Copy(lf, rf); err != nil {
		return "", err
	}
	return out, nil
}

func (m *Manager) BeginUpload(ctx context.Context, hostID int, remoteDir, filename string, passwordProvider func(hostID int) (string, error)) (string, error) {
	c, err := m.ensure(ctx, hostID, passwordProvider)
	if err != nil {
		return "", err
	}
	if filename == "" {
		return "", errors.New("filename is empty")
	}
	dir := cleanRemotePath(remoteDir)
	p := path.Join(dir, path.Base(filename))

	f, err := c.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return "", err
	}

	id := fmt.Sprintf("%d-%d", hostID, time.Now().UnixNano())
	m.mu.Lock()
	m.uploads[id] = &upload{hostID: hostID, f: f}
	m.mu.Unlock()
	return id, nil
}

func (m *Manager) UploadChunk(uploadID string, data []byte) error {
	m.mu.Lock()
	u := m.uploads[uploadID]
	m.mu.Unlock()
	if u == nil || u.f == nil {
		return errors.New("upload not found")
	}
	_, err := u.f.Write(data)
	return err
}

func (m *Manager) EndUpload(uploadID string) error {
	m.mu.Lock()
	u := m.uploads[uploadID]
	delete(m.uploads, uploadID)
	m.mu.Unlock()
	if u == nil {
		return nil
	}
	if u.f != nil {
		return u.f.Close()
	}
	return nil
}

func isSFTPEnabled(h model.Host) bool {
	if h.SFTP != nil {
		return h.SFTP.Enabled
	}
	return h.SFTPEnabled
}

func (m *Manager) sftpAuth(host model.Host, passwordProvider func(hostID int) (string, error)) (user string, pw func() (string, error), err error) {
	// Default to reusing connection credentials.
	mode := model.SFTPCredsConnection
	if host.SFTP != nil && host.SFTP.Credentials != "" {
		mode = host.SFTP.Credentials
	}

	switch mode {
	case model.SFTPCredsCustom:
		if host.SFTP == nil {
			return "", nil, errors.New("sftp custom credentials missing")
		}
		u := strings.TrimSpace(host.SFTP.User)
		p := host.SFTP.Password
		if u == "" || p == "" {
			return "", nil, errors.New("sftp custom credentials are incomplete")
		}
		return u, func() (string, error) { return p, nil }, nil

	case model.SFTPCredsConnection, "":
		return host.User, func() (string, error) {
			if host.Auth.Method != model.AuthPassword {
				// Password provider isn't required for key/agent auth, but is required by sshclient
				// only when method=password.
				return "", nil
			}
			if passwordProvider == nil {
				return "", errors.New("password provider not set")
			}
			return passwordProvider(host.ID)
		}, nil

	default:
		return "", nil, fmt.Errorf("unknown sftp credential mode: %s", mode)
	}
}

func (m *Manager) findHost(id int) (model.Host, bool) {
	m.mu.Lock()
	cfg := m.cfg
	m.mu.Unlock()

	for _, netw := range cfg.Networks {
		for _, h := range netw.Hosts {
			if h.ID == id {
				return h, true
			}
		}
	}
	return model.Host{}, false
}

func cleanRemotePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "."
	}
	// Force Unix semantics.
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "~") {
		// Let server resolve it best-effort.
		return p
	}
	return path.Clean(p)
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
