package utiles

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/zeromicro/go-zero/core/logx"
)

// ErrFnosManagedUpdate 表示 FNOS 版本的版本检查与自更新由专用发布包管理，
// 不走通用在线更新链。调用方应显式向用户说明，而不是伪装成“无更新”。
var ErrFnosManagedUpdate = errors.New("FnOS 版本由专用发布包管理，不支持在线版本检查与自更新")

func GetRemoteVersion() (remoteVersion string, err error) {
	localVersion := config.Version
	if strings.Contains(localVersion, "FNOS") {
		// FNOS 版本使用独立发布包，不需要也不应该访问通用版 version 文件。
		logx.Infof("飞牛版本，不支持在线版本检查")
		return "", ErrFnosManagedUpdate
	}

	githubProxy := os.Getenv("githubProxy")
	if githubProxy != "" {
		githubProxy = strings.TrimRight(githubProxy, "/") + "/"
	}
	versionURL := githubProxy + "https://raw.githubusercontent.com/" + UpdateRepo() + "/" + UpdateBranch() + "/version"
	remoteVersion, err = fetchVersionFromURL(versionURL)
	if err != nil {
		return "0.0.0", err
	}
	if localVersion == remoteVersion {
		logx.Info("版本一致:", localVersion)
		return remoteVersion, nil
	} else {
		logx.Infof("版本不一致! 本地: %s, 远程: %s\n", localVersion, remoteVersion)
		return remoteVersion, nil
	}

}

func fetchVersionFromURL(url string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("关闭Body失败:", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取版本信息失败：%s", resp.Status)
	}

	versionData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(versionData)), nil
}
