package utiles

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var backupFilenamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func BackupDir() string {
	return EffectiveBackupDir()
}

func ResolveBackupPath(filename string, allowedExts ...string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "", fmt.Errorf("非法文件名")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("非法文件名")
	}
	if name != filepath.Base(name) {
		return "", fmt.Errorf("非法文件名")
	}
	if strings.ContainsAny(name, `/\`) || !backupFilenamePattern.MatchString(name) {
		return "", fmt.Errorf("非法文件名")
	}

	if len(allowedExts) > 0 {
		ext := strings.ToLower(filepath.Ext(name))
		allowed := false
		for _, candidate := range allowedExts {
			if ext == strings.ToLower(candidate) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("非法文件名")
		}
	}

	base, err := filepath.Abs(filepath.Clean(BackupDir()))
	if err != nil {
		return "", fmt.Errorf("备份目录不合法")
	}
	fullPath := filepath.Join(base, name)
	rel, err := filepath.Rel(base, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("非法文件名")
	}

	return fullPath, nil
}
