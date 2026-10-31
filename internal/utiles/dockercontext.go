package utiles

import (
	"context"
	"time"
)

const (
	dockerListTimeout      = 60 * time.Second
	dockerInspectTimeout   = 60 * time.Second
	dockerMutationTimeout  = 2 * time.Minute
	dockerNegotiateTimeout = 30 * time.Second
)

// dockerTimeout 给单次 Docker API 调用加超时。
// 更新/恢复的 ImagePull 使用任务上下文，不在这里限制，避免大镜像被误杀。
func dockerTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, timeout)
}
