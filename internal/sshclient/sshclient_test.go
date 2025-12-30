package sshclient

import (
	"testing"

	"github.com/ankouros/pterminal/internal/model"
)

func TestAuthMethodKeyboardInteractive(t *testing.T) {
	host := model.Host{
		Auth: model.AuthConfig{
			Method: model.AuthKeyboardInteractive,
		},
	}

	t.Run("no provider", func(t *testing.T) {
		if _, _, err := authMethod(host, nil); err == nil {
			t.Fatal("expected error when password provider is missing")
		}
	})

	t.Run("with provider", func(t *testing.T) {
		called := 0
		auth, _, err := authMethod(host, func() (string, error) {
			called++
			return "secret", nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if auth == nil {
			t.Fatalf("expected auth method, got nil")
		}
		if called != 0 {
			t.Fatalf("keyboard-interactive should not invoke provider before handshake")
		}
	})
}
