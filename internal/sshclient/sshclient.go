package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ankouros/pterminal/internal/model"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

/*
Known-hosts UX errors
*/

type ErrUnknownHostKey struct {
	HostPort    string
	Fingerprint string
	Key         ssh.PublicKey
}

func (e ErrUnknownHostKey) Error() string {
	return "unknown host key: " + e.HostPort + " (" + e.Fingerprint + ")"
}

type ErrHostKeyMismatch struct {
	HostPort    string
	Fingerprint string
	Key         ssh.PublicKey
}

func (e ErrHostKeyMismatch) Error() string {
	return "host key mismatch: " + e.HostPort + " (" + e.Fingerprint + ")"
}

/*
NodeSession
*/

type NodeSession struct {
	Node model.Host

	client *ssh.Client
	sess   *ssh.Session

	stdin  io.WriteCloser
	stdout io.Reader
	stderr io.Reader

	Output chan []byte
	Done   chan struct{}

	cancel context.CancelFunc
	once   sync.Once
}

func DialAndStart(
	ctx context.Context,
	host model.Host,
	cols, rows int,
	passwordProvider func() (string, error),
) (*NodeSession, error) {

	cfg, cleanup, err := buildClientConfig(host, passwordProvider)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cleanup != nil {
			cleanup()
		}
	}()

	addr := net.JoinHostPort(host.Host, fmt.Sprint(host.Port))
	fmt.Println("SSH DIAL:", addr, "user:", host.User)

	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		fmt.Println("SSH HANDSHAKE ERROR:", err)
		return nil, err
	}

	client := ssh.NewClient(c, chans, reqs)
	fmt.Println("SSH CONNECTED:", addr)

	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = sess.Close()
		client.Close()
		return nil, err
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		client.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		client.Close()
		return nil, err
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		_ = sess.Close()
		client.Close()
		return nil, err
	}

	if err := sess.Shell(); err != nil {
		_ = sess.Close()
		client.Close()
		return nil, err
	}

	ctx2, cancel := context.WithCancel(context.Background())

	ns := &NodeSession{
		Node:   host,
		client: client,
		sess:   sess,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		Output: make(chan []byte, 128),
		Done:   make(chan struct{}),
		cancel: cancel,
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ns.pump(ctx2, stdout)
	}()
	go func() {
		defer wg.Done()
		ns.pump(ctx2, stderr)
	}()

	// Close the session once both output streams are done (EOF / disconnect).
	go func() {
		wg.Wait()
		_ = ns.Close()
	}()

	return ns, nil
}

/*
Pump output
*/

func (s *NodeSession) pump(ctx context.Context, r io.Reader) {
	buf := make([]byte, 8192)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := r.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
			s.Output <- b
		}
		if err != nil {
			return
		}
	}
}

func (s *NodeSession) Write(p []byte) error {
	_, err := s.stdin.Write(p)
	return err
}

func (s *NodeSession) Resize(cols, rows int) error {
	if s.sess == nil {
		return nil
	}
	return s.sess.WindowChange(rows, cols)
}

func (s *NodeSession) Close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		close(s.Done)
		close(s.Output)
	})

	if s.sess != nil {
		_ = s.sess.Close()
	}
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

/*
Client config
*/

func buildClientConfig(
	host model.Host,
	passwordProvider func() (string, error),
) (*ssh.ClientConfig, func(), error) {

	auth, cleanup, err := authMethod(host, passwordProvider)
	if err != nil {
		return nil, nil, err
	}

	hkcb, err := hostKeyCallback(host)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, err
	}

	return &ssh.ClientConfig{
		User:            host.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hkcb,
		Timeout:         10 * time.Second,
	}, cleanup, nil
}

/*
Authentication
*/

func authMethod(
	host model.Host,
	passwordProvider func() (string, error),
) (ssh.AuthMethod, func(), error) {

	switch host.Auth.Method {

	case model.AuthPassword:
		if passwordProvider == nil {
			return nil, nil, errors.New("password provider not set")
		}
		pwd, err := passwordProvider()
		if err != nil {
			return nil, nil, err
		}
		return ssh.Password(pwd), nil, nil

	case model.AuthKey:
		kp := expandHome(host.Auth.KeyPath)
		if kp == "" {
			kp = expandHome("~/.ssh/id_rsa")
		}
		b, err := os.ReadFile(kp)
		if err != nil {
			return nil, nil, err
		}
		signer, err := ssh.ParsePrivateKey(b)
		if err != nil {
			return nil, nil, err
		}
		return ssh.PublicKeys(signer), nil, nil

	case model.AuthAgent:
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, nil, errors.New("SSH_AUTH_SOCK is not set")
		}
		conn, err := net.DialTimeout("unix", sock, 2*time.Second)
		if err != nil {
			return nil, nil, err
		}
		ag := agent.NewClient(conn)
		return ssh.PublicKeysCallback(ag.Signers), func() { _ = conn.Close() }, nil

	default:
		return nil, nil, fmt.Errorf("unknown auth method: %s", host.Auth.Method)
	}
}

/*
Host key verification
*/

func hostKeyCallback(host model.Host) (ssh.HostKeyCallback, error) {
	khPath := expandHome("~/.ssh/known_hosts")

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		fp := ssh.FingerprintSHA256(key)
		hostPort := knownhosts.Normalize(hostname)

		matcher, err := knownhosts.New(khPath)
		if err != nil {
			return err
		}

		err = matcher(hostname, remote, key)
		if err == nil {
			return nil
		}

		var kerr *knownhosts.KeyError
		if errors.As(err, &kerr) {

			// Unknown host
			if len(kerr.Want) == 0 {
				return ErrUnknownHostKey{
					HostPort:    hostPort,
					Fingerprint: fp,
					Key:         key,
				}
			}

			// Host key mismatch
			return ErrHostKeyMismatch{
				HostPort:    hostPort,
				Fingerprint: fp,
				Key:         key,
			}
		}

		return err
	}, nil
}

/*
Trust helper
*/

func TrustHostKey(hostPort string, key ssh.PublicKey) error {
	khPath := expandHome("~/.ssh/known_hosts")

	if err := os.MkdirAll(filepath.Dir(khPath), 0o700); err != nil {
		return err
	}

	line := knownhosts.Line([]string{hostPort}, key)

	f, err := os.OpenFile(khPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(line + "\n")
	return err
}

/*
Utils
*/

func expandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}
