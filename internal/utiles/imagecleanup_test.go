package utiles

import (
	"testing"

	MyType "github.com/onlyLTY/dockerCopilot/internal/types"
)

func TestPickImagesForCleanupUntagged(t *testing.T) {
	images := []MyType.Image{
		{ImageName: "nginx", ImageTag: "latest", InUsed: false},
		{ImageName: "None", ImageTag: "None", InUsed: false},
		{ImageName: "<none>", ImageTag: "<none>", InUsed: true},
		{ImageName: "redis", ImageTag: "None", InUsed: false},
	}
	picked := PickImagesForCleanup(images, CleanupFilterUntagged)
	if len(picked) != 3 {
		t.Fatalf("expected 3 untagged images, got %d: %+v", len(picked), picked)
	}
	for _, img := range picked {
		if img.ImageName == "nginx" {
			t.Fatalf("tagged image should not be picked: %+v", img)
		}
	}
}

func TestPickImagesForCleanupUnused(t *testing.T) {
	images := []MyType.Image{
		{ImageName: "nginx", ImageTag: "latest", InUsed: false},
		{ImageName: "redis", ImageTag: "7", InUsed: true},
		{ImageName: "None", ImageTag: "None", InUsed: false},
	}
	picked := PickImagesForCleanup(images, CleanupFilterUnused)
	if len(picked) != 2 {
		t.Fatalf("expected 2 unused images, got %d", len(picked))
	}
	for _, img := range picked {
		if img.InUsed {
			t.Fatalf("used image should not be picked: %+v", img)
		}
	}
}

func TestPickImagesForCleanupUnknownFilter(t *testing.T) {
	images := []MyType.Image{
		{ImageName: "nginx", ImageTag: "latest", InUsed: false},
	}
	if picked := PickImagesForCleanup(images, "bogus"); len(picked) != 0 {
		t.Fatalf("unknown filter should pick nothing, got %d", len(picked))
	}
}
