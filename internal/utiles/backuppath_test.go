package utiles

import (
	"path/filepath"
	"testing"
)

func TestResolveBackupPathRelativeBackupDir(t *testing.T) {
	t.Setenv("BACKUP_DIR", ".")
	t.Setenv("SETTINGS_FILE", filepath.Join(t.TempDir(), "settings.json"))

	got, err := ResolveBackupPath("backup-2026-01-01.json", ".json")
	if err != nil {
		t.Fatalf("ResolveBackupPath() error = %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("resolved path = %q, want absolute path", got)
	}
	if filepath.Base(got) != "backup-2026-01-01.json" {
		t.Fatalf("base = %q", filepath.Base(got))
	}
}

func TestResolveBackupPathRejectsTraversal(t *testing.T) {
	t.Setenv("BACKUP_DIR", t.TempDir())
	t.Setenv("SETTINGS_FILE", filepath.Join(t.TempDir(), "settings.json"))

	if _, err := ResolveBackupPath("../escape.json", ".json"); err == nil {
		t.Fatal("expected traversal error")
	}
}
