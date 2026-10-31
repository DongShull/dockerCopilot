package svc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestProgressConcurrentUpdatesPersistValidJSON(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "progress.json")
	t.Setenv("PROGRESS_STORE", storePath)

	ctx := &ServiceContext{ProgressStore: make(ProgressStoreType)}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx.UpdateProgress(fmt.Sprintf("task-%d", i), TaskProgress{
				TaskID:     fmt.Sprintf("task-%d", i),
				Percentage: 100,
				Message:    "done",
				IsDone:     true,
			})
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read progress store: %v", err)
	}
	var stored ProgressStoreType
	if err = json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("progress store is not valid JSON: %v\n%s", err, data)
	}
	if len(stored) != 20 {
		t.Fatalf("stored tasks = %d, want 20", len(stored))
	}
	if progress, ok := ctx.GetProgress("task-7"); !ok || !progress.IsDone {
		t.Fatalf("task-7 not persisted in memory: %+v, %v", progress, ok)
	}
}
