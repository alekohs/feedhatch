package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"alekohs/feedhatch/internal/settings"
)

// settingsModel is a model that remembers its settings in a file of its own.
func settingsModel(t *testing.T, path string, images bool) Model {
	t.Helper()
	m := sizedModel(t)
	m.settingsPath = path
	saved := settings.Load(path)
	m.sidebarWidth, m.listWidth, m.imagesOff, m.notificationsOn = saved.SidebarWidth, saved.ListWidth, saved.ImagesOff, saved.Notifications
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
	if s := settings.Load(path); s.SidebarWidth != 0 || s.ListWidth != 0 || !s.ImagesOff {
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
	if s := settings.Load(path); s.SidebarWidth != 41 {
		t.Fatalf("sidebar width = %d, want 41 saved on release", s.SidebarWidth)
	}
}

func TestImageChoiceSurvivesTerminalsWithoutImages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	press(settingsModel(t, path, true), "I") // images off in a terminal that supports them

	press(settingsModel(t, path, false), "alt+1", ">") // resized in one that doesn't
	if !settings.Load(path).ImagesOff {
		t.Fatal("saving in a terminal without images must keep the earlier choice")
	}
}

func TestFeedNotificationsAndTheMasterSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := settingsModel(t, path, false)
	m = press(m, "alt+1")
	m.view = feedView
	feed := m.feeds[m.feedIndex]

	m = press(m, "B")
	if !m.feeds[m.feedIndex].notify || m.tone != toneOK {
		t.Fatalf("B should turn on notifications for %s: %q", feed.name, m.status)
	}
	if stored, _ := m.store.Feeds(); !stored[m.feedIndex].Notification {
		t.Fatal("the feed's choice should be saved in the database")
	}
	if !strings.Contains(m.sidebar(40, 20), feed.name+" ♪") {
		t.Fatal("the sidebar should mark feeds that notify")
	}

	m = press(m, "alt+b")
	if m.notificationsOn || settings.Load(path).Notifications {
		t.Fatal("Alt+b should turn all notifications off and remember it")
	}
	m = press(m, "B", "B") // off, then on again while the switch is off
	if m.tone != toneWarn || !strings.Contains(m.status, "Alt+b") {
		t.Fatalf("turning a feed on while all notifications are off should say so: %q", m.status)
	}

	m.view = unreadView
	m = press(m, "B")
	if m.tone != toneWarn || m.status != "Välj ett riktigt flöde." {
		t.Fatal("B only applies to a real feed")
	}
}
