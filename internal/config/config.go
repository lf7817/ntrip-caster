// Package config provides configuration loading and types for the NTRIP Caster.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for the NTRIP Caster.
type Config struct {
	Server             ServerConfig             `yaml:"server"`
	Auth               AuthConfig               `yaml:"auth"`
	Database           DatabaseConfig           `yaml:"database"`
	Limits             LimitsConfig             `yaml:"limits"`
	MountpointDefaults MountpointDefaultsConfig `yaml:"mountpoint_defaults"`
}

// ServerConfig holds network listener addresses.
type ServerConfig struct {
	Listen      string `yaml:"listen"`
	AdminListen string `yaml:"admin_listen"`
}

// AuthConfig controls authentication behaviour.
type AuthConfig struct {
	Enabled         bool   `yaml:"enabled"`
	AdminMode       string `yaml:"admin_mode"`
	NtripSourceAuth string `yaml:"ntrip_source_auth"`
	NtripRoverAuth  string `yaml:"ntrip_rover_auth"`
}

// DatabaseConfig specifies the backing store.
type DatabaseConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

// LimitsConfig sets global resource limits.
type LimitsConfig struct {
	MaxClients    int `yaml:"max_clients"`
	MaxConnPerIP  int `yaml:"max_conn_per_ip"`
}

// MountpointDefaultsConfig provides default values applied to all mountpoints
// unless overridden per-mountpoint.
type MountpointDefaultsConfig struct {
	WriteQueue   int           `yaml:"write_queue"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// Load reads a YAML configuration file from path and returns a Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:      ":2101",
			AdminListen: ":8080",
		},
		Auth: AuthConfig{
			Enabled:         true,
			AdminMode:       "session",
			NtripSourceAuth: "user_binding",
			NtripRoverAuth:  "basic",
		},
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: "caster.db",
		},
		Limits: LimitsConfig{
			MaxClients:   5000,
			MaxConnPerIP: 10,
		},
		MountpointDefaults: MountpointDefaultsConfig{
			WriteQueue:   64,
			WriteTimeout: 3 * time.Second,
		},
	}
}
