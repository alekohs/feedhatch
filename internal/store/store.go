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

// Dir is the data directory of the feedhatch called name, e.g. ~/.cache/feedhatch.
func Dir(name string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// Open opens feeds.db in the data directory of the feedhatch called name.
func Open(name string) (*Store, error) {
	dir, err := Dir(name)
	if err != nil {
		return nil, err
	}
	return OpenPath(filepath.Join(dir, constants.FeedDb))
}

// CreateWithExamples creates the database of the feedhatch called name with example
// feeds, so a first start has something to read; an existing database is left alone.
func CreateWithExamples(name string) error {
	dir, err := Dir(name)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, constants.FeedDb)
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
