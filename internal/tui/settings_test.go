package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// settingsModel is a model that remembers its settings in a file of its own.
func settingsModel(t *testing.T, path string, images bool) Model {
	t.Helper()
	m := sizedModel(t)
	m.settingsPath = path
	saved := loadSettings(path)
	m.sidebarWidth, m.listWidth, m.imagesOff = saved.SidebarWidth, saved.ListWidth, saved.ImagesOff
	m.imagesSupported, m.imagesOn = images, images && !m.imagesOff
	return m
}

func TestWidthsAndImageChoiceAreRemembered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := settingsModel(t, path, true)
	left, _, _ := m.dimensions()

	press(m, "alt+1", ">", ">", "I")
	m = settingsModel(t, path, true)
	if got, _, _ := m.dimensions(); got != left+2*resizeStep {
		t.Fatalf("sidebar = %d after a restart, want %d", got, left+2*resizeStep)
	}
	if m.imagesOn {
		t.Fatal("images turned off with I should stay off after a restart")
	}

	m = press(m, "=")
	if s := loadSettings(path); s.SidebarWidth != 0 || s.ListWidth != 0 || !s.ImagesOff {
		t.Fatalf("settings = %+v: = should reset only the widths", s)
	}
}

func TestDraggingSavesOnRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := settingsModel(t, path, false)
	left, _, _ := m.dimensions()

	m = update(m, tea.MouseClickMsg{X: left - 1, Y: 5, Button: tea.MouseLeft})
	m = update(m, tea.MouseMotionMsg{X: 40, Y: 5, Button: tea.MouseLeft})
	if _, err := os.Stat(path); err == nil {
		t.Fatal("dragging should not write the file on every movement")
	}
	update(m, tea.MouseReleaseMsg{X: 40, Y: 5, Button: tea.MouseLeft})
	if s := loadSettings(path); s.SidebarWidth != 41 {
		t.Fatalf("sidebar width = %d, want 41 saved on release", s.SidebarWidth)
	}
}

func TestImageChoiceSurvivesTerminalsWithoutImages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	press(settingsModel(t, path, true), "I") // images off in a terminal that supports them

	press(settingsModel(t, path, false), "alt+1", ">") // resized in one that doesn't
	if !loadSettings(path).ImagesOff {
		t.Fatal("saving in a terminal without images must keep the earlier choice")
	}
}

func TestBrokenSettingsFileMeansDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := loadSettings(path); s != (settings{}) {
		t.Fatalf("settings = %+v, want defaults", s)
	}
}
