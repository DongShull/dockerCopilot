package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/handler"
	"github.com/onlyLTY/dockerCopilot/internal/mcp"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/onlyLTY/dockerCopilot/internal/utiles"
	"github.com/robfig/cron/v3"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/x/errors"
	xhttp "github.com/zeromicro/x/http"
)

var configFile = flag.String("f", "etc/dockerCopilot.yaml", "the config file")

type UnauthorizedResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data map[string]interface{} `json:"data"`
}

func main() {
	logDir := "./logs"
	ErrSetupLog := SetupLog(logDir)
	if ErrSetupLog != nil {
		logx.Errorf("初始化日志失败: %v", ErrSetupLog)
		os.Exit(1)
	}
	logx.SetLevel(logx.InfoLevel)

	flag.Parse()
	var c config.Config
	err := conf.Load(*configFile, &c, conf.UseEnv())
	if err != nil {
		logx.Errorf("无法加载配置文件出错: %v", err)
		logx.Errorf("请确认secretKey设置正确，要求非纯数字且大于八位")
		os.Exit(1)
	}
	if err := validateSecretKey(c.Auth.AccessSecret); err != nil {
		logx.Errorf("secretKey 配置不合法: %v", err)
		os.Exit(1)
	}
	server := rest.MustNewServer(c.RestConf, rest.WithCors("*"), rest.WithUnauthorizedCallback(
		func(w http.ResponseWriter, r *http.Request, err error) {
			response := UnauthorizedResponse{
				Code: http.StatusUnauthorized, // 401
				Msg:  "未授权",
				Data: map[string]interface{}{},
			}
			httpx.WriteJson(w, http.StatusUnauthorized, response)
		}))
	defer server.Stop()
	ctx := svc.NewServiceContext(c)
	if ctx.DockerInitErr != nil {
		logx.Errorf("Docker 客户端初始化失败: %v", ctx.DockerInitErr)
		logx.Errorf("请检查 DOCKER_HOST、DOCKER_TLS_VERIFY、DOCKER_CERT_PATH 等环境变量")
		os.Exit(1)
	}

	// Ensure data directory and config exist (Auto-init)
	dataDir := "/data/config/image"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		logx.Errorf("创建数据目录失败: %v", err)
	}

	imageLogosPath := "/data/config/imageLogos.js"
	if _, err := os.Stat(imageLogosPath); os.IsNotExist(err) {
		defaultConfig := []byte(`// 自定义镜像logo配置
export const customImageLogos = {
};
`)
		if err := os.WriteFile(imageLogosPath, defaultConfig, 0644); err != nil {
			logx.Errorf("创建默认 imageLogos.js 失败: %v", err)
		}
	}

	list, err := utiles.GetImagesList(ctx)
	if err != nil {
		// Docker 暂不可用时不要杀死进程，交由定时任务稍后重试
		logx.Errorf("启动时获取镜像列表失败（稍后自动重试）: %v", err)
	} else {
		go ctx.HubImageInfo.CheckUpdate(list)
	}
	corndanmu := cron.New(cron.WithParser(cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
	)))
	_, err = corndanmu.AddFunc("30 * * * *", func() {
		defer func() {
			if r := recover(); r != nil {
				logx.Errorf("镜像更新检查 panic: %v", r)
			}
		}()
		list, listErr := utiles.GetImagesList(ctx)
		if listErr != nil {
			logx.Errorf("获取镜像列表出错: %v", listErr)
			return
		}
		ctx.HubImageInfo.CheckUpdate(list)
	})
	if err != nil {
		logx.Errorf("panic添加定时任务出错: %v", err)
		panic(err)
	}
	// 定时自动更新：AUTOUPDATE_CRON 为 cron 表达式，AUTOUPDATE_WHITELIST 为容器白名单
	if cronExpr := os.Getenv("AUTOUPDATE_CRON"); cronExpr != "" {
		_, err = corndanmu.AddFunc(cronExpr, func() {
			defer func() {
				if r := recover(); r != nil {
					logx.Errorf("自动更新任务 panic: %v", r)
				}
			}()
			utiles.RunAutoUpdate(ctx)
		})
		if err != nil {
			logx.Errorf("自动更新定时任务表达式非法: %v", err)
		} else if !utiles.AutoUpdateConfigured() {
			logx.Errorf("自动更新未完全启用：AUTOUPDATE_WHITELIST 为空，任务将跳过")
		} else {
			logx.Infof("自动更新已启用: %s, 白名单: %s", cronExpr, os.Getenv("AUTOUPDATE_WHITELIST"))
		}
	}
	corndanmu.Start()
	defer corndanmu.Stop()
	httpx.SetErrorHandler(func(err error) (int, any) {
		switch e := err.(type) {
		case *errors.CodeMsg:
			return http.StatusOK, xhttp.BaseResponse[types.Nil]{
				Code: e.Code,
				Msg:  e.Msg,
			}
		default:
			return http.StatusOK, xhttp.BaseResponse[types.Nil]{
				Code: 50000,
				Msg:  err.Error(),
			}
		}
	})
	handler.RegisterHandlers(server, ctx)
	mcp.Register(server, ctx)
	fmt.Printf("服务已启动: %s:%d...\n", c.Host, c.Port)
	logx.Info("程序版本" + config.Version)
	server.Start()
}

// validateSecretKey 校验 secretKey：去掉首尾空白后长度需大于 8 位，且不能是纯数字。
func validateSecretKey(secret string) error {
	secret = strings.TrimSpace(secret)
	// 变量未展开（如 ${secretKey}）时长度可能“看起来”合规，必须明确拒绝。
	if strings.Contains(secret, "${") || strings.Contains(secret, "$") {
		return fmt.Errorf("secretKey 未展开，请直接填写真实值")
	}
	if utf8.RuneCountInString(secret) <= 8 {
		return fmt.Errorf("长度必须大于 8 位")
	}
	allDigits := true
	for _, r := range secret {
		if !unicode.IsDigit(r) {
			allDigits = false
			break
		}
	}
	if allDigits {
		return fmt.Errorf("不能是纯数字")
	}
	return nil
}

// ensureLogDirectory 检查并创建日志目录
func ensureLogDirectory(logDir string) error {
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		return os.MkdirAll(logDir, 0755) // 创建目录并设置权限
	}
	return nil
}

// SetupLog 初始化日志设置
func SetupLog(logDir string) error {
	// 检查日志目录是否存在
	if err := ensureLogDirectory(logDir); err != nil {
		return fmt.Errorf("failed to create log directory: %v", err)
	}

	logConf := logx.LogConf{
		Path:     logDir,
		Level:    "info",
		KeepDays: 7,
		Compress: true,
		Mode:     "file",
		// 使用纯文本格式，日志行只保留“时间 + 级别 + 内容”，
		// 不再输出 JSON 字段或服务名前缀。
		Encoding: "plain",
	}
	logx.MustSetup(logConf)
	logx.AddWriter(logx.NewWriter(os.Stdout))
	return nil
}
