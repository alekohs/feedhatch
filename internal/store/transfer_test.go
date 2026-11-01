package store

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExportCopiesFeedsArticlesAndReadState(t *testing.T) {
	s := openTest(t)
	if err := s.SeedMock(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "export.db")

	if err := s.Export(path); err != nil {
		t.Fatal(err)
	}

	copied, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	feeds, err := copied.Feeds()
	if err != nil {
		t.Fatal(err)
	}
	if len(feeds) != 4 {
		t.Fatalf("got %d feeds, want 4", len(feeds))
	}
	articles, err := copied.Articles(feeds[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(articles) != 3 || articles[0].Read || !articles[2].Read {
		t.Fatalf("articles = %+v, want the seeded articles with their read state", articles)
	}
}

func TestExportRefusesToOverwrite(t *testing.T) {
	s := openTest(t)
	path := filepath.Join(t.TempDir(), "export.db")
	if err := os.WriteFile(path, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.Export(path); err == nil {
		t.Fatal("export should refuse an existing file")
	}
	if data, _ := os.ReadFile(path); string(data) != "keep me" {
		t.Fatal("the existing file must be left alone")
	}
}

func TestImportReplacesDatabaseAndKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "export.db")
	dst := filepath.Join(dir, "data", "feeds.db")

	other, err := OpenPath(filepath.Join(dir, "other.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := other.SeedMock(); err != nil {
		t.Fatal(err)
	}
	if err := other.Export(src); err != nil {
		t.Fatal(err)
	}
	other.Close()

	local, err := OpenPath(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.AddFeed(Feed{URL: "https://example.com/local.xml"}); err != nil {
		t.Fatal(err)
	}
	local.Close()

	backup, err := Import(src, dst)
	if err != nil {
		t.Fatal(err)
	}

	imported, err := OpenPath(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	if feeds, _ := imported.Feeds(); len(feeds) != 4 {
		t.Fatalf("got %d feeds after import, want the 4 exported ones", len(feeds))
	}

	if backup == "" || !strings.HasPrefix(filepath.Base(backup), "feeds.db.bak-") {
		t.Fatalf("backup = %q, want feeds.db.bak-<time>", backup)
	}
	old, err := OpenPath(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if feeds, _ := old.Feeds(); len(feeds) != 1 || feeds[0].URL != "https://example.com/local.xml" {
		t.Fatalf("backup feeds = %+v, want the previous local feed", feeds)
	}
}

func TestImportIntoEmptyDirectoryNeedsNoBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "export.db")
	s := openTest(t)
	if err := s.Export(src); err != nil {
		t.Fatal(err)
	}

	backup, err := Import(src, filepath.Join(dir, "new", "feeds.db"))
	if err != nil || backup != "" {
		t.Fatalf("backup = %q, err = %v, want no backup and no error", backup, err)
	}
}

func TestImportRejectsWhatIsNotAFeedhatchExport(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "feeds.db")

	text := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(text, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := filepath.Join(dir, "newer.db")
	s, err := OpenPath(newer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	for _, src := range []string{text, newer, filepath.Join(dir, "missing.db")} {
		if _, err := Import(src, dst); err == nil {
			t.Fatalf("importing %s should fail", filepath.Base(src))
		}
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("a rejected import must not create the database")
	}
}

func TestImportKeepsOnlyTheNewestBackups(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "export.db")
	dst := filepath.Join(dir, "data", "feeds.db")
	s := openTest(t)
	if err := s.Export(src); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []string{"20260101-000000", "20260102-000000", "20260103-000000", "20260104-000000"} {
		if err := os.WriteFile(dst+".bak-"+stamp, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	backup, err := Import(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := filepath.Glob(dst + ".bak-*")
	if len(left) != keepBackups {
		t.Fatalf("backups = %v, want the %d newest", left, keepBackups)
	}
	for _, name := range left {
		if strings.Contains(name, "20260101") || strings.Contains(name, "20260102") {
			t.Fatalf("old backup %s should be deleted", name)
		}
	}
	if !slices.Contains(left, backup) {
		t.Fatal("the backup just made must be kept")
	}
}
