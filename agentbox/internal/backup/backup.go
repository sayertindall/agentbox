package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"devbox/agentbox/internal/store"

	_ "modernc.org/sqlite"
)

type UnitObserver func(unit string) bool

type Service struct {
	db  *store.DB
	dir string
}

func New(db *store.DB, dir string) *Service {
	return &Service{db: db, dir: dir}
}

func (s *Service) Create() (string, error) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".backup-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary backup: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := s.db.BackupTo(context.Background(), tmpName); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("sqlite backup: %w", err)
	}
	if err := verify(tmpName); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	if err := fsyncFile(tmpName); err != nil {
		os.Remove(tmpName)
		return "", err
	}
	final := filepath.Join(s.dir, "control-"+time.Now().UTC().Format("20060102T150405.000000000Z")+".db")
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("install backup: %w", err)
	}
	if err := fsyncDir(s.dir); err != nil {
		return "", err
	}
	if err := s.db.RecordBackup(final); err != nil {
		return "", err
	}
	return final, nil
}

func (s *Service) Prune(retention time.Duration) error {
	backups, err := s.list()
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return nil
	}
	newest := backups[len(backups)-1]
	cutoff := time.Now().Add(-retention)
	for _, path := range backups[:len(backups)-1] {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if path == newest {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove expired backup %s: %w", path, err)
		}
	}
	return nil
}

func (s *Service) Drill(obs UnitObserver) error {
	backups, err := s.list()
	if err != nil {
		return err
	}
	if len(backups) == 0 {
		return fmt.Errorf("no verified backup")
	}
	return s.reconcileCopy(backups[len(backups)-1], obs)
}

func (s *Service) RestoreControl(path string, obs UnitObserver) error {
	if err := s.db.SetWritersBlocked(true); err != nil {
		return err
	}
	if err := s.reconcileCopy(path, obs); err != nil {
		return err
	}
	return fmt.Errorf("restore-control replacement is not implemented until live unit reconciliation is available")
}

func (s *Service) reconcileCopy(path string, obs UnitObserver) error {
	isolatedDir, err := os.MkdirTemp(s.dir, "restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(isolatedDir)
	isolated := filepath.Join(isolatedDir, "control.db")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(isolated, data, 0o600); err != nil {
		return err
	}
	if err := verify(isolated); err != nil {
		return err
	}
	db, err := store.Open(isolated)
	if err != nil {
		return err
	}
	defer db.Close()
	units, err := db.ActiveUnits()
	if err != nil {
		return err
	}
	for _, unit := range units {
		if obs == nil || !obs(unit) {
			return fmt.Errorf("reconciliation failed for unit %s", unit)
		}
	}
	return nil
}

func (s *Service) list() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type item struct {
		path string
		mod  time.Time
	}
	var items []item
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "control-") || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		path := filepath.Join(s.dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		items = append(items, item{path: path, mod: info.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.Before(items[j].mod) })
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.path
	}
	return out, nil
}

func verify(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("backup integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup integrity check: %s", result)
	}
	return nil
}

func fsyncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}
	return nil
}

func fsyncDir(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync backup directory: %w", err)
	}
	return nil
}
