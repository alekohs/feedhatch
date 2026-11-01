package tui

import "alekohs/feedhatch/internal/settings"

// saveSettings remembers the current choices in the settings file.
func (m *Model) saveSettings() {
	if m.settingsPath == "" {
		return
	}
	c := settings.Config{Notifications: m.notificationsOn, SidebarWidth: m.sidebarWidth, ListWidth: m.listWidth, ImagesOff: m.imagesOff}
	if err := c.Save(m.settingsPath); err != nil {
		m.status, m.tone = "Kunde inte spara inställningarna: "+err.Error(), toneErr
	}
}
