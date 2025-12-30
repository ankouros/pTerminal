package sftpclient

import (
	"testing"

	"github.com/ankouros/pterminal/internal/model"
)

func TestCleanRemotePath(t *testing.T) {
	cases := map[string]string{
		"":           ".",
		" /tmp ":     "/tmp",
		"foo/../bar": "bar",
		"a\\b":       "a/b",
		"~/data":     "~/data",
	}
	for in, expected := range cases {
		if got := cleanRemotePath(in); got != expected {
			t.Fatalf("cleanRemotePath(%q) = %q, want %q", in, got, expected)
		}
	}
}

func TestSFTPCustomPasswordCache(t *testing.T) {
	mgr := NewManager(model.AppConfig{})

	host := model.Host{
		ID:   7,
		Name: "h1",
		User: "u1",
		SFTP: &model.SFTPConfig{
			Enabled:     true,
			Credentials: model.SFTPCredsCustom,
			User:        "sftp-user",
		},
	}

	mgr.SetCustomPassword(host.ID, "secret")

	user, pwFn, err := mgr.sftpAuth(host, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user != "sftp-user" {
		t.Fatalf("unexpected user: %q", user)
	}
	pw, err := pwFn()
	if err != nil {
		t.Fatalf("unexpected pw error: %v", err)
	}
	if pw != "secret" {
		t.Fatalf("unexpected password: %q", pw)
	}
}
