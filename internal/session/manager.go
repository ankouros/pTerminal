package session

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ankouros/pterminal/internal/cmdclient"
	"github.com/ankouros/pterminal/internal/model"
	"github.com/ankouros/pterminal/internal/sshclient"
	"github.com/ankouros/pterminal/internal/terminal"
)

var truncatedOutputMsg = []byte("\r\n\x1b[33m[output truncated]\x1b[0m\r\n")

type sessionKey struct {
	hostID int
	tabID  int
}

func makeSessionKey(hostID, tabID int) sessionKey {
	if tabID <= 0 {
		tabID = 1
	}
	return sessionKey{hostID: hostID, tabID: tabID}
}

type SessionState int

const (
	StateConnected SessionState = iota
	StateDisconnected
	StateReconnecting
)

type SessionInfo struct {
	State    SessionState `json:"-"`
	Attempts int          `json:"-"`
	LastErr  string       `json:"-"`
	Err      error        `json:"-"`
}

type ManagedSession struct {
	Key  sessionKey
	Host model.Host
	Sess terminal.Session

	State         SessionState
	Err           error
	Attempts      int
	AutoReconnect bool

	cols int
	rows int

	connectSeq    int64
	connectCancel context.CancelFunc

	mu sync.Mutex
}

type Manager struct {
	mu sync.Mutex

	cfg model.AppConfig

	sessions map[sessionKey]*ManagedSession
	buffers  map[sessionKey][][]byte
	bufBytes map[sessionKey]int
	bufDrop  map[sessionKey]bool

	passwordProvider func(hostID int) (string, error)
}

func NewManager(cfg model.AppConfig) *Manager {
	return &Manager{
		cfg:      cfg,
		sessions: make(map[sessionKey]*ManagedSession),
		buffers:  make(map[sessionKey][][]byte),
		bufBytes: make(map[sessionKey]int),
		bufDrop:  make(map[sessionKey]bool),
	}
}

func (m *Manager) SetConfig(cfg model.AppConfig) {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
}

func (m *Manager) Config() model.AppConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func (m *Manager) Ensure(
	ctx context.Context,
	hostID int,
	tabID int,
	cols, rows int,
	pw func(hostID int) (string, error),
) (terminal.Session, error) {
	k := makeSessionKey(hostID, tabID)

	m.mu.Lock()
	m.passwordProvider = pw

	if ms := m.sessions[k]; ms != nil {
		ms.mu.Lock()
		ms.cols, ms.rows = cols, rows
		sess := ms.Sess
		state := ms.State
		auto := ms.AutoReconnect
		ms.mu.Unlock()

		// If a session exists and is live, reuse it.
		if sess != nil {
			m.mu.Unlock()
			_ = sess.Resize(cols, rows)
			return sess, nil
		}

		// If the session is disconnected and auto-reconnect is disabled (e.g. telecom),
		// allow a fresh dial instead of returning "reconnecting".
		if state == StateDisconnected && !auto {
			delete(m.sessions, k)
			m.mu.Unlock()
		} else {
			m.mu.Unlock()
			return nil, errors.New("session reconnecting")
		}
	} else {
		m.mu.Unlock()
	}

	host, ok := m.findHost(hostID)
	if !ok {
		return nil, fmt.Errorf("host %d not found", hostID)
	}

	sess, err := m.dial(ctx, host, cols, rows, pw)
	if err != nil {
		return nil, err
	}

	ms := &ManagedSession{
		Key:           k,
		Host:          host,
		Sess:          sess,
		State:         StateConnected,
		AutoReconnect: host.Driver == "" || host.Driver == model.DriverSSH,
		cols:          cols,
		rows:          rows,
	}

	m.mu.Lock()
	m.sessions[k] = ms
	m.mu.Unlock()

	go m.monitor(ms)
	return sess, nil
}

