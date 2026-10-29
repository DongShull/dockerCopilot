package utiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings 运行时可修改的设置（持久化到 /data/config/settings.json）。
type Settings struct {
	BackupDir string `json:"backupDir"`
}

var settingsMu sync.Mutex

func settingsPath() string {
	if p := os.Getenv("SETTINGS_FILE"); p != "" {
		return p
	}
	return "/data/config/settings.json"
}

// LoadSettings 读取设置文件；文件不存在或损坏时返回空设置。
func LoadSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	var s Settings
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return s
	}
	if err = json.Unmarshal(data, &s); err != nil {
		return Settings{}
	}
	return s
}

// SaveSettings 写入设置文件。
func SaveSettings(s Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	path := settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EffectiveBackupDir 返回实际生效的备份目录：
// 环境变量 BACKUP_DIR > Settings 中的 backupDir > 默认 /data/backups。
func EffectiveBackupDir() string {
	if dir := os.Getenv("BACKUP_DIR"); dir != "" {
		return dir
	}
	if s := LoadSettings(); s.BackupDir != "" {
		return s.BackupDir
	}
	return "/data/backups"
}
