package cmdclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ankouros/pterminal/internal/model"
	"github.com/ankouros/pterminal/internal/terminal"
	"github.com/creack/pty"
)

type ProcessSession struct {
	Host model.Host

	cmd *exec.Cmd
	pty *os.File

	output chan []byte
	done   chan struct{}

	postCommand string
	commandSent bool

	userInteracted bool
	promptBuf      string

	once sync.Once
}

var _ terminal.Session = (*ProcessSession)(nil)

func StartIOShell(ctx context.Context, host model.Host, cols, rows int) (*ProcessSession, error) {
	if host.IOShell == nil {
		return nil, errors.New("ioshell config is missing")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path := strings.TrimSpace(host.IOShell.Path)
	if path == "" {
		return nil, errors.New("ioshell path is empty")
	}

	args := applyPlaceholders(host.IOShell.Args, host)
	if len(args) == 0 {
		proto := strings.TrimSpace(host.IOShell.Protocol)
		if proto == "" {
			proto = "ssh"
		}

		identity := host.User + "@" + host.Host
		args = []string{
			"-t", proto,
			"-p", fmt.Sprint(host.Port),
			"-i", identity,
		}
	}

	cmd := exec.Command(path, args...) //nolint:gosec // user-configured executable path

	// cwd
	if wd := strings.TrimSpace(host.IOShell.WorkDir); wd != "" {
		cmd.Dir = wd
	} else if root := inferIOSHELLRoot(path); root != "" {
		cmd.Dir = root
	}

	// env
	cmd.Env = os.Environ()
	if root := inferIOSHELLRoot(path); root != "" && !hasEnvKey(host.IOShell.Env, "IOSHELLROOT") {
		cmd.Env = append(cmd.Env, "IOSHELLROOT="+root)
	}
	for k, v := range host.IOShell.Env {
		if k == "" {
			continue
		}
		cmd.Env = append(cmd.Env, k+"="+applyPlaceholdersOne(v, host))
	}

	f, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("start ioshell: %w", err)
	}

	s := &ProcessSession{
		Host:        host,
		cmd:         cmd,
		pty:         f,
		output:      make(chan []byte, 128),
		done:        make(chan struct{}),
		postCommand: strings.TrimSpace(host.IOShell.Command),
	}

	_ = s.Resize(cols, rows)

	go s.pump(f)
	go func() {
		_ = cmd.Wait()
		_ = s.Close()
	}()

	return s, nil
}

func (s *ProcessSession) Output() <-chan []byte { return s.output }
func (s *ProcessSession) Done() <-chan struct{} { return s.done }

func (s *ProcessSession) pump(r io.Reader) {
	buf := make([]byte, 8192)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			b := make([]byte, n)
			copy(b, buf[:n])
			s.maybeRunCommand(b)
			select {
			case s.output <- b:
			default:
				// drop if UI can't keep up; output is also buffered in manager
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *ProcessSession) maybeRunCommand(chunk []byte) {
	// Only attempt the "first command" after the user has interacted at least once.
	// This avoids interfering with IOshell's own login prompts/flows.
	if s.commandSent || s.postCommand == "" || s.pty == nil || !s.userInteracted {
		return
	}

	s.promptBuf += string(chunk)
	if len(s.promptBuf) > 4096 {
		s.promptBuf = s.promptBuf[len(s.promptBuf)-4096:]
	}

	l := strings.ToLower(s.promptBuf)
	if strings.Contains(l, "cannot log in") || strings.Contains(l, "expect-telnet") {
		return
	}

	lastLine := s.lastNonEmptyLine()
	if lastLine == "" {
		return
	}

	if s.looksLikeInteractivePrompt(lastLine) {
		s.commandSent = true
		_, _ = s.pty.Write([]byte(s.postCommand + "\n"))
	}
}

func (s *ProcessSession) lastNonEmptyLine() string {
	// Take the last non-empty line from the recent prompt buffer.
	buf := strings.ReplaceAll(s.promptBuf, "\r\n", "\n")
	buf = strings.ReplaceAll(buf, "\r", "\n")
	lines := strings.Split(buf, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func (s *ProcessSession) looksLikeInteractivePrompt(line string) bool {
	// Heuristic: avoid sending during login prompts; only send when we see a
	// likely "ready for commands" prompt.
	l := strings.ToLower(strings.TrimSpace(line))
	if l == "" {
		return false
	}
	if strings.Contains(l, "username") ||
		strings.Contains(l, "password") ||
		strings.Contains(l, "safeword") ||
		strings.Contains(l, "please enter") ||
		strings.Contains(l, "connecting to ") ||
		strings.Contains(l, "cannot log in") ||
		strings.Contains(l, "disconnected") {
		return false
	}

	trimRight := strings.TrimRight(line, " \t")
	if trimRight == "" {
		return false
	}
	last := trimRight[len(trimRight)-1]
	switch last {
	case '>', '#', '$', '%':
		return true
	default:
		return false
	}
}

func (s *ProcessSession) Write(p []byte) error {
	if s.pty == nil {
		return errors.New("process not running")
	}
	s.userInteracted = true
	_, err := s.pty.Write(p)
	return err
}

func (s *ProcessSession) Resize(cols, rows int) error {
	if s.pty == nil || cols <= 0 || rows <= 0 {
		return nil
	}
	return pty.Setsize(s.pty, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

func (s *ProcessSession) Close() error {
	var err error
	s.once.Do(func() {
		close(s.done)
		close(s.output)

		if s.pty != nil {
			_ = s.pty.Close()
			s.pty = nil
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
			_, _ = s.cmd.Process.Wait()
		}
	})
	return err
}

func inferIOSHELLRoot(path string) string {
	p := filepath.Clean(path)
	// /.../IOshell/bin/ioshell -> /.../IOshell
	if strings.HasSuffix(p, string(filepath.Separator)+"bin"+string(filepath.Separator)+"ioshell") {
		return filepath.Dir(filepath.Dir(p))
	}
	// /.../IOshell/ioshell_local -> /.../IOshell
	if strings.HasSuffix(p, string(filepath.Separator)+"ioshell_local") {
		return filepath.Dir(p)
	}
	return ""
}

func hasEnvKey(env map[string]string, key string) bool {
	if env == nil {
		return false
	}
	_, ok := env[key]
	return ok
}

func applyPlaceholders(args []string, host model.Host) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		out = append(out, applyPlaceholdersOne(a, host))
	}
	return out
}

func applyPlaceholdersOne(s string, host model.Host) string {
	s = strings.ReplaceAll(s, "{host}", host.Host)
	s = strings.ReplaceAll(s, "{port}", fmt.Sprint(host.Port))
	s = strings.ReplaceAll(s, "{user}", host.User)
	s = strings.ReplaceAll(s, "{name}", host.Name)
	s = strings.ReplaceAll(s, "{id}", fmt.Sprint(host.ID))
	return s
}
