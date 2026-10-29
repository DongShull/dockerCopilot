package module

import (
	"crypto/tls"
	"errors"
	"fmt"
	ref "github.com/distribution/reference"
	"github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
	"net"
	"net/http"
	url2 "net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ImageCheckList 检查更新处理后的镜像列表
type ImageCheckList struct {
	NeedUpdate bool
}
type ImageUpdateData struct {
	mu   sync.RWMutex
	Data map[string]ImageCheckList
}

// Get 返回指定镜像的更新检查结果（并发安全）。
func (i *ImageUpdateData) Get(imageID string) (ImageCheckList, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	v, ok := i.Data[imageID]
	return v, ok
}

const ContentDigestHeader = "Docker-Content-Digest"

const checkUpdateConcurrency = 4

func NewImageCheck() *ImageUpdateData {
	return &ImageUpdateData{
		Data: map[string]ImageCheckList{},
	}
}
func (i *ImageUpdateData) CheckUpdate(imageList []types.Image) {
	results := make(map[string]ImageCheckList)
	var resultsMu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, checkUpdateConcurrency)
	for _, image := range imageList {
		if strings.Contains(image.ImageName, "0nlylty/dockercopilot") {
			continue
		}
		wg.Add(1)
		go func(img types.Image) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if result, ok := i.checkSingleImage(img); ok {
				resultsMu.Lock()
				results[img.ID] = result
				resultsMu.Unlock()
			}
		}(image)
	}
	wg.Wait()
	i.mu.Lock()
	i.Data = results
	i.mu.Unlock()
}

func (i *ImageUpdateData) checkSingleImage(image types.Image) (ImageCheckList, bool) {
	token, err := GetToken(image, "")
	if err != nil {
		logx.Error("获取token失败或者无需获取token，继续尝试检查" + err.Error())
	}
	digestURL, err := BuildManifestURL(image)
	if err != nil {
		logx.Error("获取digestURL失败" + err.Error())
		return ImageCheckList{}, false
	}
	remoteDigest, err := GetDigest(digestURL, token)
	if err != nil {
		logx.Error("获取digest失败" + err.Error())
		return ImageCheckList{}, false
	}
	if len(image.RepoDigests) == 0 {
		logx.Error("未在本地获取到repoDigest" + image.ImageName + ":" + image.ImageTag)
		return ImageCheckList{}, false
	}
	needUpdate := false
	for _, localRepoDigests := range image.RepoDigests {
		parts := strings.Split(localRepoDigests, "@")
		if len(parts) != 2 {
			continue
		}
		localDigest := parts[1]
		if remoteDigest == "" || localDigest == "" {
			logx.Error("Digest为空" + image.ImageName + ":" + image.ImageTag)
			continue
		}
		if remoteDigest != localDigest {
			// 任一本地 digest 与远端不同即判定需要更新，不能因后续匹配被覆盖
			logx.Info(image.ImageName + ":" + image.ImageTag + " need update")
			logx.Infof("localDigest: %s, remoteDigest: %s", localDigest, remoteDigest)
			needUpdate = true
		} else {
			logx.Info(image.ImageName + ":" + image.ImageTag + " not need update")
		}
	}
	return ImageCheckList{NeedUpdate: needUpdate}, true
}

func BuildManifestURL(image types.Image) (string, error) {
	normalizedRef, err := ref.ParseDockerRef(image.ImageName + ":" + image.ImageTag)
	if err != nil {
		return "", err
	}
	normalizedTaggedRef, isTagged := normalizedRef.(ref.NamedTagged)
	if !isTagged {
		return "", errors.New("镜像无tag" + normalizedRef.String())
	}

	host, ErrGetRegistryAddress := GetRegistryAddress(normalizedTaggedRef.Name())
	img, tag := ref.Path(normalizedTaggedRef), normalizedTaggedRef.Tag()

	if ErrGetRegistryAddress != nil {
		return "", ErrGetRegistryAddress
	}

	url := url2.URL{
		Scheme: "https",
		Host:   host,
		Path:   fmt.Sprintf("/v2/%s/manifests/%s", img, tag),
	}
	return url.String(), nil
}

func GetDigest(url string, token string) (string, error) {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: os.Getenv("REGISTRY_INSECURE") != "false"},
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest("HEAD", url, nil)

	if token != "" {
		req.Header.Add("Authorization", token)
	}
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v1+json")
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("GetDigest关闭body失败" + err.Error())
		}
	}(res.Body)

	if res.StatusCode != 200 {
		wwwAuthHeader := res.Header.Get("www-authenticate")
		if wwwAuthHeader == "" {
			wwwAuthHeader = "not present"
		}
		return "", fmt.Errorf("registry responded to head request with %q, auth: %q", res.Status, wwwAuthHeader)
	}
	return res.Header.Get(ContentDigestHeader), nil
}
