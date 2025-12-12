package session

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ankouros/pterminal/internal/model"
	"github.com/ankouros/pterminal/internal/sshclient"
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
	Sess *sshclient.NodeSession

	State    SessionState
	Err      error
	Attempts int

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
) (*sshclient.NodeSession, error) {

	m.mu.Lock()
	m.passwordProvider = pw

	if ms := m.sessions[hostID]; ms != nil {
		ms.mu.Lock()
		ms.cols, ms.rows = cols, rows
		sess := ms.Sess
		ms.mu.Unlock()
		m.mu.Unlock()

		if sess != nil {
			_ = sess.Resize(cols, rows)
			return sess, nil
		}
		return nil, errors.New("session reconnecting")
	}
	m.mu.Unlock()

	host, ok := m.findHost(hostID)
	if !ok {
		return nil, fmt.Errorf("host %d not found", hostID)
	}

	// sshclient now dials directly from model.Host (no FromHost wrapper).
	sess, err := sshclient.DialAndStart(ctx, host, cols, rows, func() (string, error) {
		// only used when host.Auth.Method == password
		return pw(hostID)
	})
	if err != nil {
		return nil, err
	}

	ms := &ManagedSession{
		Host:  host,
		Sess:  sess,
		State: StateConnected,
		cols:  cols,
		rows:  rows,
	}

	m.mu.Lock()
	m.sessions[hostID] = ms
	m.mu.Unlock()

	go m.monitor(ms)
	return sess, nil
}

func (m *Manager) monitor(ms *ManagedSession) {
	// Wait for CLOSE (Output is closed by sshclient when the session ends)
	for range ms.Sess.Output {
	}

	ms.mu.Lock()
	ms.State = StateDisconnected
	if ms.Err == nil {
		ms.Err = errors.New("ssh connection lost")
	}
	ms.mu.Unlock()

	go m.reconnect(ms)
}

func backoff(attempt int) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<attempt) * time.Second
}

func (m *Manager) reconnect(ms *ManagedSession) {
	for attempt := 1; ; attempt++ {
		ms.mu.Lock()
		ms.State = StateReconnecting
		ms.Attempts = attempt
		ms.mu.Unlock()

		time.Sleep(backoff(attempt))

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)

		// Dial using the stored host directly.
		sess, err := sshclient.DialAndStart(ctx, ms.Host, ms.cols, ms.rows, func() (string, error) {
			m.mu.Lock()
			pw := m.passwordProvider
			m.mu.Unlock()
			if pw == nil {
				return "", errors.New("password provider not set")
			}
			return pw(ms.Host.ID)
		})
		cancel()

		if err != nil {
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

	m.buffers[hostID] = append(m.buffers[hostID], data)
	if len(m.buffers[hostID]) > 200 {
		m.buffers[hostID] = m.buffers[hostID][len(m.buffers[hostID])-200:]
	}
}

func (m *Manager) DrainBuffered(hostID int) [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.buffers[hostID]
	m.buffers[hostID] = nil
	return b
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

func WithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
