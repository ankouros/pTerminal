package model

type AuthMethod string

const (
	AuthPassword AuthMethod = "password"
	AuthKey      AuthMethod = "key"
	AuthAgent    AuthMethod = "agent"
)

type HostKeyMode string

const (
	HostKeyKnownHosts HostKeyMode = "known_hosts"
	HostKeyInsecure   HostKeyMode = "insecure"
)

type AuthConfig struct {
	Method   AuthMethod `json:"method"`
	KeyPath  string     `json:"keyPath,omitempty"`  // when method=key
	Password string     `json:"password,omitempty"` // when method=password (stored in config)
}

type HostKeyConfig struct {
	Mode HostKeyMode `json:"mode,omitempty"` // known_hosts / insecure
}

type Host struct {
	ID   int    `json:"id"`
	Name string `json:"name"`

	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`

	Auth    AuthConfig    `json:"auth"`
	HostKey HostKeyConfig `json:"hostKey"`

	// Placeholders for future features
	SFTPEnabled bool `json:"sftpEnabled,omitempty"`
}

type Network struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Hosts []Host `json:"hosts"`
}

type AppConfig struct {
	Version  int       `json:"version"`
	Networks []Network `json:"networks"`
}
