package versionup

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// makeZip 生成一个含给定文件内容的 zip 文件，返回路径。
func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	f, err := os.Create(zp)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return zp
}

// makeTarGz 生成一个含给定文件内容的 tar.gz 文件，返回路径。
func makeTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	tp := filepath.Join(dir, "a.tar.gz")
	f, err := os.Create(tp)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return tp
}

func TestExtractArchive_Zip(t *testing.T) {
	zp := makeZip(t, map[string]string{
		"app.exe":    "binary-content",
		"readme.txt": "hello",
	})
	dest := t.TempDir()
	if err := ExtractArchive(zp, dest); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "app.exe"))
	if err != nil || string(got) != "binary-content" {
		t.Errorf("app.exe = %q, err=%v", string(got), err)
	}
	got2, err := os.ReadFile(filepath.Join(dest, "readme.txt"))
	if err != nil || string(got2) != "hello" {
		t.Errorf("readme.txt = %q, err=%v", string(got2), err)
	}
}

func TestExtractArchive_TarGz(t *testing.T) {
	tp := makeTarGz(t, map[string]string{
		"bin/tool": "tool-binary",
		"cfg/x":    "x",
	})
	dest := t.TempDir()
	if err := ExtractArchive(tp, dest, WithStripComponents(1)); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	// strip=1 应去掉 "bin"/"cfg" 顶层
	if _, err := os.Stat(filepath.Join(dest, "tool")); err != nil {
		t.Errorf("expected tool after strip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "x")); err != nil {
		t.Errorf("expected x after strip: %v", err)
	}
}

func TestExtractArchive_FileHookSkip(t *testing.T) {
	zp := makeZip(t, map[string]string{
		"app.exe":    "new-binary",
		"config.ini": "new-config",
	})
	dest := t.TempDir()
	// 预置用户配置
	if err := os.WriteFile(filepath.Join(dest, "config.ini"), []byte("user-config"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用 ArchiveStrategy 验证保留行为
	strat := ArchiveStrategy{
		Dest:     dest,
		Preserve: func(rel string) bool { return rel == "config.ini" },
	}
	if err := strat.Apply(context.Background(), zp, UpdateEvent{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	cfg, err := os.ReadFile(filepath.Join(dest, "config.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg) != "user-config" {
		t.Errorf("config.ini should be preserved, got %q", string(cfg))
	}
	app, err := os.ReadFile(filepath.Join(dest, "app.exe"))
	if err != nil || string(app) != "new-binary" {
		t.Errorf("app.exe = %q, err=%v", string(app), err)
	}
}

func TestExtractArchive_Unsupported(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "x.xyz")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExtractArchive(src, dir); err == nil {
		t.Error("expected unsupported error")
	}
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256Hex([]byte("data"))
	if err := VerifyChecksum(p, "sha256:"+sum); err != nil {
		t.Errorf("VerifyChecksum pass: %v", err)
	}
	if err := VerifyChecksum(p, "sha256:"+sha256Hex([]byte("other"))); err == nil {
		t.Error("expected mismatch error")
	}
}

func TestRunInstaller(t *testing.T) {
	// 成功：cmd /c exit 0
	if err := RunInstaller("cmd", "/c", "exit", "0"); err != nil {
		t.Errorf("RunInstaller success: %v", err)
	}
	// 失败：cmd /c exit 1 应返回 *exec.ExitError
	if err := RunInstaller("cmd", "/c", "exit", "1"); err == nil {
		t.Error("expected non-zero exit error")
	}
}

func TestInstallerStrategy(t *testing.T) {
	s := InstallerStrategy{Args: []string{"/c", "exit", "0"}}
	if err := s.Apply(context.Background(), "cmd", UpdateEvent{}); err != nil {
		t.Errorf("InstallerStrategy.Apply: %v", err)
	}
}

func TestSelfReplaceStrategy(t *testing.T) {
	s := SelfReplaceStrategy{CurrentExe: "me.exe"}
	err := s.Apply(context.Background(), "new.exe", UpdateEvent{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
