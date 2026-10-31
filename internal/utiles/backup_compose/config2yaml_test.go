package backupCompose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	composeType "github.com/compose-spec/compose-go/types"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

func TestDockerConfig2ComposeYamlEmptyPortBindingsAndTopLevel(t *testing.T) {
	dir := t.TempDir()
	id := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	stopTimeout := 15
	containerJSON := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			ID:       id,
			Name:     "/demo",
			Image:    "sha256:old",
			Platform: "linux",
			HostConfig: &container.HostConfig{
				NetworkMode: "demo-net",
				PortBindings: nat.PortMap{
					nat.Port("80/tcp"): {},
				},
				CapAdd:  []string{"NET_ADMIN"},
				CapDrop: []string{"MKNOD"},
				DNS:     []string{"1.1.1.1"},
				Resources: container.Resources{
					Devices: []container.DeviceMapping{
						{PathOnHost: "/dev/fuse", PathInContainer: "/dev/fuse", CgroupPermissions: "rwm"},
					},
				},
				RestartPolicy: container.RestartPolicy{Name: "on-failure", MaximumRetryCount: 3},
				LogConfig:     container.LogConfig{Type: "json-file", Config: map[string]string{"max-size": "10m"}},
			},
		},
		Config: &container.Config{
			Hostname:    id[:12], // 默认 hostname 等于短容器 ID，不应写入 compose
			Image:       "nginx:latest",
			StopTimeout: &stopTimeout,
			Labels: map[string]string{
				"com.docker.compose.project": "old-project",
				"app":                        "demo",
			},
			Healthcheck: &container.HealthConfig{
				Test:     []string{"CMD-SHELL", "curl -f http://localhost/ || exit 1"},
				Interval: 30 * time.Second,
				Timeout:  5 * time.Second,
				Retries:  3,
			},
		},
		Mounts: []types.MountPoint{
			{
				Type:        "volume",
				Name:        "demo-data",
				Source:      "/var/lib/docker/volumes/demo-data/_data",
				Destination: "/data",
				RW:          true,
			},
			{
				Type:        "tmpfs",
				Destination: "/tmp/cache",
				RW:          true,
			},
		},
		NetworkSettings: &types.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"demo-net": {
					IPAddress: "172.18.0.5",
					Aliases:   []string{"demo", "0123456789ab", "real-alias"},
				},
			},
		},
	}

	if err := DockerConfig2ComposeYaml([]types.ContainerJSON{containerJSON}, dir); err != nil {
		t.Fatalf("DockerConfig2ComposeYaml() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "backup-*.yaml"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("backup files = %v, err = %v", matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"volumes:",
		"demo-data:",
		"networks:",
		"demo-net:",
		"healthcheck:",
		"cap_add:",
		"devices:",
		"restart: on-failure:3",
		"stop_grace_period: 15s",
		"app: demo",
		"type: tmpfs",
		"target: /tmp/cache",
		"real-alias",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("yaml missing %q:\n%s", want, text)
		}
	}
	for _, notWant := range []string{
		"/var/lib/docker/volumes",
		"com.docker.compose.project",
		"hostname: abc123",
		"172.18.0.5",
		"0123456789ab",
	} {
		if strings.Contains(text, notWant) {
			t.Fatalf("yaml should not contain %q:\n%s", notWant, text)
		}
	}
}

func TestFormatVolumeServiceConfigSkipsTmpfsAlreadyInHostConfig(t *testing.T) {
	containerJSON := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			HostConfig: &container.HostConfig{
				Tmpfs: map[string]string{"/tmp/cache": "size=64m"},
			},
		},
		Mounts: []types.MountPoint{
			{Type: "tmpfs", Destination: "/tmp/cache", RW: true},
			{Type: "tmpfs", Destination: "/tmp/other", RW: true},
			{Type: "volume", Source: "/var/lib/docker/volumes/anonymous/_data", Destination: "/anon", RW: true},
		},
	}
	var service composeType.ServiceConfig
	formatVolumeServiceConfig(containerJSON, &service)

	if len(service.Volumes) != 1 {
		t.Fatalf("volumes = %#v, want only the tmpfs missing from HostConfig.Tmpfs", service.Volumes)
	}
	if service.Volumes[0].Target != "/tmp/other" {
		t.Fatalf("volume target = %q, want /tmp/other", service.Volumes[0].Target)
	}
}
