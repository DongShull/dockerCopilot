package utiles

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupKeepCountDefault(t *testing.T) {
	t.Setenv("BACKUP_KEEP", "")
	if got := BackupKeepCount(); got != 7 {
		t.Fatalf("default keep = %d, want 7", got)
	}
}

func TestBackupKeepCountFromEnv(t *testing.T) {
	t.Setenv("BACKUP_KEEP", "3")
	if got := BackupKeepCount(); got != 3 {
		t.Fatalf("keep = %d, want 3", got)
	}
}

func TestBackupKeepCountInvalidFallsBack(t *testing.T) {
	t.Setenv("BACKUP_KEEP", "abc")
	if got := BackupKeepCount(); got != 7 {
		t.Fatalf("invalid keep should fall back to 7, got %d", got)
	}
	t.Setenv("BACKUP_KEEP", "-2")
	if got := BackupKeepCount(); got != 7 {
		t.Fatalf("negative keep should fall back to 7, got %d", got)
	}
}

func TestPruneOldBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	names := []string{"backup-1.json", "backup-2.json", "backup-3.json", "backup-4.json", "backup-5.json"}
	for i, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Now().Add(-time.Duration(len(names)-i) * time.Hour)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	// 隐藏文件（临时文件）不参与清理。
	if err := os.WriteFile(filepath.Join(dir, ".backup-x.json.tmp"), []byte("tmp"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed := pruneOldBackups(dir, "backup-*.json", 3)
	if len(removed) != 2 {
		t.Fatalf("expected 2 removed, got %v", removed)
	}
	// 最旧的两个（backup-1/2）应被删除。
	for _, gone := range []string{"backup-1.json", "backup-2.json"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed", gone)
		}
	}
	for _, kept := range []string{"backup-3.json", "backup-4.json", "backup-5.json"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("%s should be kept: %v", kept, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".backup-x.json.tmp")); err != nil {
		t.Fatal("hidden temp file should not be touched")
	}
}

func TestPruneOldBackupsNoopWhenUnderKeep(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"backup-a.json", "backup-b.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if removed := pruneOldBackups(dir, "backup-*.json", 7); removed != nil {
		t.Fatalf("expected no removal, got %v", removed)
	}
}
