package utiles

import "github.com/docker/docker/api/types/network"

// sanitizeEndpointSettings 只保留创建容器需要的网络配置，清掉运行期产生的
// NetworkID/EndpointID/IP/MAC 等字段。否则把旧容器的 inspect 结果直接传给
// ContainerCreate，可能继承失效的端点信息或造成 MAC/IP 冲突。
func sanitizeEndpointSettings(networks map[string]*network.EndpointSettings) map[string]*network.EndpointSettings {
	if len(networks) == 0 {
		return nil
	}
	result := make(map[string]*network.EndpointSettings, len(networks))
	for name, endpoint := range networks {
		if endpoint == nil {
			result[name] = nil
			continue
		}
		cleaned := endpoint.Copy()
		cleaned.NetworkID = ""
		cleaned.EndpointID = ""
		cleaned.Gateway = ""
		cleaned.IPAddress = ""
		cleaned.IPPrefixLen = 0
		cleaned.IPv6Gateway = ""
		cleaned.GlobalIPv6Address = ""
		cleaned.GlobalIPv6PrefixLen = 0
		cleaned.MacAddress = ""
		cleaned.DNSNames = nil
		result[name] = cleaned
	}
	return result
}
