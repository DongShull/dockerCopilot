package icons

import (
	"fmt"
	"net/http"
	"os"
	"regexp"

	"github.com/onlyLTY/dockerCopilot/internal/svc"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ObtainHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsPath := "/data/config/imageLogos.js"
		logx.Infof("读取图标配置: %s", jsPath)

		contentBytes, err := os.ReadFile(jsPath)
		if err != nil {
			if os.IsNotExist(err) {
				logx.Info("图标配置文件不存在，返回空")
				httpx.OkJsonCtx(r.Context(), w, types.Resp{
					Code: 200,
					Msg:  "Success",
					Data: "{}",
				})
				return
			}
			logx.Errorf("读取图标配置失败: %v", err)
			httpx.ErrorCtx(r.Context(), w, fmt.Errorf("读取图标配置失败: %v", err))
			return
		}

		content := string(contentBytes)
		// fmt.Printf("Config content: %s\n", content) // Uncomment for deep debug

		// 改进的正则表达式：匹配 "key": "value"，允许一定的格式变化
		// 使用反引号表示原始字符串。
		re := regexp.MustCompile(`"([^"]+)"\s*:\s*"([^"]+)"`)
		matches := re.FindAllStringSubmatch(content, -1)

		icons := make(map[string]string)
		for _, match := range matches {
			if len(match) == 3 {
				key := match[1]
				val := match[2]
				icons[key] = val
				// fmt.Printf("Found icon: %s -> %s\n", key, val)
			}
		}

		logx.Infof("共解析出 %d 个图标", len(icons))

		response := struct {
			Code int               `json:"code"`
			Msg  string            `json:"msg"`
			Data map[string]string `json:"data"`
		}{
			Code: 200,
			Msg:  "Success",
			Data: icons,
		}

		httpx.OkJsonCtx(r.Context(), w, response)
	}

}
