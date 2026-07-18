package versionup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const sampleData = "hello versionup downloader\n"

func TestHTTPDownloader_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleData))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	dl := NewHTTPDownloader()
	if err := dl.Download(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != sampleData {
		t.Errorf("content = %q", string(got))
	}
	if _, err := os.Stat(dest + ".part"); err == nil {
		t.Error("temp .part file should be removed after success")
	}
}

func TestHTTPDownloader_ChecksumPass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleData))
	}))
	defer srv.Close()

	sum := sha256.Sum256([]byte(sampleData))
	want := "sha256:" + hex.EncodeToString(sum[:])

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")
	dl := NewHTTPDownloader()
	if err := dl.Download(context.Background(), srv.URL, dest, WithChecksum(want)); err != nil {
		t.Fatalf("Download with checksum: %v", err)
	}
}

func TestHTTPDownloader_ChecksumFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleData))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")
	dl := NewHTTPDownloader()
	err := dl.Download(context.Background(), srv.URL, dest, WithChecksum("sha256:deadbeef"))
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	// 失败时不应留下目标文件
	if _, err := os.Stat(dest); err == nil {
		t.Error("destination file should not exist on checksum failure")
	}
}

func TestHTTPDownloader_Progress(t *testing.T) {
	data := make([]byte, 1024*4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	var called bool
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")
	dl := NewHTTPDownloader()
	err := dl.Download(context.Background(), srv.URL, dest, WithProgress(func(done, total int64) {
		called = true
	}))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !called {
		t.Error("progress callback not invoked")
	}
}

func TestHTTPDownloader_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")
	dl := NewHTTPDownloader()
	if err := dl.Download(context.Background(), srv.URL, dest); err == nil {
		t.Error("expected error on 404")
	}
}
