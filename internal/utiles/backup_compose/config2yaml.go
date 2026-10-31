package backupCompose

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	composeType "github.com/compose-spec/compose-go/types"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	composeNat "github.com/docker/go-connections/nat"
	"github.com/zeromicro/go-zero/core/logx"
	"sigs.k8s.io/yaml"
)

// DockerConfig2ComposeYaml 将docker config转换为docker-compose.yaml
func DockerConfig2ComposeYaml(containerJSONs []dockerTypes.ContainerJSON, backupDir string) (err error) {
	var c composeYaml
	c.Services = make(map[string]composeType.ServiceConfig, len(containerJSONs))
	c.Volumes = make(composeType.Volumes)
	c.Networks = make(composeType.Networks)
	for i, containerJSON := range containerJSONs {
		var s composeType.ServiceConfig
		formatBaseServiceConfig(containerJSON, &s)
		if s.Name == "" {
			s.Name = fmt.Sprintf("container-%d", i+1)
			s.ContainerName = s.Name
		}
		formatEnvServiceConfig(containerJSON, &s)
		formatNetworkServiceConfig(containerJSON, &s)
		formatVolumeServiceConfig(containerJSON, &s)
		c.Services[s.Name] = s

		// 收集命名卷。bind mount 不需要顶层声明。
		for _, mount := range containerJSON.Mounts {
			if mount.Type == "volume" {
				source := volumeSource(mount)
				if source == "" {
					continue
				}
				volumeConfig := composeType.VolumeConfig{Name: source}
				if mount.Driver != "" {
					volumeConfig.Driver = mount.Driver
				}
				c.Volumes[source] = volumeConfig
			}
		}
		// 收集自定义网络。default/bridge/host/none 不需要在 compose 里声明。
		if containerJSON.NetworkSettings != nil {
			for networkName := range containerJSON.NetworkSettings.Networks {
				if networkName == "" || networkName == "bridge" || networkName == "host" || networkName == "none" {
					continue
				}
				c.Networks[networkName] = composeType.NetworkConfig{Name: networkName}
			}
		}
	}
	if len(c.Volumes) == 0 {
		c.Volumes = nil
	}
	if len(c.Networks) == 0 {
		c.Networks = nil
	}

	// write to file
	if backupDir == "" {
		backupDir = "/data/backups"
	}
	_, err = os.Stat(backupDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(backupDir, 0755)
		if err != nil {
			logx.Errorf("创建备份目录失败: %v", err)
			return err
		}
	}
	yamlData, yamlMarshalErr := yaml.Marshal(c)
	if yamlMarshalErr != nil {
		logx.Errorf("序列化 compose 数据失败: %v", yamlMarshalErr)
		return yamlMarshalErr
	}
	// 文件名带时间戳：同一天多次导出互不覆盖；旧备份由调用方按 BACKUP_KEEP 清理。
	fileName := "backup-" + time.Now().Format("2006-01-02-150405") + ".yaml"
	fullPath := filepath.Join(backupDir, fileName)
	tmp, err := os.CreateTemp(backupDir, ".backup-*.yaml.tmp")
	if err != nil {
		logx.Errorf("创建 compose 临时文件失败: %v", err)
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err = tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		logx.Errorf("设置 compose 临时文件权限失败: %v", err)
		return err
	}
	if _, err = tmp.Write(yamlData); err != nil {
		_ = tmp.Close()
		logx.Errorf("写入 compose 临时文件失败: %v", err)
		return err
	}
	if err = tmp.Close(); err != nil {
		logx.Errorf("关闭 compose 临时文件失败: %v", err)
		return err
	}
	if err = os.Rename(tmpName, fullPath); err != nil {
		logx.Errorf("替换 compose 备份文件失败: %v", err)
		return err
	}
	return
}

