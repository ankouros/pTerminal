package sshclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
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

var ErrPassphraseRequired = errors.New("passphrase_required")

type ErrKeyNotFound struct {
	Requested string
	Checked   []string
}

func (e *ErrKeyNotFound) Error() string {
	msg := "ssh key not found"
	if e == nil {
		return msg
	}
	if e.Requested != "" {
		msg += ": " + e.Requested
	}
	return msg
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

	output chan []byte
	done   chan struct{}

	cancel context.CancelFunc
	once   sync.Once
	wg     sync.WaitGroup
}

// DialClient establishes an SSH connection and returns an ssh.Client without
// starting a remote shell/pty. Callers must Close() the returned client.
func DialClient(
	ctx context.Context,
	host model.Host,
	passwordProvider func() (string, error),
) (*ssh.Client, func(), error) {
	cfg, cleanup, err := buildClientConfig(host, passwordProvider)
	if err != nil {
		return nil, nil, err
	}

	addr := net.JoinHostPort(host.Host, fmt.Sprint(host.Port))

	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, err
	}

	client := ssh.NewClient(c, chans, reqs)
	return client, cleanup, nil
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

	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	client := ssh.NewClient(c, chans, reqs)

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
		output: make(chan []byte, 512),
		done:   make(chan struct{}),
		cancel: cancel,
	}

	ns.wg.Add(2)

	go func() {
		defer ns.wg.Done()
		ns.pump(ctx2, stdout)
	}()
	go func() {
		defer ns.wg.Done()
		ns.pump(ctx2, stderr)
	}()

	// Close the session once both output streams are done (EOF / disconnect).
	go func() {
		ns.wg.Wait()
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
		n, err := r.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
			select {
			case <-ctx.Done():
				return
			case s.output <- b:
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *NodeSession) Output() <-chan []byte { return s.output }
func (s *NodeSession) Done() <-chan struct{} { return s.done }

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
	var ret error
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}

		// Closing the session/client unblocks stdout/stderr reads.
		if s.sess != nil {
			_ = s.sess.Close()
		}
		if s.client != nil {
			ret = s.client.Close()
		}

		close(s.done)
		s.wg.Wait()
		close(s.output)
	})
	return ret
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
		keyPath := expandHome(host.Auth.KeyPath)
		candidates := keyCandidates(keyPath)
		checked := make([]string, 0, len(candidates))

		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			candidate = expandHome(candidate)
			if candidate == "" {
				continue
			}
			checked = append(checked, candidate)
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}

			b, err := os.ReadFile(candidate)
			if err != nil {
				return nil, nil, err
			}
			signer, err := ssh.ParsePrivateKey(b)
			if err != nil {
				var missing *ssh.PassphraseMissingError
				if errors.As(err, &missing) {
					if passwordProvider == nil {
						return nil, nil, ErrPassphraseRequired
					}
					pass, perr := passwordProvider()
					if perr != nil || pass == "" {
						return nil, nil, ErrPassphraseRequired
					}
					signer, err = ssh.ParsePrivateKeyWithPassphrase(b, []byte(pass))
					if err != nil {
						return nil, nil, err
					}
				} else {
					return nil, nil, err
				}
			}
			return ssh.PublicKeys(signer), nil, nil
		}

		return nil, nil, &ErrKeyNotFound{
			Requested: keyPath,
			Checked:   checked,
		}

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

	case model.AuthKeyboardInteractive:
		if passwordProvider == nil {
			return nil, nil, errors.New("password provider not set")
		}
		return ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				ans, err := passwordProvider()
				if err != nil {
					return nil, err
				}
				answers[i] = ans
			}
			return answers, nil
		}), nil, nil

	default:
		return nil, nil, fmt.Errorf("unknown auth method: %s", host.Auth.Method)
	}
}

/*
Host key verification
*/

func hostKeyCallback(host model.Host) (ssh.HostKeyCallback, error) {
	if host.HostKey.Mode == model.HostKeyInsecure {
		return ssh.InsecureIgnoreHostKey(), nil
	}
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

func keyCandidates(requested string) []string {
	paths := make([]string, 0, 5)
	if strings.TrimSpace(requested) != "" {
		paths = append(paths, requested)
	}
	paths = append(paths,
		"~/.ssh/id_ed25519",
		"~/.ssh/id_ecdsa",
		"~/.ssh/id_rsa",
		"~/.ssh/id_dsa",
	)

	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
