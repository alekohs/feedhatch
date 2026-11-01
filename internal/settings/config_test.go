package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsHaveNotificationsOn(t *testing.T) {
	dir := t.TempDir()
	if c := Load(filepath.Join(dir, "missing.json")); !c.Notifications {
		t.Fatal("without a settings file, notifications should be on")
	}
	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c := Load(broken); c != (Config{Notifications: true}) {
		t.Fatalf("a broken file should mean defaults, got %+v", c)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "settings.json")
	want := Config{Notifications: false, SidebarWidth: 30, ListWidth: 50, ImagesOff: true}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
