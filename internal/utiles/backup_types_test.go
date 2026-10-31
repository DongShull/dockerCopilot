package utiles

import (
	"encoding/json"
	"strings"
	"testing"

	dockerBackend "github.com/docker/docker/api/types/backend"
)

func TestContainerBackupEntryKeepsStateAndOldFormat(t *testing.T) {
	entry := ContainerBackupEntry{
		ContainerCreateConfig: dockerBackend.ContainerCreateConfig{Name: "demo"},
		StateKnown:            true,
		WasRunning:            true,
		WasPaused:             true,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"Name":"demo"`, `"stateKnown":true`, `"wasRunning":true`, `"wasPaused":true`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("marshaled entry missing %s: %s", want, data)
		}
	}

	var old []ContainerBackupEntry
	if err := json.Unmarshal([]byte(`[{"Name":"old","Config":{"Image":"nginx"}}]`), &old); err != nil {
		t.Fatal(err)
	}
	if len(old) != 1 || old[0].Name != "old" || old[0].Config == nil || old[0].Config.Image != "nginx" {
		t.Fatalf("old format unmarshal failed: %+v", old)
	}
	if old[0].StateKnown {
		t.Fatal("old format should be treated as state unknown")
	}
}
