package utiles

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zeromicro/go-zero/core/logx"
)

// backupKeepDefault 每种备份（JSON / compose）默认保留份数。
const backupKeepDefault = 7

// BackupKeepCount 备份保留份数：env BACKUP_KEEP，默认 7，最小 1。
// 每次备份生成带时间戳的新文件，超出的旧备份由 pruneOldBackups 清理。
func BackupKeepCount() int {
	if v := os.Getenv("BACKUP_KEEP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		logx.Errorf("BACKUP_KEEP 非法: %q，使用默认值 %d", v, backupKeepDefault)
	}
	return backupKeepDefault
}

// pruneOldBackups 按修改时间保留最近 keep 个匹配 pattern 的备份文件。
// 隐藏文件（含备份临时文件）不参与清理。返回被删除的文件名；
// 删除失败只记日志，不影响本次备份结果。
func pruneOldBackups(dir, pattern string, keep int) []string {
	if keep <= 0 {
		keep = 1
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		logx.Errorf("清理旧备份失败，读取目录出错: %v", err)
		return nil
	}
	type backupFile struct {
		name    string
		modTime int64
	}
	var files []backupFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		matched, matchErr := filepath.Match(pattern, name)
		if matchErr != nil || !matched {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		files = append(files, backupFile{name: name, modTime: info.ModTime().UnixNano()})
	}
	if len(files) <= keep {
		return nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime > files[j].modTime })
	var removed []string
	for _, f := range files[keep:] {
		if rmErr := os.Remove(filepath.Join(dir, f.name)); rmErr != nil {
			logx.Errorf("删除旧备份 %s 失败: %v", f.name, rmErr)
			continue
		}
		removed = append(removed, f.name)
	}
	return removed
}
