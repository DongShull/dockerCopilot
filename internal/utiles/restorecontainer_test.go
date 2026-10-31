package utiles

import (
	"strings"
	"testing"
	"time"
)

func TestRestoreShouldStart(t *testing.T) {
	trueValue := true
	falseValue := false
	tests := []struct {
		name     string
		entry    ContainerBackupEntry
		override *bool
		want     bool
	}{
		{name: "old backup defaults to start", entry: ContainerBackupEntry{}, want: true},
		{name: "known stopped stays stopped", entry: ContainerBackupEntry{StateKnown: true, WasRunning: false}, want: false},
		{name: "known running starts", entry: ContainerBackupEntry{StateKnown: true, WasRunning: true}, want: true},
		{name: "override start", entry: ContainerBackupEntry{StateKnown: true, WasRunning: false}, override: &trueValue, want: true},
		{name: "override stop", entry: ContainerBackupEntry{StateKnown: true, WasRunning: true}, override: &falseValue, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := restoreShouldStart(tt.entry, tt.override); got != tt.want {
				t.Fatalf("restoreShouldStart() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestoreOldContainerName(t *testing.T) {
	at := time.Date(2026, 10, 30, 12, 34, 56, 123456789, time.UTC)
	got := restoreOldContainerName("demo", at)
	if !strings.HasPrefix(got, "demo-dc-old-") {
		t.Fatalf("restoreOldContainerName() = %q, want demo-dc-old- prefix", got)
	}
	if got == restoreOldContainerName("demo", at.Add(time.Nanosecond)) {
		t.Fatalf("restoreOldContainerName() should be unique per timestamp")
	}
}

func TestRollbackRestoredContainerNoop(t *testing.T) {
	if msg := rollbackRestoredContainer(nil, "", "", "", "", false, false); msg != "" {
		t.Fatalf("rollbackRestoredContainer() = %q, want empty", msg)
	}
}
