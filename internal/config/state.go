package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// SessionState represents saved session state
type SessionState struct {
	LastSaved   time.Time         `yaml:"lastSaved"`
	Connections []SavedConnection `yaml:"connections"`
}

// SavedConnection represents a saved port-forward connection
type SavedConnection struct {
	Namespace    string `yaml:"namespace"`
	ResourceType string `yaml:"resourceType"` // "pod" or "service"
	ResourceName string `yaml:"resourceName"`
	LocalPort    int    `yaml:"localPort"`
	RemotePort   int    `yaml:"remotePort"`
	WasActive    bool   `yaml:"wasActive"` // was active when saved
}

// DefaultStatePath returns the default state file path
func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "portfwd", "state.yaml"), nil
}

// LoadState loads the session state from file
func LoadState() (*SessionState, error) {
	path, err := DefaultStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &SessionState{Connections: []SavedConnection{}}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state SessionState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// Save saves the session state to file
func (s *SessionState) Save() error {
	path, err := DefaultStatePath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	s.LastSaved = time.Now()

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// Clear removes all saved connections
func (s *SessionState) Clear() {
	s.Connections = []SavedConnection{}
}

// AddConnection adds a connection to the state
func (s *SessionState) AddConnection(conn SavedConnection) {
	// Check if already exists
	for i, c := range s.Connections {
		if c.Namespace == conn.Namespace &&
			c.ResourceType == conn.ResourceType &&
			c.ResourceName == conn.ResourceName &&
			c.LocalPort == conn.LocalPort {
			s.Connections[i] = conn
			return
		}
	}
	s.Connections = append(s.Connections, conn)
}

// RemoveConnection removes a connection from the state
func (s *SessionState) RemoveConnection(namespace, resourceType, resourceName string, localPort int) {
	for i, c := range s.Connections {
		if c.Namespace == namespace &&
			c.ResourceType == resourceType &&
			c.ResourceName == resourceName &&
			c.LocalPort == localPort {
			s.Connections = append(s.Connections[:i], s.Connections[i+1:]...)
			return
		}
	}
}
