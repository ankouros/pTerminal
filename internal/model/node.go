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

type ConnectionDriver string

const (
	DriverSSH     ConnectionDriver = "ssh"
	DriverIOShell ConnectionDriver = "ioshell"
)

type IOShellConfig struct {
	// Path to ioshell entrypoint, e.g. /home/aggelos/IOshell/ioshell_local or /home/aggelos/IOshell/bin/ioshell
	Path string `json:"path,omitempty"`

	// Protocol for ioshell "-t" (e.g. ssh/telnet/socket/sea/cmd). If empty, defaults to "ssh".
	Protocol string `json:"protocol,omitempty"`

	// Command is the first command to be written to the session after connect (best-effort).
	Command string `json:"command,omitempty"`

	// Args are passed as-is (no shell). If empty, pTerminal builds args using Protocol + host fields.
	// Placeholders supported by pTerminal:
	// {host} {port} {user} {name} {id}
	Args []string `json:"args,omitempty"`

	// Optional working directory for the process.
	WorkDir string `json:"workDir,omitempty"`

	// Optional extra environment variables (merged with the current environment).
	Env map[string]string `json:"env,omitempty"`
}

type Host struct {
	ID   int    `json:"id"`
	Name string `json:"name"`

	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`

	// Connection driver for this host. Defaults to "ssh".
	Driver ConnectionDriver `json:"driver,omitempty"`

	Auth    AuthConfig    `json:"auth"`
	HostKey HostKeyConfig `json:"hostKey"`

	IOShell *IOShellConfig `json:"ioshell,omitempty"`

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