func formatBaseServiceConfig(containerJSON dockerTypes.ContainerJSON, s *composeType.ServiceConfig) {
	if containerJSON.Config != nil {
		cfg := containerJSON.Config
		s.Image = cfg.Image
		s.Tty = cfg.Tty
		s.StdinOpen = cfg.OpenStdin
		s.User = cfg.User
		s.DomainName = cfg.Domainname
		if cfg.MacAddress != "" {
			s.MacAddress = cfg.MacAddress
		}
		// 容器默认 hostname 是容器 ID；导出成固定值会在重建时变成旧 ID，
		// 只有用户显式设置了 hostname 才写入 compose。
		if cfg.Hostname != "" && !sameShortID(cfg.Hostname, containerJSON.ID) {
			s.Hostname = cfg.Hostname
		}
		if len(cfg.Entrypoint) > 0 {
			s.Entrypoint = composeType.ShellCommand(cfg.Entrypoint)
		}
		s.WorkingDir = cfg.WorkingDir
		if len(cfg.Cmd) > 0 {
			s.Command = composeType.ShellCommand(cfg.Cmd)
		}
		s.Labels = cleanComposeLabels(cfg.Labels)
		s.StopSignal = cfg.StopSignal
		if cfg.StopTimeout != nil {
			s.StopGracePeriod = durationPtr(time.Duration(*cfg.StopTimeout) * time.Second)
		}
		s.HealthCheck = formatHealthCheck(cfg.Healthcheck)
		s.Expose = formatExposedPorts(cfg.ExposedPorts)
	}
	name, cutNameResult := strings.CutPrefix(containerJSON.Name, "/")
	if !cutNameResult {
		logx.Infof("容器名缺少 / 前缀: %s", containerJSON.Name)
	}
	s.ContainerName = name
	s.Name = name
	if containerJSON.HostConfig != nil {
		formatHostConfig(containerJSON.HostConfig, containerJSON.Config, s)
	}
	if containerJSON.Platform != "" {
		s.Platform = containerJSON.Platform
	}
	return
}

func formatHostConfig(h *container.HostConfig, cfg *container.Config, s *composeType.ServiceConfig) {
	s.Restart = formatRestartPolicy(h.RestartPolicy)
	s.Privileged = h.Privileged
	s.ReadOnly = h.ReadonlyRootfs
	s.Init = h.Init
	s.CapAdd = []string(h.CapAdd)
	s.CapDrop = []string(h.CapDrop)
	s.CgroupParent = h.CgroupParent
	s.Cgroup = string(h.Cgroup)
	s.DNS = composeType.StringList(h.DNS)
	s.DNSOpts = h.DNSOptions
	s.DNSSearch = composeType.StringList(h.DNSSearch)
	s.ExtraHosts = parseExtraHosts(h.ExtraHosts)
	s.GroupAdd = h.GroupAdd
	s.Ipc = string(h.IpcMode)
	s.Links = h.Links
	s.Pid = string(h.PidMode)
	s.SecurityOpt = h.SecurityOpt
	s.ShmSize = composeType.UnitBytes(h.ShmSize)
	s.Sysctls = composeType.Mapping(h.Sysctls)
	s.Tmpfs = formatTmpfs(h.Tmpfs)
	s.Ulimits = formatUlimits(h.Ulimits)
	s.UserNSMode = string(h.UsernsMode)
	s.Uts = string(h.UTSMode)
	s.VolumeDriver = h.VolumeDriver
	s.VolumesFrom = h.VolumesFrom
	s.Runtime = h.Runtime
	s.Isolation = string(h.Isolation)
	s.Devices = formatDevices(h.Devices)
	s.DeviceCgroupRules = h.DeviceCgroupRules
	s.Annotations = composeType.Mapping(h.Annotations)
	s.OomScoreAdj = int64(h.OomScoreAdj)
	s.OomKillDisable = h.OomKillDisable != nil && *h.OomKillDisable
	if h.PidsLimit != nil {
		s.PidsLimit = *h.PidsLimit
	}
	if h.MemorySwappiness != nil {
		s.MemSwappiness = composeType.UnitBytes(*h.MemorySwappiness)
	}
	s.MemLimit = composeType.UnitBytes(h.Memory)
	s.MemReservation = composeType.UnitBytes(h.MemoryReservation)
	s.MemSwapLimit = composeType.UnitBytes(h.MemorySwap)
	s.CPUShares = h.CPUShares
	s.CPUQuota = h.CPUQuota
	s.CPUPeriod = h.CPUPeriod
	s.CPURTPeriod = h.CPURealtimePeriod
	s.CPURTRuntime = h.CPURealtimeRuntime
	s.CPUSet = h.CpusetCpus
	s.CPUS = float32(h.NanoCPUs) / 1e9
	s.CPUCount = h.CPUCount
	s.CPUPercent = float32(h.CPUPercent)
	if h.BlkioWeight > 0 {
		s.BlkioConfig = &composeType.BlkioConfig{Weight: h.BlkioWeight}
	}
	if h.LogConfig.Type != "" || len(h.LogConfig.Config) > 0 {
		logging := &composeType.LoggingConfig{
			Options: composeType.Options(h.LogConfig.Config),
		}
		if h.LogConfig.Type != "" {
			logging.Driver = h.LogConfig.Type
		}
		s.Logging = logging
	}
	if cfg != nil {
		if cfg.NetworkDisabled {
			s.NetworkMode = "none"
		}
	}
}

