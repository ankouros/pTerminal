package config

import (
	"testing"

	"github.com/ankouros/pterminal/internal/model"
)

func TestStripSecrets(t *testing.T) {
	cfg := model.AppConfig{
		Networks: []model.Network{
			{
				ID:   1,
				Name: "n1",
				Hosts: []model.Host{
					{
						ID:   1,
						Name: "h1",
						Auth: model.AuthConfig{
							Method:   model.AuthPassword,
							Password: "secret",
						},
						SFTP: &model.SFTPConfig{
							Enabled:     true,
							Credentials: model.SFTPCredsCustom,
							User:        "u",
							Password:    "sftp-secret",
						},
					},
				},
			},
		},
	}

	if !StripSecrets(&cfg) {
		t.Fatal("expected secrets to be stripped")
	}

	host := cfg.Networks[0].Hosts[0]
	if host.Auth.Password != "" {
		t.Fatalf("auth password not cleared: %q", host.Auth.Password)
	}
	if host.SFTP != nil && host.SFTP.Password != "" {
		t.Fatalf("sftp password not cleared: %q", host.SFTP.Password)
	}
}

func TestNormalizeSFTP(t *testing.T) {
	cfg := model.AppConfig{
		Networks: []model.Network{
			{
				ID:   1,
				Name: "n1",
				Hosts: []model.Host{
					{
						ID:          1,
						Name:        "h1",
						SFTPEnabled: true,
					},
				},
			},
		},
	}

	if !normalizeSFTP(&cfg) {
		t.Fatal("expected normalizeSFTP to report changes")
	}

	host := cfg.Networks[0].Hosts[0]
	if host.SFTP == nil || !host.SFTP.Enabled {
		t.Fatal("expected SFTP to be enabled")
	}
	if host.SFTP.Credentials != model.SFTPCredsConnection {
		t.Fatalf("unexpected credentials mode: %q", host.SFTP.Credentials)
	}
}
