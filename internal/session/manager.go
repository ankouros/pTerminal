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
}

type ManagedSession struct {
	Host model.Host
	Sess terminal.Session

	State         SessionState
	Err           error
	Attempts      int
	AutoReconnect bool

	cols int
	rows int

	mu sync.Mutex
}

type Manager struct {
	mu sync.Mutex

	cfg model.AppConfig

	sessions map[int]*ManagedSession
	buffers  map[int][][]byte

	passwordProvider func(hostID int) (string, error)
}

func NewManager(cfg model.AppConfig) *Manager {
	return &Manager{
		cfg:      cfg,
		sessions: make(map[int]*ManagedSession),
		buffers:  make(map[int][][]byte),
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
	cols, rows int,
	pw func(hostID int) (string, error),
) (terminal.Session, error) {

	m.mu.Lock()
	m.passwordProvider = pw

	if ms := m.sessions[hostID]; ms != nil {
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

		// If the session is disconnected and auto-reconnect is disabled (e.g. IOshell),
		// allow a fresh dial instead of returning "reconnecting".
		if state == StateDisconnected && !auto {
			delete(m.sessions, hostID)
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
		Host:          host,
		Sess:          sess,
		State:         StateConnected,
		AutoReconnect: host.Driver == "" || host.Driver == model.DriverSSH,
		cols:          cols,
		rows:          rows,
	}

	m.mu.Lock()
	m.sessions[hostID] = ms
	m.mu.Unlock()

	go m.monitor(ms)
	return sess, nil
}

func (m *Manager) monitor(ms *ManagedSession) {
	if ms.Sess == nil {
		return
	}
	<-ms.Sess.Done()

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
	// Only SSH sessions are auto-reconnected. IOshell runs an interactive local process.
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

func (m *Manager) Resize(hostID, cols, rows int) error {
	m.mu.Lock()
	ms := m.sessions[hostID]
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

func (m *Manager) SessionInfo(hostID int) SessionInfo {
	m.mu.Lock()
	ms := m.sessions[hostID]
	m.mu.Unlock()
	if ms == nil {
		return SessionInfo{State: StateDisconnected}
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	info := SessionInfo{
		State:    ms.State,
		Attempts: ms.Attempts,
	}
	if ms.Err != nil {
		info.LastErr = ms.Err.Error()
	}
	return info
}

func (m *Manager) BufferOutput(hostID int, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Coalesce small chunks to reduce allocations and bridge overhead during bursts.
	const coalesceIfLastBelow = 8 * 1024
	const coalesceMaxLastSize = 64 * 1024
	const maxChunks = 2000

	b := m.buffers[hostID]
	if n := len(b); n > 0 && len(b[n-1]) < coalesceIfLastBelow && len(b[n-1])+len(data) <= coalesceMaxLastSize {
		b[n-1] = append(b[n-1], data...)
	} else {
		b = append(b, data)
	}

	if len(b) > maxChunks {
		b = b[len(b)-maxChunks:]
	}
	m.buffers[hostID] = b
}

func (m *Manager) DrainBuffered(hostID int) [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buffers[hostID]
	m.buffers[hostID] = nil
	return b
}

func (m *Manager) DrainBufferedUpTo(hostID int, maxBytes int) ([][]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b := m.buffers[hostID]
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

	out := make([][]byte, n)
	copy(out, b[:n])
	m.buffers[hostID] = b[n:]
	return out, len(m.buffers[hostID]) > 0
}

func (m *Manager) Write(hostID int, b64 string) error {
	m.mu.Lock()
	ms := m.sessions[hostID]
	m.mu.Unlock()

	if ms == nil || ms.Sess == nil {
		return errors.New("session not connected")
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}
	return ms.Sess.Write(data)
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
		return cmdclient.StartIOShell(ctx, host, cols, rows)

	default:
		return nil, fmt.Errorf("unknown connection driver: %s", driver)
	}
}

func (m *Manager) Disconnect(hostID int) error {
	m.mu.Lock()
	ms := m.sessions[hostID]
	if ms == nil {
		m.mu.Unlock()
		return nil
	}
	delete(m.sessions, hostID)
	m.mu.Unlock()

	ms.mu.Lock()
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
	m.sessions = make(map[int]*ManagedSession)
	m.mu.Unlock()

	for _, ms := range sessions {
		ms.mu.Lock()
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
