package store

import (
	"alekohs/feedhatch/internal/constants"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db *sql.DB
}

func OpenMock() (*Store, error) {
	dir := filepath.Join(os.TempDir(), "feedhatch-mock")
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	db, err := OpenPath(filepath.Join(dir, constants.FeedDb))
	if err != nil {
		return nil, err
	}

	if err := db.SeedMock(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil

}

// Dir is the cache directory of the feedhatch called name, e.g. ~/.cache/feedhatch.
func Dir(name string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// DBPath is the database of the feedhatch called name, e.g. ~/.local/state/feedhatch/feeds.db.
func DBPath(name string) (string, error) {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, name, constants.FeedDb), nil
}

// Open opens the database of the feedhatch called name.
func Open(name string) (*Store, error) {
	path, err := DBPath(name)
	if err != nil {
		return nil, err
	}
	return OpenPath(path)
}

// MoveFromCache copies a database that older versions kept in the cache directory to
// DBPath, unless one is already there. The old file is left in place: a process still
// running the old version has it open.
func MoveFromCache(name string) error {
	path, err := DBPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir, err := Dir(name)
	if err != nil {
		return err
	}
	old := filepath.Join(dir, constants.FeedDb)
	if _, err := os.Stat(old); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	s, err := OpenPath(old)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Copied under another name, so an interrupted copy is never taken for the database.
	tmp := path + ".tmp"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := s.Export(tmp); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CreateWithExamples creates the database of the feedhatch called name with example
// feeds, so a first start has something to read; an existing database is left alone.
func CreateWithExamples(name string) error {
	if err := MoveFromCache(name); err != nil {
		return err
	}
	path, err := DBPath(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s, err := OpenPath(path)
	if err != nil {
		return err
	}
	defer s.Close()
	for _, f := range examples {
		if _, err := s.AddFeed(f); err != nil {
			return err
		}
	}
	return nil
}

// examples are the feeds a new database starts with, so there is something to read.
var examples = []Feed{
	{URL: "https://github.com/alekohs/feedhatch/releases.atom", Name: "feedhatch releases", Category: "feedhatch", Interval: 24 * 60},
	{URL: "https://go.dev/blog/feed.atom", Name: "The Go Blog", Category: "Utveckling", Interval: 6 * 60},
}

func OpenPath(path string) (*Store, error) {
	dsn, err := getFilepath(path)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}

	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	for i, f := range files[version:] {
		script, err := migrations.ReadFile("migrations/" + f.Name())
		if err != nil {
			return err
		}

		tx, err := db.Begin()
		if err != nil {
			return err
		}

		if _, err := tx.Exec(string(script)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", f.Name(), err)
		}

		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version+i+1)); err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func getFilepath(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	dsn := path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	return dsn, nil
}
