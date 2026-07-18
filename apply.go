package versionup

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// 应用层相关错误。
var (
	// ErrSkip 在 WithFileHook 中返回，表示跳过当前归档条目（用于保留用户配置）。
	ErrSkip = errors.New("versionup: skip this archive entry")
	// ErrNotImplemented 表示某策略尚未实现（如 SelfReplaceStrategy 需平台相关逻辑）。
	ErrNotImplemented = errors.New("versionup: not implemented")
)

// Applier 应用（安装）下载到的更新包。具体策略由项目选择并注入，
// 库不固化平台相关的「替换/重启/提权/配置迁移/回滚」逻辑。
type Applier interface {
	Apply(ctx context.Context, pkgPath string, ev UpdateEvent) error
}

// ---------------------------------------------------------------------------
// 跨平台安全原语
// ---------------------------------------------------------------------------

// ArchiveOption 配置解压行为。
type ArchiveOption func(*archiveConfig)

type archiveConfig struct {
	fileHook        func(rel string, r io.Reader) error
	stripComponents int
}

// WithFileHook 注册每个普通文件条目的回调，在写入前调用。
// 回调收到相对路径与条目内容读取器：
//   - 返回 ErrSkip 跳过该条目（如保留现有用户配置、不覆盖）；
//   - 返回其他错误则中止解压；
//   - 返回 nil 表示由库写入；此时回调【不得】消费读取器。
func WithFileHook(fn func(rel string, r io.Reader) error) ArchiveOption {
	return func(c *archiveConfig) { c.fileHook = fn }
}

// WithStripComponents 跳过路径前 n 个组件（类似 tar --strip-components）。
func WithStripComponents(n int) ArchiveOption {
	return func(c *archiveConfig) { c.stripComponents = n }
}

// ExtractArchive 解压 src 到 dest 目录，按扩展名识别格式：
// zip / tar / tar.gz(.tgz) / tar.bz2(.tbz/.tbz2)。
// tar.xz(.txz) 因标准库无 xz 解码器，返回 ErrNotImplemented（需外部依赖）。
func ExtractArchive(src, dest string, opts ...ArchiveOption) error {
	cfg := &archiveConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	lower := strings.ToLower(src)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(src, dest, cfg)
	case strings.HasSuffix(lower, ".tar"):
		return extractTarFile(src, dest, cfg, "")
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractTarFile(src, dest, cfg, "gz")
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"), strings.HasSuffix(lower, ".tbz"):
		return extractTarFile(src, dest, cfg, "bz2")
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return fmt.Errorf("versionup: tar.xz requires external xz dependency: %w", ErrNotImplemented)
	default:
		return fmt.Errorf("versionup: unsupported archive type: %s", src)
	}
}

func extractZip(src, dest string, cfg *archiveConfig) error {
	zr, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, zf := range zr.File {
		rel := stripLeading(cfg.stripComponents, zf.Name)
		if rel == "" || strings.HasPrefix(rel, "..") {
			continue
		}
		target := filepath.Join(dest, rel)
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, zf.Mode()); err != nil {
				return err
			}
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		if cfg.fileHook != nil {
			if herr := cfg.fileHook(rel, rc); herr != nil {
				rc.Close()
				if errors.Is(herr, ErrSkip) {
					continue
				}
				return herr
			}
		}
		if err := writeFileFromReader(target, rc, zf.Mode()); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}

func extractTarFile(src, dest string, cfg *archiveConfig, algo string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = f
	switch algo {
	case "gz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		r = gz
	case "bz2":
		r = bzip2.NewReader(f)
	}
	return extractTarReader(r, dest, cfg)
}

func extractTarReader(r io.Reader, dest string, cfg *archiveConfig) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		rel := stripLeading(cfg.stripComponents, hdr.Name)
		if rel == "" || strings.HasPrefix(rel, "..") {
			continue
		}
		target := filepath.Join(dest, rel)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if cfg.fileHook != nil {
				if herr := cfg.fileHook(rel, tr); herr != nil {
					if errors.Is(herr, ErrSkip) {
						continue
					}
					return herr
				}
			}
			if err := writeFileFromReader(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		default:
			// 跳过符号链接/硬链接/设备等，避免安全风险
			continue
		}
	}
	return nil
}

// stripLeading 去掉路径前 n 个组件并清理，防止目录穿越。
func stripLeading(n int, name string) string {
	if n <= 0 {
		return filepath.Clean(name)
	}
	parts := strings.Split(name, "/")
	if len(parts) <= n {
		return ""
	}
	return filepath.Clean(strings.Join(parts[n:], "/"))
}

func writeFileFromReader(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// RunInstaller 启动下载到的安装器（.exe/.msi/.pkg/.deb 等）并等待其退出。
// 提权、是否等待完成由调用方决定；本函数仅负责启动进程。
func RunInstaller(path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// VerifyChecksum 校验文件 sha256 是否与 sum（"sha256:..." 或纯 hex）一致。
func VerifyChecksum(path, sum string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !matchChecksum(got, sum) {
		return fmt.Errorf("versionup: checksum mismatch: got %s, want %s", got, sum)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 三种形态策略骨架（核心库不内置平台相关执行逻辑）
// ---------------------------------------------------------------------------

// InstallerStrategy 处理「安装器」形态：直接启动安装器，后续由其自身完成。
type InstallerStrategy struct {
	Args []string
}

// Apply 启动安装器。
func (s InstallerStrategy) Apply(ctx context.Context, pkg string, ev UpdateEvent) error {
	return RunInstaller(pkg, s.Args...)
}

// ArchiveStrategy 处理「压缩包」形态：解压到 Dest，并通过 Preserve 保留用户配置（不覆盖）。
type ArchiveStrategy struct {
	Dest           string // 解压目标目录
	StripComponents int   // 跳过压缩包内前 N 层目录
	Preserve       func(rel string) bool // 返回 true 表示保留现有文件（跳过不覆盖）
}

// Apply 解压压缩包，按 Preserve 跳过用户配置文件。
func (s ArchiveStrategy) Apply(ctx context.Context, pkg string, ev UpdateEvent) error {
	opts := []ArchiveOption{}
	if s.StripComponents > 0 {
		opts = append(opts, WithStripComponents(s.StripComponents))
	}
	if s.Preserve != nil {
		opts = append(opts, WithFileHook(func(rel string, r io.Reader) error {
			if s.Preserve(rel) {
				return ErrSkip
			}
			return nil
		}))
	}
	return ExtractArchive(pkg, s.Dest, opts...)
}

// SelfReplaceStrategy 处理「单可执行文件」形态：平台相关（Windows 文件锁需改名/重启），
// 核心库不实现。推荐参考 go-update / Squirrel 等成熟方案，或项目侧按目标平台实现：
//  1) 下载新 exe 到临时名；2) 将当前 exe 重命名为 .old；
//  3) 新 exe 落到原路径；4) 启动新 exe 并退出自己；
//     若仍被锁则用 MoveFileEx 的 MOVEFILE_DELAY_UNTIL_REBOOT 延迟到重启替换。
type SelfReplaceStrategy struct {
	CurrentExe string
}

// Apply 返回 ErrNotImplemented 占位。
func (s SelfReplaceStrategy) Apply(ctx context.Context, pkg string, ev UpdateEvent) error {
	return ErrNotImplemented
}
