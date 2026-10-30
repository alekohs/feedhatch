package store

import (
	"database/sql"
	"embed"
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
	db, err := OpenPath(filepath.Join(dir, "feeds.db"))
	if err != nil {
		return nil, err
	}

	if err := db.SeedMock(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil

}

func Open() (*Store, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return OpenPath(filepath.Join(dir, "feedhatch", "feeds.db"))
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