func formatEnvServiceConfig(containerJSON dockerTypes.ContainerJSON, s *composeType.ServiceConfig) {
	if containerJSON.Config == nil {
		return
	}
	s.Environment = composeType.NewMappingWithEquals(containerJSON.Config.Env)
}

func formatNetworkServiceConfig(containerJSON dockerTypes.ContainerJSON, s *composeType.ServiceConfig) {
	if containerJSON.HostConfig == nil {
		return
	}
	mode := string(containerJSON.HostConfig.NetworkMode)
	// compose 的 network_mode 只接受 host/none/container:...；自定义网络要用
	// services.<name>.networks 挂载，不能把网络名塞进 network_mode。
	if mode == "host" || mode == "none" || strings.HasPrefix(mode, "container:") {
		s.NetworkMode = mode
	}
	if containerJSON.NetworkSettings != nil {
		for networkName, endpoint := range containerJSON.NetworkSettings.Networks {
			if networkName == "" || networkName == "bridge" || networkName == "host" || networkName == "none" {
				continue
			}
			networkConfig := &composeType.ServiceNetworkConfig{}
			if endpoint != nil {
				networkConfig.Aliases = filterAliases(endpoint.Aliases, containerJSON.ID, containerJSON.Name)
				// 只导出用户静态指定的地址；运行中的 IPAddress 是 Docker 动态分配的，
				// 写成 ipv4_address 会在重建时造成地址冲突。
				if endpoint.IPAMConfig != nil {
					networkConfig.Ipv4Address = endpoint.IPAMConfig.IPv4Address
					networkConfig.Ipv6Address = endpoint.IPAMConfig.IPv6Address
					networkConfig.LinkLocalIPs = endpoint.IPAMConfig.LinkLocalIPs
				}
			}
			if s.Networks == nil {
				s.Networks = make(map[string]*composeType.ServiceNetworkConfig)
			}
			s.Networks[networkName] = networkConfig
		}
	}
	for containerPort, bindings := range containerJSON.HostConfig.PortBindings {
		if len(bindings) == 0 {
			// 某些容器的 PortBindings 是空切片，直接 v[0] 会 panic。
			logx.Errorf("端口 %s 没有绑定地址，跳过", containerPort)
			continue
		}
		proto, port := composeNat.SplitProtoPort(string(containerPort))
		portNum, convertErr := strconv.Atoi(port)
		if convertErr != nil {
			logx.Errorf("端口转换失败: %v", convertErr)
			continue
		}
		for _, binding := range bindings {
			s.Ports = append(s.Ports, composeType.ServicePortConfig{
				HostIP:    binding.HostIP,
				Published: binding.HostPort,
				Target:    uint32(portNum),
				Protocol:  proto,
			})
		}
	}
}

func formatVolumeServiceConfig(containerJSON dockerTypes.ContainerJSON, s *composeType.ServiceConfig) {
	for _, containerVolume := range containerJSON.Mounts {
		if string(containerVolume.Type) == "tmpfs" {
			// HostConfig.Tmpfs 已经会导出成 tmpfs:，这里再写 volumes 会造成重复挂载点。
			if containerJSON.HostConfig != nil {
				if _, exists := containerJSON.HostConfig.Tmpfs[containerVolume.Destination]; exists {
					continue
				}
			}
			// tmpfs 没有 Source，长语法只需要 type + target。
			s.Volumes = append(s.Volumes, composeType.ServiceVolumeConfig{
				Type:     "tmpfs",
				Target:   containerVolume.Destination,
				ReadOnly: !containerVolume.RW,
			})
			continue
		}
		source := volumeSource(containerVolume)
		if source == "" {
			continue
		}
		v := composeType.ServiceVolumeConfig{
			Type:     string(containerVolume.Type),
			Source:   source,
			Target:   containerVolume.Destination,
			ReadOnly: !containerVolume.RW,
		}
		for _, part := range strings.Split(containerVolume.Mode, ",") {
			switch strings.TrimSpace(part) {
			case "z", "Z":
				if v.Bind == nil {
					v.Bind = &composeType.ServiceVolumeBind{}
				}
				v.Bind.SELinux = part
			case "nocopy":
				if v.Type == "volume" {
					v.Volume = &composeType.ServiceVolumeVolume{NoCopy: true}
				}
			}
		}
		if containerVolume.Propagation != "" {
			if v.Bind == nil {
				v.Bind = &composeType.ServiceVolumeBind{}
			}
			v.Bind.Propagation = string(containerVolume.Propagation)
		}
		s.Volumes = append(s.Volumes, v)
	}
}