// StartConnectAsync starts (or reuses) a connection attempt without blocking the caller.
// It updates per-host state so callers can poll SessionInfo for progress/errors.
// onResult is invoked from a background goroutine once the attempt succeeds or fails.
func (m *Manager) StartConnectAsync(
	hostID int,
	tabID int,
	cols, rows int,
	pw func(hostID int) (string, error),
	onResult func(sess terminal.Session, err error),
) (terminal.Session, bool, error) {
	if hostID == 0 {
		return nil, false, errors.New("host id is required")
	}
	if pw == nil {
		return nil, false, errors.New("password provider not set")
	}

	k := makeSessionKey(hostID, tabID)

	m.mu.Lock()
	m.passwordProvider = pw

	if ms := m.sessions[k]; ms != nil {
		ms.mu.Lock()
		ms.cols, ms.rows = cols, rows
		sess := ms.Sess
		state := ms.State
		ms.mu.Unlock()

		if sess != nil {
			m.mu.Unlock()
			_ = sess.Resize(cols, rows)
			return sess, true, nil
		}
		if state == StateReconnecting {
			m.mu.Unlock()
			return nil, false, nil
		}
	}

	cfg := m.cfg
	var (
		host model.Host
		ok   bool
	)
	for _, netw := range cfg.Networks {
		for _, h := range netw.Hosts {
			if h.ID == hostID {
				host = h
				ok = true
				break
			}
		}
		if ok {
			break
		}
	}
	if !ok {
		m.mu.Unlock()
		return nil, false, fmt.Errorf("host %d not found", hostID)
	}

	ms := m.sessions[k]
	if ms == nil {
		ms = &ManagedSession{Key: k, Host: host}
		m.sessions[k] = ms
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

	ms.mu.Lock()
	ms.Host = host
	ms.cols, ms.rows = cols, rows
	ms.State = StateReconnecting
	ms.Attempts = 0
	ms.Err = nil
	ms.Sess = nil
	ms.AutoReconnect = false
	ms.connectSeq++
	seq := ms.connectSeq
	ms.connectCancel = cancel
	ms.mu.Unlock()

	m.mu.Unlock()

	go func() {
		defer cancel()

		sess, err := m.dial(ctx, host, cols, rows, pw)

		// If disconnected or a newer connect attempt started, discard the result.
		m.mu.Lock()
		current := m.sessions[k] == ms
		m.mu.Unlock()
		ms.mu.Lock()
		stillCurrent := current && ms.connectSeq == seq
		if !stillCurrent {
			ms.mu.Unlock()
			if sess != nil {
				_ = sess.Close()
			}
			return
		}

		ms.connectCancel = nil

		if err != nil {
			ms.State = StateDisconnected
			ms.Sess = nil
			ms.Err = err
			ms.Attempts = 0
			ms.AutoReconnect = false
			ms.mu.Unlock()
			if onResult != nil {
				onResult(nil, err)
			}
			return
		}

		ms.Sess = sess
		ms.State = StateConnected
		ms.Err = nil
		ms.Attempts = 0
		ms.AutoReconnect = host.Driver == "" || host.Driver == model.DriverSSH
		ms.mu.Unlock()

		go m.monitor(ms)
		if onResult != nil {
			onResult(sess, nil)
		}
	}()

	return nil, false, nil
}

func (m *Manager) monitor(ms *ManagedSession) {
	ms.mu.Lock()
	sess := ms.Sess
	ms.mu.Unlock()
	if sess == nil {
		return
	}
	<-sess.Done()

	ms.mu.Lock()
	ms.State = StateDisconnected
	ms.Sess = nil
	if ms.Err == nil {
		ms.Err = errors.New("connection lost")
	}
	auto := ms.AutoReconnect
	ms.mu.Unlock()

	if auto {
		go m.reconnect(ms)
	}
}

func backoff(attempt int) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<attempt) * time.Second
}

func (m *Manager) reconnect(ms *ManagedSession) {
	// Only SSH sessions are auto-reconnected. Telecom runs an interactive local process.
	if ms.Host.Driver != "" && ms.Host.Driver != model.DriverSSH {
		return
	}

	for attempt := 1; ; attempt++ {
		ms.mu.Lock()
		auto := ms.AutoReconnect
		ms.mu.Unlock()
		if !auto {
			return
		}

		ms.mu.Lock()
		ms.State = StateReconnecting
		ms.Attempts = attempt
		ms.mu.Unlock()

		time.Sleep(backoff(attempt))

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)

		sess, err := m.dial(ctx, ms.Host, ms.cols, ms.rows, func(hostID int) (string, error) {
			m.mu.Lock()
			pw := m.passwordProvider
			m.mu.Unlock()
			if pw == nil {
				return "", errors.New("password provider not set")
			}
			return pw(hostID)
		})
		cancel()

		if err != nil {
			// 🚨 CRITICAL FIX:
			// Host key problems MUST NOT auto-reconnect.
			var unk sshclient.ErrUnknownHostKey
			var mismatch sshclient.ErrHostKeyMismatch

			if errors.As(err, &unk) || errors.As(err, &mismatch) {
				ms.mu.Lock()
				ms.State = StateDisconnected
				ms.Err = err
				ms.Attempts = 0
				ms.mu.Unlock()
				return
			}

			ms.mu.Lock()
			ms.Err = err
			ms.mu.Unlock()
			continue
		}

		ms.mu.Lock()
		ms.Sess = sess
		ms.State = StateConnected
		ms.Err = nil
		ms.Attempts = 0
		ms.AutoReconnect = true
		ms.mu.Unlock()

		go m.monitor(ms)
		return
	}
}

