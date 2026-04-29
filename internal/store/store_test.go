package store

import (
	"path/filepath"
	"testing"
)

func TestOpen_CreatesTables(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(db)
	if err != nil { t.Fatal(err) }
	defer s.Close()

	row := s.DB().QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='jobs'")
	var n int
	if err := row.Scan(&n); err != nil { t.Fatal(err) }
	if n != 1 { t.Errorf("jobs table count = %d, want 1", n) }
}

func TestOpen_IsIdempotent(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(db)
	if err != nil { t.Fatal(err) }
	s.Close()
	s2, err := Open(db)
	if err != nil { t.Fatal(err) }
	s2.Close()
}

func TestOpen_WALEnabled(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(db)
	if err != nil { t.Fatal(err) }
	defer s.Close()
	var mode string
	if err := s.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil { t.Fatal(err) }
	if mode != "wal" { t.Errorf("journal_mode = %q, want wal", mode) }
}
