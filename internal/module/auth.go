package module

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	ref "github.com/distribution/reference"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

const ChallengeHeader = "WWW-Authenticate"
const (
	DefaultRegistryDomain = "docker.io"
	DefaultRegistryHost   = "registry-1.docker.io"
)

var registryHTTPClient = newRegistryHTTPClient()

func newRegistryHTTPClient() *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		transport = &http.Transport{}
	} else {
		transport = transport.Clone()
	}
	if os.Getenv("REGISTRY_INSECURE") == "true" {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- 由用户显式开启
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}
}

// host 探测结果缓存，避免每个镜像都重新跑一遍 10 个加速器。
type hostCheckResult struct {
	ok bool
	at time.Time
}

var hostCheckCache sync.Map

func checkHostCached(host string) bool {
	if v, ok := hostCheckCache.Load(host); ok {
		r := v.(hostCheckResult)
		if time.Since(r.at) < 10*time.Minute {
			return r.ok
		}
	}
	ok := checkHost(host)
	hostCheckCache.Store(host, hostCheckResult{ok: ok, at: time.Now()})
	return ok
}

var DefaultAcceleratorHostList = []string{"docker.1ms.run", "docker.m.daocloud.io",
	"docker.1panel.top", "docker.1panel.live", "proxy.1panel.live", "dockerproxy.1panel.live", "docker.1panel.dev",
	"docker.anye.in", "hub.rat.dev", "docker.amingg.com"}

func GetToken(image types.Image, registryAuth string) (string, error) {
	logx.Infof("镜像名称: %s", image.ImageName)
	normalizedRef, err := ref.ParseNormalizedNamed(image.ImageName)
	if err != nil {
		return "", err
	}

	URL := GetChallengeURL(normalizedRef)

	var req *http.Request
	if req, err = GetChallengeRequest(URL); err != nil {
		return "", err
	}

	var res *http.Response
	if res, err = registryHTTPClient.Do(req); err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("GetToken关闭Body失败" + err.Error())
		}
	}(res.Body)
	v := res.Header.Get(ChallengeHeader)

	challenge := strings.TrimSpace(v)
	lowerChallenge := strings.ToLower(challenge)
	if strings.HasPrefix(lowerChallenge, "basic") {
		if registryAuth == "" {
			return "", fmt.Errorf("无可用凭证")
		}

		return fmt.Sprintf("Basic %s", registryAuth), nil
	}
	if strings.HasPrefix(lowerChallenge, "bearer") {
		return GetBearerHeader(challenge, normalizedRef, registryAuth)
	}

	return "", errors.New("registry 返回了不支持的认证质询类型")
}

func GetChallengeRequest(URL url.URL) (*http.Request, error) {
	req, err := http.NewRequest("GET", URL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "dockerCopilot")
	return req, nil
}

func GetBearerHeader(challenge string, imageRef ref.Named, registryAuth string) (string, error) {
	authURL, err := GetAuthURL(challenge, imageRef)

	if err != nil {
		return "", err
	}

	var r *http.Request
	if r, err = http.NewRequest("GET", authURL.String(), nil); err != nil {
		return "", err
	}

	if registryAuth != "" {
		logx.Info("私有镜像，无法获取是否有更新")
		r.Header.Add("Authorization", fmt.Sprintf("Basic %s", registryAuth))
	} else {
		logx.Info("未找到凭证。")
	}

	var authResponse *http.Response
	if authResponse, err = registryHTTPClient.Do(r); err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		if closeErr := Body.Close(); closeErr != nil {
			logx.Errorf("GetBearerHeader关闭Body失败: %v", closeErr)
		}
	}(authResponse.Body)

	body, err := io.ReadAll(io.LimitReader(authResponse.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if authResponse.StatusCode < 200 || authResponse.StatusCode >= 300 {
		return "", fmt.Errorf("获取 registry token 失败: %s", authResponse.Status)
	}
	tokenResponse := &types.TokenResponse{}

	err = json.Unmarshal(body, tokenResponse)
	if err != nil {
		return "", err
	}
	if tokenResponse.Token == "" {
		return "", fmt.Errorf("registry 返回的 token 为空")
	}

	return fmt.Sprintf("Bearer %s", tokenResponse.Token), nil
}

func GetAuthURL(challenge string, imageRef ref.Named) (*url.URL, error) {
	raw := strings.TrimSpace(challenge)
	if idx := strings.IndexByte(raw, ' '); idx >= 0 {
		raw = raw[idx+1:]
	}

	pairs := strings.Split(raw, ",")
	values := make(map[string]string, len(pairs))

	for _, pair := range pairs {
		trimmed := strings.TrimSpace(pair)
		if key, val, ok := strings.Cut(trimmed, "="); ok {
			// challenge 参数名大小写不敏感，但 realm URL 的路径必须保留原始大小写。
			values[strings.ToLower(key)] = strings.Trim(val, `"`)
		}
	}
	if values["realm"] == "" || values["service"] == "" {

		return nil, fmt.Errorf("challenge 头缺少构造认证 URL 所需的字段")
	}

	authURL, err := url.Parse(values["realm"])
	if err != nil || authURL.Host == "" || (authURL.Scheme != "http" && authURL.Scheme != "https") {
		return nil, fmt.Errorf("registry 返回了非法的 realm: %q", values["realm"])
	}
	q := authURL.Query()
	q.Add("service", values["service"])

	scopeImage := ref.Path(imageRef)

	scope := fmt.Sprintf("repository:%s:pull", scopeImage)
	q.Add("scope", scope)

	authURL.RawQuery = q.Encode()
	return authURL, nil
}

func GetChallengeURL(imageRef ref.Named) url.URL {
	host, err := GetRegistryAddress(imageRef.Name())
	if err != nil || host == "" {
		host = DefaultRegistryHost
	}

	URL := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   "/v2/",
	}
	return URL
}

func GetRegistryAddress(imageRef string) (string, error) {
	normalizedRef, err := ref.ParseNormalizedNamed(imageRef)
	if err != nil {
		return "", err
	}

	address := ref.Domain(normalizedRef)

	if address == DefaultRegistryDomain {
		if checkHostCached(DefaultRegistryHost) {
			address = DefaultRegistryHost
		} else {
			for _, host := range DefaultAcceleratorHostList {
				if checkHostCached(host) {
					address = host
					break
				}
			}
		}
		if address == DefaultRegistryDomain {
			address = DefaultRegistryHost
		}
	}
	return address, nil
}

func checkHost(host string) bool {
	URL := "https://" + host + "/v2/"
	// 复用 registry 客户端 transport，确保 REGISTRY_INSECURE 对探测同样生效。
	client := http.Client{
		Transport: registryHTTPClient.Transport,
		Timeout:   5 * time.Second,
	}
	// 发送 HEAD 请求
	resp, err := client.Get(URL)
	if err != nil {
		logx.Errorf("连接 %s 失败: %s", URL, err)
		return false
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Errorf("关闭body失败: %v", err)
		}
	}(resp.Body)

	// 检查 HTTP 响应状态码
	if resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusUnauthorized {
		return true
	}

	logx.Errorf("连接 %s 失败: %s", URL, resp.Status)
	return false
}
