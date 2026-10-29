package utiles

import (
	"os"
	"strings"
)

// UpdateRepo 返回自更新与版本检查所使用的仓库（owner/repo）。
// 默认指向本项目的自维护 fork，可通过环境变量 UPDATE_REPO 覆盖。
func UpdateRepo() string {
	repo := strings.TrimSpace(os.Getenv("UPDATE_REPO"))
	if repo == "" {
		repo = "DongShull/dockerCopilot"
	}
	return repo
}

// UpdateBranch 返回版本检查读取的分支，默认 latest，可用 UPDATE_BRANCH 覆盖。
func UpdateBranch() string {
	branch := strings.TrimSpace(os.Getenv("UPDATE_BRANCH"))
	if branch == "" {
		branch = "latest"
	}
	return branch
}
