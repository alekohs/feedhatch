// Package settings holds the user's choices, remembered in a JSON file next to the
// database so both the TUI and the daemon read the same ones.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	// Notifications is the master switch for OS notifications; each feed also has its own.
	Notifications bool `json:"notifications"`
	SidebarWidth  int  `json:"sidebar_width,omitempty"`
	ListWidth     int  `json:"list_width,omitempty"`
	ImagesOff     bool `json:"images_off,omitempty"`
}

// Load reads the settings file; a missing or unreadable file means the defaults, with
// notifications on.
func Load(path string) Config {
	c := Config{Notifications: true}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

// Ensure creates the settings file with the defaults when it doesn't exist yet, so
// users can find it and edit it.
func Ensure(path string) error {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return Load(path).Save(path)
}

// Save writes the settings through a temporary file, so a crash can't leave a
// half-written file behind.
func (c Config) Save(path string) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
