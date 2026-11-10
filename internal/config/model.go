package config

import "time"

// Status is the persisted lifecycle state of a database instance.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusError    Status = "error"
)

// Instance is the persisted identity and last-known runtime state of one H2 database.
// ServerPassword authenticates H2's remote shutdown tool and is never sent to the frontend.
type Instance struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Username         string     `json:"username"`
	Password         string     `json:"password"`
	ServerPassword   string     `json:"serverPassword"`
	Port             int        `json:"port"`
	Status           Status     `json:"status"`
	PID              int        `json:"pid,omitempty"`
	TailscalePort    int        `json:"tailscalePort"`
	TailscaleEnabled bool       `json:"tailscaleEnabled"`
	TailscaleIP      string     `json:"tailscaleIp,omitempty"`
	FunnelEnabled    bool       `json:"funnelEnabled,omitempty"`
	FunnelPort       int        `json:"funnelPort,omitempty"`
	FunnelHost       string     `json:"funnelHost,omitempty"`
	DatabaseFile     string     `json:"databaseFile"`
	FileSizeBytes    int64      `json:"fileSizeBytes"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
}

// Clone returns a deep copy safe to hand to other goroutines.
func (i Instance) Clone() Instance {
	out := i
	if i.StartedAt != nil {
		started := *i.StartedAt
		out.StartedAt = &started
	}
	return out
}

// Settings are user-overridable paths, persisted with the instance list.
type Settings struct {
	H2JarPath     string `json:"h2JarPath"`
	JavaPath      string `json:"javaPath"`
	TailscalePath string `json:"tailscalePath"`
}

// State is the on-disk schema. Version is reserved for migrations.
type State struct {
	Version   int        `json:"version"`
	Settings  Settings   `json:"settings"`
	Instances []Instance `json:"instances"`
}
