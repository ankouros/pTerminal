package main

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

    "github.com/webview/webview/v2"
	"golang.org/x/crypto/ssh"
)

//go:embed web/index.html
var indexHTML string

func main() {
	debug := false

	w := webview.New(debug)
	defer w.Destroy()

	w.SetTitle("pTerminal")
	w.SetSize(1200, 800, webview.HintNone)

	// SSH session state
	var stdin net.Conn
	var stdout net.Conn

	// Called from JS when user types
	w.Bind("goWrite", func(data string) {
		if stdin != nil {
			stdin.Write([]byte(data))
		}
	})

	// Connect to SSH (hardcoded for prototype)
	w.Bind("goConnect", func() {
		go func() {
			user := "aggelos"
			addr := "192.168.11.231:4421"
			password := "paparia!23"

			cfg := &ssh.ClientConfig{
				User: user,
				Auth: []ssh.AuthMethod{
					ssh.Password(password),
				},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			}

			client, err := ssh.Dial("tcp", addr, cfg)
			if err != nil {
				log.Println(err)
				return
			}

			sess, err := client.NewSession()
			if err != nil {
				log.Println(err)
				return
			}

			modes := ssh.TerminalModes{
				ssh.ECHO:          1,
				ssh.TTY_OP_ISPEED: 14400,
				ssh.TTY_OP_OSPEED: 14400,
			}

			if err := sess.RequestPty("xterm-256color", 40, 120, modes); err != nil {
				log.Println(err)
				return
			}

			in, _ := sess.StdinPipe()
			out, _ := sess.StdoutPipe()
			sess.Stderr = out

			sess.Shell()

			stdin = wrapConn(in)
			stdout = wrapConn(out)

			// Read SSH output → JS
			buf := make([]byte, 4096)
			for {
				n, err := stdout.Read(buf)
				if err != nil {
					return
				}
				data := string(buf[:n])
				data = strings.ReplaceAll(data, "\\", "\\\\")
				data = strings.ReplaceAll(data, "`", "\\`")
				data = strings.ReplaceAll(data, "$", "\\$")
				data = strings.ReplaceAll(data, "\n", "\\n")

				w.Dispatch(func() {
					w.Eval(fmt.Sprintf("termWrite(`%s`)", data))
				})
			}
		}()
	})

	w.Navigate("data:text/html," + indexHTML)
	w.Run()
}

// Wrap io pipes into net.Conn-like interface
type pipeConn struct{ f *os.File }

func (p pipeConn) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p pipeConn) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p pipeConn) Close() error                { return p.f.Close() }
func (p pipeConn) LocalAddr() net.Addr         { return nil }
func (p pipeConn) RemoteAddr() net.Addr        { return nil }
func (p pipeConn) SetDeadline(t time.Time) error      { return nil }
func (p pipeConn) SetReadDeadline(t time.Time) error  { return nil }
func (p pipeConn) SetWriteDeadline(t time.Time) error { return nil }

func wrapConn(f interface{}) net.Conn {
	return pipeConn{f.(*os.File)}
}
