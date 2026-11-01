package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// settings are the choices the TUI remembers between starts.
type settings struct {
	SidebarWidth int  `json:"sidebar_width,omitempty"`
	ListWidth    int  `json:"list_width,omitempty"`
	ImagesOff    bool `json:"images_off,omitempty"`
}

// loadSettings reads the settings file; a missing or unreadable file means defaults.
func loadSettings(path string) settings {
	var s settings
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

// saveSettings writes the current choices, through a temporary file so a crash can't
// leave a half-written file behind.
func (m *Model) saveSettings() {
	if m.settingsPath == "" {
		return
	}
	data, _ := json.Marshal(settings{SidebarWidth: m.sidebarWidth, ListWidth: m.listWidth, ImagesOff: m.imagesOff})
	tmp := m.settingsPath + ".tmp"
	err := os.MkdirAll(filepath.Dir(m.settingsPath), 0o755)
	if err == nil {
		err = os.WriteFile(tmp, data, 0o600)
	}
	if err == nil {
		err = os.Rename(tmp, m.settingsPath)
	}
	if err != nil {
		m.status, m.tone = "Kunde inte spara inställningarna: "+err.Error(), toneErr
	}
}
