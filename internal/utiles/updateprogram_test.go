package utiles

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeTarGz(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	gzw := gzip.NewWriter(file)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	for name, content := range entries {
		header := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDecompressTarGzAllowsNormalRootFile(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "dockerCopilot.tar.gz")
	writeTarGz(t, archive, map[string]string{"dockerCopilot-new": "binary"})

	dest := t.TempDir()
	if err := decompressTarGz(archive, dest); err != nil {
		t.Fatalf("decompressTarGz() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "dockerCopilot-new"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "binary" {
		t.Fatalf("content = %q, want binary", data)
	}
}

func TestDecompressTarGzRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "dockerCopilot.tar.gz")
	writeTarGz(t, archive, map[string]string{"../escape": "bad"})

	dest := t.TempDir()
	if err := decompressTarGz(archive, dest); err == nil {
		t.Fatal("decompressTarGz() expected traversal error, got nil")
	}
}

func TestVerifySHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	if err := verifySHA256(path, fmt.Sprintf("%x", sum)); err != nil {
		t.Fatalf("verifySHA256() error = %v", err)
	}
	if err := verifySHA256(path, "deadbeef"); err == nil {
		t.Fatal("verifySHA256() expected mismatch error")
	}
}

func TestDownloadFileRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "out")
	if err := downloadFile(server.URL, dest); err == nil {
		t.Fatal("downloadFile() expected HTTP 404 error")
	}
}

func TestFetchVersionFromURLRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := fetchVersionFromURL(server.URL); err == nil {
		t.Fatal("fetchVersionFromURL() expected HTTP 404 error")
	}
}
