package utiles

import (
	dockerBackend "github.com/docker/docker/api/types/backend"
)

// ContainerBackupEntry 是备份文件中的一条容器记录。
// 前四个字段来自旧的 ContainerCreateConfig 备份格式，保持向后兼容；
// StateKnown 之后是新增字段，旧备份没有这些字段时按“状态未知”处理。
type ContainerBackupEntry struct {
	dockerBackend.ContainerCreateConfig
	StateKnown bool `json:"stateKnown,omitempty"`
	WasRunning bool `json:"wasRunning,omitempty"`
	WasPaused  bool `json:"wasPaused,omitempty"`
}
