package model

type Scope string

const (
	ScopePrivate Scope = "private"
	ScopeTeam    Scope = "team"
)

const (
	TeamRoleAdmin = "admin"
	TeamRoleUser  = "user"
)

type UserProfile struct {
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
	DeviceID string `json:"deviceId,omitempty"`
}

type TeamMember struct {
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Role  string `json:"role,omitempty"`
}

type Team struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Members   []TeamMember   `json:"members,omitempty"`
	UpdatedAt int64          `json:"updatedAt,omitempty"`
	UpdatedBy string         `json:"updatedBy,omitempty"`
	Version   map[string]int `json:"version,omitempty"`
	Conflict  bool           `json:"conflict,omitempty"`
	Deleted   bool           `json:"deleted,omitempty"`
}

type TeamScript struct {
	ID          string         `json:"id,omitempty"`
	TeamID      string         `json:"teamId,omitempty"`
	Scope       Scope          `json:"scope,omitempty"`
	Name        string         `json:"name,omitempty"`
	Command     string         `json:"command,omitempty"`
	Description string         `json:"description,omitempty"`
	UpdatedAt   int64          `json:"updatedAt,omitempty"`
	UpdatedBy   string         `json:"updatedBy,omitempty"`
	Version     map[string]int `json:"version,omitempty"`
	Conflict    bool           `json:"conflict,omitempty"`
	Deleted     bool           `json:"deleted,omitempty"`
}