func (m *Manager) Resize(hostID, cols, rows int) error { return m.ResizeTab(hostID, 1, cols, rows) }

func (m *Manager) ResizeTab(hostID, tabID, cols, rows int) error {
	k := makeSessionKey(hostID, tabID)
	m.mu.Lock()
	ms := m.sessions[k]
	m.mu.Unlock()
	if ms == nil {
		return nil
	}

	ms.mu.Lock()
	ms.cols, ms.rows = cols, rows
	sess := ms.Sess
	ms.mu.Unlock()

	if sess != nil {
		return sess.Resize(cols, rows)
	}
	return nil
}

func (m *Manager) SessionInfo(hostID int) SessionInfo { return m.SessionInfoTab(hostID, 1) }

func (m *Manager) SessionInfoTab(hostID, tabID int) SessionInfo {
	k := makeSessionKey(hostID, tabID)
	m.mu.Lock()
	ms := m.sessions[k]
	m.mu.Unlock()
	if ms == nil {
		return SessionInfo{State: StateDisconnected}
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	info := SessionInfo{
		State:    ms.State,
		Attempts: ms.Attempts,
		Err:      ms.Err,
	}
	if ms.Err != nil {
		info.LastErr = ms.Err.Error()
	}
	return info
}

func (m *Manager) BufferOutput(hostID int, data []byte) { m.BufferOutputTab(hostID, 1, data) }

func (m *Manager) BufferOutputTab(hostID, tabID int, data []byte) {
	k := makeSessionKey(hostID, tabID)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Coalesce small chunks to reduce allocations and bridge overhead during bursts.
	const coalesceIfLastBelow = 8 * 1024
	const coalesceMaxLastSize = 64 * 1024
	const maxChunks = 2000
	const maxBufferedBytes = 8 * 1024 * 1024

	b := m.buffers[k]
	if n := len(b); n > 0 && len(b[n-1]) < coalesceIfLastBelow && len(b[n-1])+len(data) <= coalesceMaxLastSize {
		b[n-1] = append(b[n-1], data...)
	} else {
		b = append(b, data)
	}

	m.bufBytes[k] += len(data)

	if len(b) > maxChunks {
		drop := len(b) - maxChunks
		for i := 0; i < drop; i++ {
			m.bufBytes[k] -= len(b[i])
			b[i] = nil
		}
		b = b[drop:]
		m.bufDrop[k] = true
	}

	for m.bufBytes[k] > maxBufferedBytes && len(b) > 1 {
		m.bufBytes[k] -= len(b[0])
		b[0] = nil
		b = b[1:]
		m.bufDrop[k] = true
	}

	m.buffers[k] = b
}

func (m *Manager) DrainBuffered(hostID int) [][]byte { return m.DrainBufferedTab(hostID, 1) }

func (m *Manager) DrainBufferedTab(hostID, tabID int) [][]byte {
	k := makeSessionKey(hostID, tabID)
	m.mu.Lock()
	defer m.mu.Unlock()

	b := m.buffers[k]
	m.buffers[k] = nil
	m.bufBytes[k] = 0

	if m.bufDrop[k] {
		m.bufDrop[k] = false
		return append([][]byte{truncatedOutputMsg}, b...)
	}
	return b
}

func (m *Manager) DrainBufferedUpTo(hostID int, maxBytes int) ([][]byte, bool) {
	return m.DrainBufferedUpToTab(hostID, 1, maxBytes)
}

func (m *Manager) DrainBufferedUpToTab(hostID, tabID, maxBytes int) ([][]byte, bool) {
	k := makeSessionKey(hostID, tabID)
	m.mu.Lock()
	defer m.mu.Unlock()

	b := m.buffers[k]
	if len(b) == 0 {
		return nil, false
	}

	total := 0
	n := 0
	for n < len(b) {
		// Always take at least one chunk.
		if maxBytes > 0 && n > 0 && total+len(b[n]) > maxBytes {
			break
		}
		total += len(b[n])
		n++
		if maxBytes > 0 && total >= maxBytes {
			break
		}
	}

	out := b[:n]
	for i := 0; i < n; i++ {
		m.bufBytes[k] -= len(out[i])
	}
	if m.bufBytes[k] < 0 {
		m.bufBytes[k] = 0
	}

	if n >= len(b) {
		m.buffers[k] = nil
		m.bufBytes[k] = 0

		if m.bufDrop[k] {
			m.bufDrop[k] = false
			return append([][]byte{truncatedOutputMsg}, out...), false
		}
		return out, false
	}
	m.buffers[k] = b[n:]

	more := true
	if m.bufDrop[k] {
		m.bufDrop[k] = false
		return append([][]byte{truncatedOutputMsg}, out...), more
	}
	return out, more
}

func (m *Manager) Write(hostID int, b64 string) error { return m.WriteTab(hostID, 1, b64) }

func (m *Manager) WriteTab(hostID, tabID int, b64 string) error {
	k := makeSessionKey(hostID, tabID)
	m.mu.Lock()
	ms := m.sessions[k]
	m.mu.Unlock()
	if ms == nil {
		return errors.New("session not connected")
	}

	ms.mu.Lock()
	sess := ms.Sess
	ms.mu.Unlock()
	if sess == nil {
		return errors.New("session not connected")
	}

	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return sess.Write(data)
}

func (m *Manager) dial(
	ctx context.Context,
	host model.Host,
	cols, rows int,
	pw func(hostID int) (string, error),
) (terminal.Session, error) {
	driver := host.Driver
	if driver == "" {
		driver = model.DriverSSH
	}

	switch driver {
	case model.DriverSSH:
		return sshclient.DialAndStart(ctx, host, cols, rows, func() (string, error) {
			if pw == nil {
				return "", errors.New("password provider not set")
			}
			return pw(host.ID)
		})

	case model.DriverTelecom, model.DriverIOShell:
		return cmdclient.StartTelecom(ctx, host, cols, rows)

	default:
		return nil, fmt.Errorf("unknown connection driver: %s", driver)
	}
}

func (m *Manager) Disconnect(hostID int) error { return m.DisconnectTab(hostID, 1) }

func (m *Manager) DisconnectTab(hostID, tabID int) error {
	k := makeSessionKey(hostID, tabID)
	m.mu.Lock()
	ms := m.sessions[k]
	if ms == nil {
		m.mu.Unlock()
		return nil
	}
	delete(m.sessions, k)
	delete(m.buffers, k)
	delete(m.bufBytes, k)
	delete(m.bufDrop, k)
	m.mu.Unlock()

	ms.mu.Lock()
	if ms.connectCancel != nil {
		ms.connectCancel()
		ms.connectCancel = nil
	}
	ms.connectSeq++
	ms.AutoReconnect = false
	sess := ms.Sess
	ms.Sess = nil
	ms.State = StateDisconnected
	ms.Attempts = 0
	ms.Err = nil
	ms.mu.Unlock()

	if sess != nil {
		return sess.Close()
	}
	return nil
}

func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	sessions := make([]*ManagedSession, 0, len(m.sessions))
	for _, ms := range m.sessions {
		if ms != nil {
			sessions = append(sessions, ms)
		}
	}
	m.sessions = make(map[sessionKey]*ManagedSession)
	m.buffers = make(map[sessionKey][][]byte)
	m.bufBytes = make(map[sessionKey]int)
	m.bufDrop = make(map[sessionKey]bool)
	m.mu.Unlock()

	for _, ms := range sessions {
		ms.mu.Lock()
		if ms.connectCancel != nil {
			ms.connectCancel()
			ms.connectCancel = nil
		}
		ms.connectSeq++
		ms.AutoReconnect = false
		sess := ms.Sess
		ms.Sess = nil
		ms.State = StateDisconnected
		ms.Attempts = 0
		ms.Err = nil
		ms.mu.Unlock()

		if sess != nil {
			_ = sess.Close()
		}
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
