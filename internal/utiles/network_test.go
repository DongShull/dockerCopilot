package utiles

import (
	"testing"

	"github.com/docker/docker/api/types/network"
)

func TestSanitizeEndpointSettings(t *testing.T) {
	original := map[string]*network.EndpointSettings{
		"demo": {
			Aliases:             []string{"demo-alias"},
			MacAddress:          "02:42:ac:11:00:02",
			NetworkID:           "net-id",
			EndpointID:          "endpoint-id",
			Gateway:             "172.18.0.1",
			IPAddress:           "172.18.0.5",
			IPPrefixLen:         16,
			IPv6Gateway:         "fe80::1",
			GlobalIPv6Address:   "2001:db8::5",
			GlobalIPv6PrefixLen: 64,
			DNSNames:            []string{"demo"},
			IPAMConfig: &network.EndpointIPAMConfig{
				IPv4Address: "172.18.0.9",
				IPv6Address: "2001:db8::9",
			},
		},
		"nil": nil,
	}

	got := sanitizeEndpointSettings(original)
	if got == nil {
		t.Fatal("sanitizeEndpointSettings() = nil")
	}
	demo := got["demo"]
	if demo == nil {
		t.Fatal("demo endpoint missing")
	}
	if demo.IPAddress != "" || demo.NetworkID != "" || demo.EndpointID != "" ||
		demo.Gateway != "" || demo.IPPrefixLen != 0 || demo.IPv6Gateway != "" ||
		demo.GlobalIPv6Address != "" || demo.GlobalIPv6PrefixLen != 0 ||
		demo.MacAddress != "" || demo.DNSNames != nil {
		t.Fatalf("operational fields were not cleared: %#v", demo)
	}
	if demo.IPAMConfig == nil || demo.IPAMConfig.IPv4Address != "172.18.0.9" || demo.IPAMConfig.IPv6Address != "2001:db8::9" {
		t.Fatalf("IPAMConfig was not preserved: %#v", demo.IPAMConfig)
	}
	if len(demo.Aliases) != 1 || demo.Aliases[0] != "demo-alias" {
		t.Fatalf("Aliases were not preserved: %#v", demo.Aliases)
	}
	if got["nil"] != nil {
		t.Fatalf("nil endpoint should stay nil")
	}
	if original["demo"].IPAddress != "172.18.0.5" {
		t.Fatal("sanitizeEndpointSettings mutated the original map")
	}
}

func TestRestoreNetworkingConfig(t *testing.T) {
	if got := restoreNetworkingConfig(nil); got != nil {
		t.Fatalf("restoreNetworkingConfig(nil) = %#v, want nil", got)
	}
	empty := &network.NetworkingConfig{}
	if got := restoreNetworkingConfig(empty); got != empty {
		t.Fatalf("restoreNetworkingConfig(empty) should return original pointer")
	}
	config := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			"demo": {IPAddress: "172.18.0.5", Aliases: []string{"demo"}},
		},
	}
	got := restoreNetworkingConfig(config)
	if got.EndpointsConfig["demo"].IPAddress != "" {
		t.Fatalf("IPAddress was not cleared: %#v", got.EndpointsConfig["demo"])
	}
	if len(got.EndpointsConfig["demo"].Aliases) != 1 {
		t.Fatalf("Aliases were not preserved: %#v", got.EndpointsConfig["demo"])
	}
	if config.EndpointsConfig["demo"].IPAddress != "172.18.0.5" {
		t.Fatal("restoreNetworkingConfig mutated the input")
	}
}
