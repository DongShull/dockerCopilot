package utiles

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// updateProgramMu 串行化自更新：并发两次调用会交错写同一个
// dockerCopilot.tar.gz / dockerCopilot-new，可能装上损坏或错版的包。
var updateProgramMu sync.Mutex

// ErrUpdateProgramRunning 已有一轮自更新在执行时返回。
var ErrUpdateProgramRunning = errors.New("已有一轮自更新在执行，请稍后再试")

func UpdateProgram(ctx *svc.ServiceContext) error {
	if !updateProgramMu.TryLock() {
		return ErrUpdateProgramRunning
	}
	defer updateProgramMu.Unlock()

	if strings.Contains(config.Version, "FNOS") {
		return fmt.Errorf("飞牛版本请使用 FNOS 专用发布包更新，禁止覆盖为通用版")
	}
	githubProxy := os.Getenv("githubProxy")
	if githubProxy != "" {
		githubProxy = strings.TrimRight(githubProxy, "/") + "/"
	}
	versionURL := githubProxy + "https://raw.githubusercontent.com/" + UpdateRepo() + "/" + UpdateBranch() + "/version"
	releaseBaseURL := githubProxy + "https://github.com/" + UpdateRepo() + "/releases/download"
	logx.Infof("版本检查地址: %s", versionURL)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(versionURL)
	if err != nil {
		logx.Info("没有获取到最新版本信息:", err)
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("关闭resp.Body失败:", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取版本信息失败: HTTP %s", resp.Status)
	}
	versionData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	logx.Infof("远端版本文件内容: %s", versionData)
	if err != nil {
		logx.Info("没有获取到最新版本信息:", err)
		return err
	}

	version := strings.TrimSpace(string(versionData))
	logx.Info("获取到最新版本：", version)
	// 2. 构造下载链接
	downloadURL := fmt.Sprintf("%s/%s/dockerCopilot-%s.tar.gz", releaseBaseURL, version, runtime.GOARCH)
	logx.Info("下载链接：", downloadURL)
	dest := "dockerCopilot.tar.gz"
	checksumDest := dest + ".sha256"
	defer func() {
		_ = os.Remove(dest)
		_ = os.Remove(checksumDest)
	}()

	if err := downloadFile(downloadURL, dest); err != nil {
		logx.Error("下载失败:", err)
		return err
	}
	if os.Getenv("UPDATE_SKIP_CHECKSUM") != "true" {
		if err := downloadFile(downloadURL+".sha256", checksumDest); err != nil {
			return fmt.Errorf("下载校验文件失败（如为旧版本可临时设置 UPDATE_SKIP_CHECKSUM=true）: %w", err)
		}
		checksumData, readErr := os.ReadFile(checksumDest)
		if readErr != nil {
			return readErr
		}
		fields := strings.Fields(string(checksumData))
		if len(fields) == 0 {
			return fmt.Errorf("校验文件为空")
		}
		if err := verifySHA256(dest, fields[0]); err != nil {
			return fmt.Errorf("安装包 SHA256 校验失败: %w", err)
		}
		logx.Info("SHA256 校验通过")
	}
	logx.Info("下载成功")

	// 先解压到临时目录，成功后再原子替换；避免解压失败留下半个 dockerCopilot-new
	// 被 start.sh 在下次启动时误当成新版本。
	tmpDir, err := os.MkdirTemp(".", ".dc-update-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()
	if err := decompressTarGz(dest, tmpDir); err != nil {
		logx.Info("解压缩失败:", err)
		return err
	}

	candidates := []string{
		filepath.Join(tmpDir, "dockerCopilot-new"),
		filepath.Join(tmpDir, "dist", "linux", runtime.GOARCH, "dockerCopilot-new"),
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			if err := os.Rename(candidate, "dockerCopilot-new"); err != nil {
				return err
			}
			logx.Info("解压缩成功")
			return nil
		}
	}
	return fmt.Errorf("解压完成但未找到 dockerCopilot-new")
}

const maxUpdateAssetSize = 512 << 20 // 512MB，足够覆盖当前发布包，同时防止异常响应写满磁盘

func downloadFile(url string, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("关闭resp.Body失败:", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 失败: HTTP %s", url, resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			logx.Error("关闭out失败:", err)
		}
	}(out)

	written, err := io.Copy(out, io.LimitReader(resp.Body, maxUpdateAssetSize+1))
	if err != nil {
		return err
	}
	if written > maxUpdateAssetSize {
		return fmt.Errorf("下载文件超过大小限制 %d 字节", maxUpdateAssetSize)
	}
	return nil
}

func verifySHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("期望 %s，实际 %s", expected, actual)
	}
	return nil
}

func decompressTarGz(gzFilePath string, dest string) error {
	file, err := os.Open(gzFilePath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logx.Error("关闭file失败:", err)
		}
	}(file)

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func(gzr *gzip.Reader) {
		err := gzr.Close()
		if err != nil {
			logx.Error("关闭gzr失败:", err)
		}
	}(gzr)

	tarReader := tar.NewReader(gzr)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, filepath.FromSlash(header.Name))
		// 防止 zip-slip：用绝对路径判断，避免 dest="." 时把正常文件全部误判。
		absDest, err := filepath.Abs(dest)
		if err != nil {
			return err
		}
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(absDest, absTarget)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("非法的文件路径: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				err := outFile.Close()
				if err != nil {
					return err
				}
				return err
			}
			err = outFile.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("未知类型: %v in %s", header.Typeflag, header.Name)
		}
	}

	return nil
}
