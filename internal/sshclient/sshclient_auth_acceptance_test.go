package sshclient

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ankouros/pterminal/internal/model"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const (
	testSSHUser       = "pterminal-test"
	testSSHPassword   = "correct-horse"
	testSSHKiwiAnswer = "keyboard-kiwi"
)

const testSSHPrivateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
NhAAAAAwEAAQAAAQEAw2EnDftbXOb1sBHtPm5TQzzBMNPmel2aYkMjiZckb9AmIxqPSnmB
bd5BIHcWnweJ0TbIVoDlB7E85ePBYkGolJixm68alfUFi7cCjl/Uzs+iCD8rR6ewvOVKHX
omvA08A0/I2qpvr29gWjvEcupQGxCnwVbl9Cnn/6MshXK+sfnKRXLNhf5ZdlGUQlAIMVrJ
eUKkfhp3kbJhwt5LhJSpYmm2Ws1XMfrjHXjVHrL6kc9mPOIwcnRtmSmFLd1LPQaL/5RDf/
j0aFhNP6IMcnM47RYAvHXsc4zfDBNHaiPcWzwd5pC9O1v3M1SjKZvASI3iTSuZxR/ebaT9
Y6jRLhrN0QAAA8jLFRTdyxUU3QAAAAdzc2gtcnNhAAABAQDDYScN+1tc5vWwEe0+blNDPM
Ew0+Z6XZpiQyOJlyRv0CYjGo9KeYFt3kEgdxafB4nRNshWgOUHsTzl48FiQaiUmLGbrxqV
9QWLtwKOX9TOz6IIPytHp7C85Uodeia8DTwDT8jaqm+vb2BaO8Ry6lAbEKfBVuX0Kef/oy
yFcr6x+cpFcs2F/ll2UZRCUAgxWsl5QqR+GneRsmHC3kuElKliabZazVcx+uMdeNUesvqR
z2Y84jBydG2ZKYUt3Us9Bov/lEN/+PRoWE0/ogxyczjtFgC8dexzjN8ME0dqI9xbPB3mkL
07W/czVKMpm8BIjeJNK5nFH95tpP1jqNEuGs3RAAAAAwEAAQAAAQACbtupxcks2l7xoP2F
dyIADroAqcjfWfpN0jR3douAfXT2H7LsXGA/XiLNPNJqK1G86lvbEOqZOoytt7T9LGBlLl
Qa4la4Spd1tpMYcwrPQwBrbh7zutu9dHUEcjSYh6kpSOVxTKlMo9xNL1yaSj7yYVYXdyWw
sVNnaHCp3kSP6oz3KlHMLsP1/NCSjQ/mTI7dZX3VtK6s3sjLwC/NF0LbdNup+L6qWwCLXJ
udLId4byFz4pprYMXKDw5ztFl3oWn4PevCDtQTBxNrRdxTbUXpEfU3h/lKsJvLXrNv3xjs
fKnoVvZo5nbQP2ljdY+j6vWcvvy0FFrGRF3qftJ3OaCBAAAAgF8PXTNdJD5P/USVyqmbJj
RZpeY43Szbo9tCRYyL/WWPK0AkH8jk2hIy/5tHMhYXEEE+M3Vg3HSnMG1iB4g+tYj/EMyr
GysU+WBXBhKELFMs3SeRDEkMubFUQW/XuGaXwPs1a5Yi8bOT1jZ+wFSWAZeU7L1coq2h6V
C5AxTBCdMgAAAAgQDkNiq0Q6bKOMkaluMJthCCnsStAb9vQrY3cX/wqQpXq2PdCrqE/Z2h
GLWWByLmYajMEI5MQAm4RbRYn5J8QYsAsWA0JcUhgt710fQcxPRathKD4rZkR9sPnhg9KA
GNVl6F9R0T7ObQWOJkjs5kPdWHLCW3gRJ3hkxcJi5bwctiQQAAAIEA2yuKc/1aOE/B8BX0
mLygcoweJOg3T38vto9TSBFG+SaBwL4V1iIeUYyTkzndlx7n8QSDKUBGqU1jruKnwo8k8B
CPk0yv65Cx3AEtTCld+6HSqPcEZoRC5BmQSyCtaREmZleEISyeubSd4aPMCM5sb8Rq8WtI
dVV7J8XGYX1gZ5EAAAAPYWdnZWxvc0BzYW1ha2lhAQIDBA==
-----END OPENSSH PRIVATE KEY-----
`

func mustTestSigner(t *testing.T) ssh.Signer {
	t.Helper()
	signer, err := ssh.ParsePrivateKey([]byte(testSSHPrivateKey))
	if err != nil {
		t.Fatalf("parse test private key: %v", err)
	}
	return signer
}

func TestSSHAuthAcceptance(t *testing.T) {
	signer := mustTestSigner(t)
	authorizedKey := signer.PublicKey()

	scenarios := []struct {
		name             string
		hostSetup        func(t *testing.T) *model.Host
		passwordProvider func() (string, error)
		serverConfig     func(t *testing.T) *ssh.ServerConfig
		needsAgent       bool
	}{
		{
			name: "password",
			hostSetup: func(t *testing.T) *model.Host {
				return &model.Host{
					User:    testSSHUser,
					Auth:    model.AuthConfig{Method: model.AuthPassword},
					HostKey: model.HostKeyConfig{Mode: model.HostKeyInsecure},
				}
			},
			passwordProvider: func() (string, error) {
				return testSSHPassword, nil
			},
			serverConfig: func(t *testing.T) *ssh.ServerConfig {
				cfg := &ssh.ServerConfig{
					PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
						if conn.User() != testSSHUser {
							return nil, fmt.Errorf("unexpected user %s", conn.User())
						}
						if string(password) != testSSHPassword {
							return nil, errors.New("bad password")
						}
						return nil, nil
					},
				}
				cfg.AddHostKey(signer)
				return cfg
			},
		},
		{
			name: "key",
			hostSetup: func(t *testing.T) *model.Host {
				tmp := t.TempDir()
				keyPath := filepath.Join(tmp, "id_rsa")
				if err := os.WriteFile(keyPath, []byte(testSSHPrivateKey), 0o600); err != nil {
					t.Fatalf("write key: %v", err)
				}
				return &model.Host{
					User: testSSHUser,
					Auth: model.AuthConfig{
						Method:  model.AuthKey,
						KeyPath: keyPath,
					},
					HostKey: model.HostKeyConfig{Mode: model.HostKeyInsecure},
				}
			},
			serverConfig: func(t *testing.T) *ssh.ServerConfig {
				cfg := &ssh.ServerConfig{
					PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
						if conn.User() != testSSHUser {
							return nil, fmt.Errorf("unexpected user %s", conn.User())
						}
						if !bytes.Equal(key.Marshal(), authorizedKey.Marshal()) {
							return nil, errors.New("bad key")
						}
						return nil, nil
					},
				}
				cfg.AddHostKey(signer)
				return cfg
			},
		},
		{
			name: "agent",
			hostSetup: func(t *testing.T) *model.Host {
				return &model.Host{
					User:    testSSHUser,
					Auth:    model.AuthConfig{Method: model.AuthAgent},
					HostKey: model.HostKeyConfig{Mode: model.HostKeyInsecure},
				}
			},
			serverConfig: func(t *testing.T) *ssh.ServerConfig {
				cfg := &ssh.ServerConfig{
					PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
						if conn.User() != testSSHUser {
							return nil, fmt.Errorf("unexpected user %s", conn.User())
						}
						if !bytes.Equal(key.Marshal(), authorizedKey.Marshal()) {
							return nil, errors.New("bad key")
						}
						return nil, nil
					},
				}
				cfg.AddHostKey(signer)
				return cfg
			},
			needsAgent: true,
		},
		{
			name: "keyboard-interactive",
			hostSetup: func(t *testing.T) *model.Host {
				return &model.Host{
					User:    testSSHUser,
					Auth:    model.AuthConfig{Method: model.AuthKeyboardInteractive},
					HostKey: model.HostKeyConfig{Mode: model.HostKeyInsecure},
				}
			},
			passwordProvider: func() (string, error) {
				return testSSHKiwiAnswer, nil
			},
			serverConfig: func(t *testing.T) *ssh.ServerConfig {
				cfg := &ssh.ServerConfig{
					KeyboardInteractiveCallback: func(conn ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
						if conn.User() != testSSHUser {
							return nil, fmt.Errorf("unexpected user %s", conn.User())
						}
						answers, err := challenge("", "", []string{"favorite fruit?"}, []bool{false})
						if err != nil {
							return nil, err
						}
						if len(answers) != 1 || answers[0] != testSSHKiwiAnswer {
							return nil, errors.New("bad answer")
						}
						return nil, nil
					},
				}
				cfg.AddHostKey(signer)
				return cfg
			},
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.needsAgent {
				sock := setupTestAgent(t)
				t.Setenv("SSH_AUTH_SOCK", sock)
			}

			host := scenario.hostSetup(t)
			serverCfg := scenario.serverConfig(t)
			runSSHAuthScenario(t, host, serverCfg, scenario.passwordProvider)
		})
	}
}

func setupTestAgent(t *testing.T) string {
	t.Helper()
	keyring := agent.NewKeyring()
	rawKey, err := ssh.ParseRawPrivateKey([]byte(testSSHPrivateKey))
	if err != nil {
		t.Fatalf("parse raw key: %v", err)
	}
	cryptoSigner, ok := rawKey.(crypto.Signer)
	if !ok {
		t.Fatalf("unexpected raw key type %T", rawKey)
	}
	if err := keyring.Add(agent.AddedKey{PrivateKey: cryptoSigner, Comment: "pterminal-test"}); err != nil {
		t.Fatalf("add agent key: %v", err)
	}

	socketPath := filepath.Join(t.TempDir(), "ssh-agent.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() {
		_ = ln.Close()
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = agent.ServeAgent(keyring, c)
			}(conn)
		}
	}()

	return socketPath
}

func runSSHAuthScenario(t *testing.T, host *model.Host, serverCfg *ssh.ServerConfig, passwordProvider func() (string, error)) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		srvConn, chans, reqs, err := ssh.NewServerConn(conn, serverCfg)
		if err != nil {
			serverErr <- err
			return
		}
		go ssh.DiscardRequests(reqs)
		go func() {
			for ch := range chans {
				_ = ch.Reject(ssh.Prohibited, "not supported")
			}
		}()
		serverErr <- srvConn.Close()
	}()

	host.Host = ln.Addr().(*net.TCPAddr).IP.String()
	host.Port = ln.Addr().(*net.TCPAddr).Port

	client, cleanup, err := DialClient(context.Background(), *host, passwordProvider)
	if err != nil {
		t.Fatalf("dial client: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if err := client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("close client: %v", err)
	}

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("server handshake failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server handshake timed out")
	}
}
