package svc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/client"
	"github.com/onlyLTY/dockerCopilot/internal/config"
	"github.com/onlyLTY/dockerCopilot/internal/module"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

type ServiceContext struct {
	Config                     config.Config
	CookieCheckMiddleware      rest.Middleware
	Jwtuuid                    string
	BearerTokenCheckMiddleware rest.Middleware
	JwtSecret                  string
	PortainerJwt               string
	HubImageInfo               *module.ImageUpdateData
	IndexCheckMiddleware       rest.Middleware
	ProgressStore              ProgressStoreType
	DockerClient               *client.Client
	DockerInitErr              error
	mu                         sync.Mutex
	taskCancels                map[string]context.CancelFunc
	saveMu                     sync.Mutex
	lastProgressSave           time.Time
}

type TaskProgress struct {
	TaskID     string `json:"taskID"`
	Percentage int    `json:"percentage"`
	Message    string `json:"message"`
	Name       string `json:"name"`
	DetailMsg  string `json:"detailMsg"`
	IsDone     bool   `json:"isDone"`
	StartedAt  int64  `json:"startedAt"`
	FinishedAt int64  `json:"finishedAt"`
}

type ProgressStoreType map[string]TaskProgress

func NewServiceContext(c config.Config) *ServiceContext {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logx.Errorf("创建 Docker 客户端失败: %s", err)
	}
	ctx := &ServiceContext{
		Config:        c,
		HubImageInfo:  module.NewImageCheck(),
		ProgressStore: make(ProgressStoreType),
		taskCancels:   make(map[string]context.CancelFunc),
		DockerClient:  cli,
		DockerInitErr: err,
	}
	ctx.loadProgress()
	go ctx.taskJanitor()
	return ctx
}

// progressStorePath 进度持久化文件路径，可用 PROGRESS_STORE 覆盖。
func progressStorePath() string {
	if p := os.Getenv("PROGRESS_STORE"); p != "" {
		return p
	}
	return "/data/config/progress.json"
}

// loadProgress 从磁盘恢复历史任务进度（重启不丢历史）。
func (ctx *ServiceContext) loadProgress() {
	data, err := os.ReadFile(progressStorePath())
	if err != nil {
		return
	}
	var stored ProgressStoreType
	if err = json.Unmarshal(data, &stored); err != nil {
		logx.Errorf("进度文件解析失败: %v", err)
		return
	}
	now := time.Now().Unix()
	interrupted := false
	ctx.mu.Lock()
	for k, v := range stored {
		if !v.IsDone {
			// 进程刚重启，磁盘里还处于“进行中”的任务不可能再继续执行。
			v.IsDone = true
			v.FinishedAt = now
			v.Percentage = 100
			v.Message = "进程重启，任务已中断"
			v.DetailMsg = "任务执行期间服务重启，请重新发起"
			interrupted = true
		}
		ctx.ProgressStore[k] = v
	}
	ctx.mu.Unlock()
	if interrupted {
		ctx.saveProgress()
	}
}

// saveProgress 原子写入进度文件（临时文件 + rename）。
// 序列化和落盘都持有 ctx.mu：否则并发保存时，旧快照可能后落盘，
// 把更新的任务状态覆盖掉。进度写入频率已被 UpdateProgress 限制，
// 这里短暂持锁不会成为任务更新的瓶颈。
func (ctx *ServiceContext) saveProgress() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	data, err := json.Marshal(ctx.ProgressStore)
	if err != nil {
		logx.Errorf("进度序列化失败: %v", err)
		return
	}
	ctx.saveMu.Lock()
	defer ctx.saveMu.Unlock()
	path := progressStorePath()
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logx.Errorf("创建进度目录失败: %v", err)
		return
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0644); err != nil {
		logx.Errorf("写进度临时文件失败: %v", err)
		return
	}
	if err = os.Rename(tmp, path); err != nil {
		logx.Errorf("进度文件落地失败: %v", err)
	}
}

