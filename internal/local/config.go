package local

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// CLIConfig is ~/.config/deckhand/config.toml (brief §6).
type CLIConfig struct {
	Hub struct {
		URL   string `toml:"url"`
		Token string `toml:"token"`
	} `toml:"hub"`
}

// ConfigPath honours XDG_CONFIG_HOME.
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "deckhand", "config.toml")
}

// LoadConfig reads the file; a missing file is an empty config.
func LoadConfig() (CLIConfig, error) {
	var c CLIConfig
	_, err := toml.DecodeFile(ConfigPath(), &c)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if tok := os.Getenv("DECKHAND_TOKEN"); tok != "" {
		c.Hub.Token = tok
	}
	if u := os.Getenv("DECKHAND_HUB"); u != "" {
		c.Hub.URL = u
	}
	return c, err
}

// SaveConfig writes the file with private permissions (it holds the token).
func SaveConfig(c CLIConfig) error {
	p := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return toml.NewEncoder(f).Encode(c)
}
