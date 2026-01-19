package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Profiles []Profile `yaml:"profiles"`
}

// Profile represents a saved port-forward profile
type Profile struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description,omitempty"`
	Forwards    []ForwardSpec `yaml:"forwards"`
}

// ForwardSpec represents a single port-forward specification
type ForwardSpec struct {
	Namespace  string `yaml:"namespace"`
	Pod        string `yaml:"pod,omitempty"`
	Service    string `yaml:"service,omitempty"`
	LocalPort  int    `yaml:"localPort"`
	RemotePort int    `yaml:"remotePort"`
}

// DefaultConfigPath returns the default configuration file path
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "portfwd", "config.yaml"), nil
}

// Load loads the configuration from a file
func Load(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Profiles: []Profile{}}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// Save saves the configuration to a file
func (c *Config) Save(path string) error {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetProfile returns a profile by name
func (c *Config) GetProfile(name string) (*Profile, error) {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			return &c.Profiles[i], nil
		}
	}
	return nil, fmt.Errorf("profile not found: %s", name)
}

// AddProfile adds or updates a profile
func (c *Config) AddProfile(profile Profile) {
	for i := range c.Profiles {
		if c.Profiles[i].Name == profile.Name {
			c.Profiles[i] = profile
			return
		}
	}
	c.Profiles = append(c.Profiles, profile)
}

// DeleteProfile deletes a profile by name
func (c *Config) DeleteProfile(name string) error {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			c.Profiles = append(c.Profiles[:i], c.Profiles[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("profile not found: %s", name)
}

// ListProfiles returns all profile names
func (c *Config) ListProfiles() []string {
	names := make([]string, len(c.Profiles))
	for i, p := range c.Profiles {
		names[i] = p.Name
	}
	return names
}

// Validate validates the configuration
func (c *Config) Validate() error {
	seen := make(map[string]bool)
	for _, p := range c.Profiles {
		if p.Name == "" {
			return fmt.Errorf("profile name cannot be empty")
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate profile name: %s", p.Name)
		}
		seen[p.Name] = true

		for _, f := range p.Forwards {
			if f.Namespace == "" {
				return fmt.Errorf("namespace cannot be empty in profile %s", p.Name)
			}
			if f.Pod == "" && f.Service == "" {
				return fmt.Errorf("either pod or service must be specified in profile %s", p.Name)
			}
			if f.LocalPort <= 0 || f.LocalPort > 65535 {
				return fmt.Errorf("invalid local port %d in profile %s", f.LocalPort, p.Name)
			}
			if f.RemotePort <= 0 || f.RemotePort > 65535 {
				return fmt.Errorf("invalid remote port %d in profile %s", f.RemotePort, p.Name)
			}
		}
	}
	return nil
}