// taskJanitor 定期清理超过 TTL 的已完成任务，避免内存与进度文件无限增长。
func (ctx *ServiceContext) taskJanitor() {
	ttl := 24 * time.Hour
	if v := os.Getenv("PROGRESS_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttl = time.Duration(n) * time.Hour
		}
	}
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-ttl).Unix()
		removed := false
		ctx.mu.Lock()
		for k, v := range ctx.ProgressStore {
			if v.IsDone && v.FinishedAt > 0 && v.FinishedAt < cutoff {
				delete(ctx.ProgressStore, k)
				if cancel, ok := ctx.taskCancels[k]; ok {
					cancel()
					delete(ctx.taskCancels, k)
				}
				removed = true
			}
		}
		ctx.mu.Unlock()
		if removed {
			ctx.saveProgress()
		}
	}
}

func (ctx *ServiceContext) UpdateProgress(taskID string, progress TaskProgress) {
	ctx.mu.Lock()
	now := time.Now().Unix()
	if progress.StartedAt == 0 {
		if old, ok := ctx.ProgressStore[taskID]; ok {
			progress.StartedAt = old.StartedAt
		}
		if progress.StartedAt == 0 {
			progress.StartedAt = now
		}
	}
	if progress.IsDone {
		if old, ok := ctx.ProgressStore[taskID]; ok && old.IsDone && old.FinishedAt > 0 {
			progress.FinishedAt = old.FinishedAt
		}
		if progress.FinishedAt == 0 {
			progress.FinishedAt = now
		}
	}
	ctx.ProgressStore[taskID] = progress
	// 高频拉取进度只留在内存；最多每秒落盘一次，完成时立即落盘。
	shouldSave := progress.IsDone || time.Since(ctx.lastProgressSave) >= time.Second
	if shouldSave {
		ctx.lastProgressSave = time.Now()
	}
	ctx.mu.Unlock()
	if shouldSave {
		ctx.saveProgress()
	}
}

func (ctx *ServiceContext) GetProgress(taskID string) (TaskProgress, bool) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	progress, ok := ctx.ProgressStore[taskID]
	return progress, ok
}

// ListProgress 返回全部任务进度，按开始时间倒序。
func (ctx *ServiceContext) ListProgress() []TaskProgress {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	list := make([]TaskProgress, 0, len(ctx.ProgressStore))
	for _, v := range ctx.ProgressStore {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].StartedAt != list[j].StartedAt {
			return list[i].StartedAt > list[j].StartedAt
		}
		return list[i].TaskID > list[j].TaskID
	})
	return list
}

// RegisterTask 为任务创建可取消的上下文。
func (ctx *ServiceContext) RegisterTask(taskID string) context.Context {
	taskCtx, cancel := context.WithCancel(context.Background())
	ctx.mu.Lock()
	if ctx.taskCancels == nil {
		ctx.taskCancels = make(map[string]context.CancelFunc)
	}
	ctx.taskCancels[taskID] = cancel
	ctx.mu.Unlock()
	return taskCtx
}

// CancelTask 取消一个进行中的任务，返回是否找到该任务。
func (ctx *ServiceContext) CancelTask(taskID string) bool {
	ctx.mu.Lock()
	cancel, ok := ctx.taskCancels[taskID]
	ctx.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// CancelActiveTask 原子地检查并取消任务，避免“刚好完成”时被取消逻辑覆盖状态。
func (ctx *ServiceContext) CancelActiveTask(taskID string) (ok bool, msg string) {
	ctx.mu.Lock()
	progress, exists := ctx.ProgressStore[taskID]
	if !exists {
		ctx.mu.Unlock()
		return false, "taskID 未找到"
	}
	if progress.IsDone {
		ctx.mu.Unlock()
		return false, "任务已结束，无需取消"
	}
	cancel, exists := ctx.taskCancels[taskID]
	if !exists {
		ctx.mu.Unlock()
		return false, "任务不可取消（可能已结束）"
	}
	cancel()
	progress.Message = "正在取消..."
	progress.DetailMsg = "已发出取消请求，任务将在当前步骤结束后停止"
	ctx.ProgressStore[taskID] = progress
	ctx.mu.Unlock()
	ctx.saveProgress()
	return true, ""
}

// FinishTask 任务收尾，释放取消句柄。
func (ctx *ServiceContext) FinishTask(taskID string) {
	ctx.mu.Lock()
	if cancel, ok := ctx.taskCancels[taskID]; ok {
		cancel()
		delete(ctx.taskCancels, taskID)
	}
	ctx.mu.Unlock()
}
