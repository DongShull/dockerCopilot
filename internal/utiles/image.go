package utiles

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/image"
	"github.com/onlyLTY/dockerCopilot/internal/svc"
	MyType "github.com/onlyLTY/dockerCopilot/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"strings"
)

func GetImagesList(ctx *svc.ServiceContext) ([]MyType.Image, error) {
	var imagesList []MyType.Image
	if ctx.DockerClient == nil {
		return nil, fmt.Errorf("Docker 客户端未初始化，请检查 DOCKER_HOST 配置")
	}
	requestCtx, cancel := dockerTimeout(context.Background(), dockerListTimeout)
	defer cancel()
	dockerImages, err := ctx.DockerClient.ImageList(requestCtx, image.ListOptions{})
	if err != nil {
		logx.Errorf("获取镜像列表失败: %s", err)
		return nil, err
	}

	for _, img := range dockerImages {
		i := MyType.Image{
			Summary:    img,
			ImageName:  "",
			ImageTag:   "",
			InUsed:     false,
			SizeFormat: "",
		}
		imagesList = append(imagesList, i)
	}
	//看不明白就不要看了，这内存反复地申请，如果你看明白了 给这改成指针吧，啥？我为啥不直接写指针，我懒癌犯了就这样，欢迎pr
	imagesList, err = checkImageInUsed(ctx, splitImageNameAndTag(calculateImageSize(imagesList)))
	if err != nil {
		return imagesList, err
	}
	return imagesList, nil
}

func splitImageNameAndTag(imagesList []MyType.Image) []MyType.Image {
	for i, imageInfo := range imagesList {
		if len(imageInfo.RepoTags) != 0 {
			repoTag := imageInfo.RepoTags[0]
			// 用最后一个冒号切分，兼容带端口的私有仓库（如 registry.example.com:5000/app:v1）
			if idx := strings.LastIndex(repoTag, ":"); idx > 0 && !strings.Contains(repoTag[idx+1:], "/") {
				imagesList[i].ImageName = repoTag[:idx]
				imagesList[i].ImageTag = repoTag[idx+1:]
			} else {
				imagesList[i].ImageName = repoTag
				imagesList[i].ImageTag = "None"
			}
		} else if len(imageInfo.RepoDigests) != 0 {
			imagesList[i].ImageName = strings.Split(imageInfo.RepoDigests[0], "@")[0]
			imagesList[i].ImageTag = "None"
		} else {
			imagesList[i].ImageName = "None"
			imagesList[i].ImageTag = "None"
		}
	}
	return imagesList
}
func checkImageInUsed(svc *svc.ServiceContext, imageList []MyType.Image) ([]MyType.Image, error) {
	list, err := GetContainerList(svc)
	if err != nil {
		return imageList, err
	}
	used := make(map[string]struct{}, len(list))
	for _, v := range list {
		used[v.ImageID] = struct{}{}
	}
	for i := range imageList {
		if _, ok := used[imageList[i].ID]; ok {
			imageList[i].InUsed = true
		}
	}
	return imageList, nil
}
func calculateImageSize(imagesList []MyType.Image) []MyType.Image {
	for i := range imagesList {
		if imagesList[i].Size >= 1024*1024*1024 {
			imagesList[i].SizeFormat = // Convert size to gigabytes
				fmt.Sprintf("%d Gb", imagesList[i].Size/1024/1024/1024)
		} else {
			imagesList[i].SizeFormat = // Convert size to megabytes
				fmt.Sprintf("%d Mb", imagesList[i].Size/1024/1024)
		}
	}
	return imagesList
}

// 批量镜像清理过滤器：删除无 TAG 镜像 / 删除未被容器引用的镜像。
const (
	CleanupFilterUntagged = "untagged"
	CleanupFilterUnused   = "unused"
)

// isUntaggedImage 判断镜像是否没有可用的 tag。
// splitImageNameAndTag 会把无 tag 镜像的 ImageTag 归一为 "None"，
// 少数异常镜像的 RepoTags 里是字面 "<none>:<none>"，这里一并覆盖。
func isUntaggedImage(img MyType.Image) bool {
	return img.ImageTag == "None" || img.ImageTag == "<none>" ||
		img.ImageName == "None" || img.ImageName == "<none>"
}

// PickImagesForCleanup 按过滤器挑选待清理镜像（纯函数，便于测试）。
// untagged：无 TAG 镜像；unused：未被任何容器（含已停止容器）引用的镜像。
func PickImagesForCleanup(images []MyType.Image, filter string) []MyType.Image {
	picked := make([]MyType.Image, 0, len(images))
	for _, img := range images {
		switch filter {
		case CleanupFilterUntagged:
			if isUntaggedImage(img) {
				picked = append(picked, img)
			}
		case CleanupFilterUnused:
			if !img.InUsed {
				picked = append(picked, img)
			}
		}
	}
	return picked
}

// RemoveImagesFiltered 批量删除镜像，逐个尝试，单个失败不影响其它。
// 返回已删除与失败的镜像清单（失败时带原因）。filter 只接受上面两个常量。
func RemoveImagesFiltered(ctx *svc.ServiceContext, filter string) (removed []string, failed []string, err error) {
	if filter != CleanupFilterUntagged && filter != CleanupFilterUnused {
		return nil, nil, fmt.Errorf("filter 只支持 %s 或 %s", CleanupFilterUntagged, CleanupFilterUnused)
	}
	images, err := GetImagesList(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, img := range PickImagesForCleanup(images, filter) {
		if removeErr := RemoveImage(ctx, img.ID, false); removeErr != nil {
			logx.Errorf("批量清理镜像失败 %s: %v", img.ID, removeErr)
			failed = append(failed, img.ID+" ("+img.ImageName+":"+img.ImageTag+"): "+removeErr.Error())
			continue
		}
		removed = append(removed, img.ID+" ("+img.ImageName+":"+img.ImageTag+")")
	}
	return removed, failed, nil
}
