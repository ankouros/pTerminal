package model

type AuthMethod string

const (
	AuthPassword            AuthMethod = "password"
	AuthKey                 AuthMethod = "key"
	AuthAgent               AuthMethod = "agent"
	AuthKeyboardInteractive AuthMethod = "keyboard-interactive"
)

type HostKeyMode string

const (
	HostKeyKnownHosts HostKeyMode = "known_hosts"
	HostKeyInsecure   HostKeyMode = "insecure"
)

type HostRole string

const (
	HostRoleGeneric  HostRole = "generic"
	HostRoleFabric   HostRole = "fabric"
	HostRolePlatform HostRole = "platform"
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
	DriverTelecom ConnectionDriver = "telecom"

	// DriverIOShell is a legacy alias kept for backward compatibility.
	DriverIOShell ConnectionDriver = "ioshell"
)

type TelecomConfig struct {
	// Path to the telecom entrypoint executable (local process).
	Path string `json:"path,omitempty"`

	// Protocol for telecom "-t" (e.g. ssh/telnet/socket/sea/cmd). If empty, defaults to "ssh".
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

// IOShellConfig is kept as a type alias for backward compatibility.
type IOShellConfig = TelecomConfig

type SFTPCredentialsMode string

const (
	SFTPCredsConnection SFTPCredentialsMode = "connection"
	SFTPCredsCustom     SFTPCredentialsMode = "custom"
)

type SFTPConfig struct {
	Enabled bool `json:"enabled,omitempty"`

	// Credentials controls whether SFTP uses the connection credentials or custom ones.
	// Defaults to "connection" when empty.
	Credentials SFTPCredentialsMode `json:"credentials,omitempty"`

	// Custom credentials (only when Credentials=custom).
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type Host struct {
	ID   int    `json:"id"`
	Name string `json:"name"`

	// UID is a stable identifier used for sync across peers.
	UID string `json:"uid,omitempty"`

	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`

	// Role labels the host for Samakia usage (fabric/platform/generic).
	Role HostRole `json:"role,omitempty"`

	// ManagedBy marks automated ownership (e.g. samakia-import).
	ManagedBy string `json:"managedBy,omitempty"`

	// Connection driver for this host. Defaults to "ssh".
	Driver ConnectionDriver `json:"driver,omitempty"`

	Auth    AuthConfig    `json:"auth"`
	HostKey HostKeyConfig `json:"hostKey"`

	Telecom *TelecomConfig `json:"telecom,omitempty"`

	// IOShell is a legacy field kept for backward compatibility with older exported configs.
	IOShell *IOShellConfig `json:"ioshell,omitempty"`

	// SFTP controls per-host SFTP settings (credentials may reuse the SSH connection ones).
	SFTP *SFTPConfig `json:"sftp,omitempty"`

	// Legacy (kept for backward compatibility with older exported configs).
	SFTPEnabled bool `json:"sftpEnabled,omitempty"`

	// Scope controls whether the host is private to this user or shared with a team.
	Scope Scope `json:"scope,omitempty"`

	// TeamID links this host to a team when Scope=team.
	TeamID string `json:"teamId,omitempty"`

	// Sync metadata for conflict detection.
	UpdatedAt int64          `json:"updatedAt,omitempty"`
	UpdatedBy string         `json:"updatedBy,omitempty"`
	Version   map[string]int `json:"version,omitempty"`
	Conflict  bool           `json:"conflict,omitempty"`
	Deleted   bool           `json:"deleted,omitempty"`
}

type Network struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Hosts []Host `json:"hosts"`

	// UID is a stable identifier used for sync across peers.
	UID string `json:"uid,omitempty"`

	// TeamID links this network to a team (empty = private/personal).
	TeamID string `json:"teamId,omitempty"`

	// Sync metadata for conflict detection.
	UpdatedAt int64          `json:"updatedAt,omitempty"`
	UpdatedBy string         `json:"updatedBy,omitempty"`
	Version   map[string]int `json:"version,omitempty"`
	Conflict  bool           `json:"conflict,omitempty"`
	Deleted   bool           `json:"deleted,omitempty"`
}

type AppConfig struct {
	Version  int          `json:"version"`
	User     UserProfile  `json:"user"`
	Teams    []Team       `json:"teams,omitempty"`
	Scripts  []TeamScript `json:"scripts,omitempty"`
	Networks []Network    `json:"networks"`
}
