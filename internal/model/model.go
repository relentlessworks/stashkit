package model

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"
)

// Entry represents a key-value store entry.
type Entry struct {
	Handle    string    `json:"handle"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Namespace string    `json:"namespace,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Workspace represents a tenant workspace.
type Workspace struct {
	Handle    string    `json:"handle"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID        int       `json:"id"`
	Workspace string    `json:"workspace"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	Actor     string    `json:"actor"`
	Timestamp time.Time `json:"timestamp"`
}

// generateHandle creates a short, unique handle with the given prefix.
// Format: prefix_5char (e.g. stash_a1b2c, ws_abc12)
func generateHandle(prefix string) string {
	b := make([]byte, 5)
	rand.Read(b)
	enc := base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)
	return fmt.Sprintf("%s_%s", prefix, enc.EncodeToString(b)[:5])
}

// NewStashHandle generates a handle for a stash entry.
func NewStashHandle() string {
	return generateHandle("stash")
}

// NewWorkspaceHandle generates a handle for a workspace.
func NewWorkspaceHandle() string {
	return generateHandle("ws")
}

// IsExpired checks if the entry has expired.
func (e *Entry) IsExpired() bool {
	if e.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*e.ExpiresAt)
}
