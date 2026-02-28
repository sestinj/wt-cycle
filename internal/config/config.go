package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds user configuration from ~/.config/wt-cycle/config.json.
type Config struct {
	Skip []string `json:"skip"`
}

// Load reads the config file. Returns zero-value Config on any error.
func Load() Config {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "wt-cycle", "config.json"))
	if err != nil {
		return Config{}
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}
	}
	return cfg
}

// ShouldSkip returns true if the given repo root is in the skip list.
func (c Config) ShouldSkip(repoRoot string) bool {
	for _, s := range c.Skip {
		if s == repoRoot {
			return true
		}
	}
	return false
}
