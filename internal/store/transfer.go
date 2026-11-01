package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Export writes a consistent copy of the database to path, also while other
// connections use it. It refuses to overwrite an existing file.
func (s *Store) Export(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	_, err := s.db.Exec("VACUUM INTO ?", path)
	return err
}

// Import replaces the database at dst with a copy of the exported database src
// and upgrades it to the current schema. An existing database is first saved
// next to it, and its path is returned as backup. Nothing may use dst meanwhile.
func Import(src, dst string) (backup string, err error) {
	source, err := openExport(src)
	if err != nil {
		return "", err
	}
	defer source.Close()

	if _, err := os.Stat(dst); err == nil {
		backup = dst + ".bak-" + time.Now().Format("20060102-150405")
		current, err := OpenPath(dst)
		if err != nil {
			return "", err
		}
		err = current.Export(backup)
		current.Close()
		if err != nil {
			return "", fmt.Errorf("back up %s: %w", dst, err)
		}
		if err := pruneBackups(dst); err != nil {
			return backup, err
		}
		for _, p := range []string{dst, dst + "-wal", dst + "-shm"} {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				return backup, err
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return backup, err
	}
	if _, err := source.Exec("VACUUM INTO ?", dst); err != nil {
		return backup, fmt.Errorf("copy %s: %w", src, err)
	}

	imported, err := OpenPath(dst)
	if err != nil {
		return backup, err
	}
	return backup, imported.Close()
}

// openExport opens src read-only after checking that it is an intact feedhatch
// database that this version can read.
func openExport(src string) (*sql.DB, error) {
	if _, err := os.Stat(src); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+src+"?mode=ro")
	if err != nil {
		return nil, err
	}

	if err := checkExport(db, src); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func checkExport(db *sql.DB, src string) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("%s is not a feedhatch database: %w", src, err)
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	if version > len(files) {
		return fmt.Errorf("%s was made by a newer feedhatch (schema %d, this version knows %d)", src, version, len(files))
	}

	var tables int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('feeds', 'articles')").Scan(&tables)
	if err != nil || version == 0 || tables != 2 {
		return fmt.Errorf("%s is not a feedhatch database", src)
	}

	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
		return fmt.Errorf("%s is damaged: %s %v", src, check, err)
	}
	return nil
}

// keepBackups is how many import backups are kept next to the database.
const keepBackups = 3

// pruneBackups deletes all but the newest import backups of dst. Their names end in
// a timestamp, so sorting by name sorts them by age.
func pruneBackups(dst string) error {
	backups, err := filepath.Glob(dst + ".bak-*")
	if err != nil {
		return err
	}
	sort.Strings(backups)
	for _, old := range backups[:max(0, len(backups)-keepBackups)] {
		if err := os.Remove(old); err != nil {
			return err
		}
	}
	return nil
}
