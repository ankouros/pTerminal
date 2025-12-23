package p2p

import "github.com/ankouros/pterminal/internal/model"

type TeamSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PeerInfo struct {
	DeviceID string        `json:"deviceId"`
	Name     string        `json:"name,omitempty"`
	Email    string        `json:"email,omitempty"`
	Host     string        `json:"host,omitempty"`
	Addr     string        `json:"addr,omitempty"`
	TCPPort  int           `json:"tcpPort,omitempty"`
	LastSeen int64         `json:"lastSeen,omitempty"`
	Teams    []TeamSummary `json:"teams,omitempty"`
}

type PresenceSnapshot struct {
	Peers []PeerInfo        `json:"peers"`
	User  model.UserProfile `json:"user"`
}
