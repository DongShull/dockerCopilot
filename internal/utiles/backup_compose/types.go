package backupCompose

import (
	composeType "github.com/compose-spec/compose-go/types"
)

type composeYaml struct {
	Services map[string]composeType.ServiceConfig `yaml:"services" json:"services"`
	// 顶层声明命名卷和自定义网络。没有这两个声明时，导出的 compose 在
	// `docker compose up` 时无法还原卷/网络，只能得到匿名卷。
	Volumes  composeType.Volumes  `yaml:"volumes,omitempty" json:"volumes,omitempty"`
	Networks composeType.Networks `yaml:"networks,omitempty" json:"networks,omitempty"`
}