func filterAliases(aliases []string, containerID, containerName string) []string {
	if len(aliases) == 0 {
		return nil
	}
	name := strings.TrimPrefix(containerName, "/")
	shortID := containerID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	result := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		if alias == "" || alias == name || alias == containerID || alias == shortID {
			continue
		}
		result = append(result, alias)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func volumeSource(mount dockerTypes.MountPoint) string {
	if mount.Type == "volume" {
		// 匿名卷/异常卷没有 Name 时不要退化成 Source 宿主机路径，否则会导出非法卷名。
		return mount.Name
	}
	return mount.Source
}

func formatExposedPorts(ports composeNat.PortSet) composeType.StringOrNumberList {
	if len(ports) == 0 {
		return nil
	}
	result := make([]string, 0, len(ports))
	for port := range ports {
		result = append(result, string(port))
	}
	return composeType.StringOrNumberList(result)
}

func formatRestartPolicy(policy container.RestartPolicy) string {
	name := string(policy.Name)
	if name == "on-failure" && policy.MaximumRetryCount > 0 {
		return fmt.Sprintf("on-failure:%d", policy.MaximumRetryCount)
	}
	return name
}

func formatHealthCheck(hc *container.HealthConfig) *composeType.HealthCheckConfig {
	if hc == nil {
		return nil
	}
	if len(hc.Test) == 1 && strings.EqualFold(hc.Test[0], "NONE") {
		return &composeType.HealthCheckConfig{Disable: true}
	}
	// 没有 test 的 healthcheck 无法还原；继续写会生成 compose 不接受的空 test。
	if len(hc.Test) == 0 {
		return nil
	}
	out := &composeType.HealthCheckConfig{
		Test: composeType.HealthCheckTest(hc.Test),
	}
	if hc.Interval > 0 {
		out.Interval = durationPtr(hc.Interval)
	}
	if hc.Timeout > 0 {
		out.Timeout = durationPtr(hc.Timeout)
	}
	if hc.StartPeriod > 0 {
		out.StartPeriod = durationPtr(hc.StartPeriod)
	}
	if hc.StartInterval > 0 {
		out.StartInterval = durationPtr(hc.StartInterval)
	}
	if hc.Retries > 0 {
		retries := uint64(hc.Retries)
		out.Retries = &retries
	}
	return out
}

func durationPtr(d time.Duration) *composeType.Duration {
	value := composeType.Duration(d)
	return &value
}

func parseExtraHosts(hosts []string) composeType.HostsList {
	if len(hosts) == 0 {
		return nil
	}
	result := make(composeType.HostsList, len(hosts))
	for _, entry := range hosts {
		host, ip, ok := strings.Cut(entry, ":")
		if !ok || host == "" || ip == "" {
			continue
		}
		result[host] = ip
	}
	return result
}

func formatTmpfs(tmpfs map[string]string) composeType.StringList {
	if len(tmpfs) == 0 {
		return nil
	}
	result := make([]string, 0, len(tmpfs))
	for path, opts := range tmpfs {
		if opts == "" {
			result = append(result, path)
		} else {
			result = append(result, path+":"+opts)
		}
	}
	return composeType.StringList(result)
}

func formatUlimits(ulimits []*container.Ulimit) map[string]*composeType.UlimitsConfig {
	if len(ulimits) == 0 {
		return nil
	}
	result := make(map[string]*composeType.UlimitsConfig, len(ulimits))
	for _, ulimit := range ulimits {
		if ulimit == nil || ulimit.Name == "" {
			continue
		}
		item := &composeType.UlimitsConfig{}
		if ulimit.Soft == ulimit.Hard {
			item.Single = int(ulimit.Soft)
		} else {
			item.Soft = int(ulimit.Soft)
			item.Hard = int(ulimit.Hard)
		}
		result[ulimit.Name] = item
	}
	return result
}

func formatDevices(devices []container.DeviceMapping) []string {
	if len(devices) == 0 {
		return nil
	}
	result := make([]string, 0, len(devices))
	for _, device := range devices {
		entry := device.PathOnHost + ":" + device.PathInContainer
		if device.CgroupPermissions != "" {
			entry += ":" + device.CgroupPermissions
		}
		result = append(result, entry)
	}
	return result
}

func cleanComposeLabels(labels map[string]string) composeType.Labels {
	if len(labels) == 0 {
		return nil
	}
	result := make(composeType.Labels, len(labels))
	for key, value := range labels {
		if strings.HasPrefix(key, "com.docker.compose.") {
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func sameShortID(hostname, id string) bool {
	if hostname == "" || id == "" {
		return false
	}
	if len(id) > 12 {
		id = id[:12]
	}
	return hostname == id
}
