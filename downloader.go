package versionup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadOption 下载选项。
type DownloadOption func(*downloadConfig)

type downloadConfig struct {
	progress func(done, total int64)
	checksum string // "sha256:..." 或纯 hex
}

// WithProgress 设置下载进度回调（done/total，total 未知时为 -1）。
func WithProgress(fn func(done, total int64)) DownloadOption {
	return func(c *downloadConfig) { c.progress = fn }
}

// WithChecksum 设置下载后校验的 sha256 值（"sha256:..." 或纯 hex）。
func WithChecksum(sum string) DownloadOption {
	return func(c *downloadConfig) { c.checksum = sum }
}

// Downloader 下载器：把版本包下载到本地。
type Downloader interface {
	Download(ctx context.Context, url, dest string, opts ...DownloadOption) error
}

type httpDownloader struct {
	client *http.Client
}

// DownloaderOption 配置下载器。
type DownloaderOption func(*httpDownloader)

// WithDownloadClient 设置自定义 *http.Client。
func WithDownloadClient(c *http.Client) DownloaderOption {
	return func(d *httpDownloader) { d.client = c }
}

// NewHTTPDownloader 创建默认 HTTP 下载器。
func NewHTTPDownloader(opts ...DownloaderOption) Downloader {
	d := &httpDownloader{client: &http.Client{}}
	for _, o := range opts {
		o(d)
	}
	return d
}

func (d *httpDownloader) Download(ctx context.Context, url, dest string, opts ...DownloadOption) error {
	cfg := &downloadConfig{}
	for _, o := range opts {
		o(cfg)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloader: unexpected status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }() // 仅失败时清理；成功时已 rename 走

	var w io.Writer = f
	var h hash.Hash
	if cfg.checksum != "" {
		h = sha256.New()
		w = io.MultiWriter(f, h)
	}

	total := resp.ContentLength
	var done int64
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		nr, rerr := resp.Body.Read(buf)
		if nr > 0 {
			nw, werr := w.Write(buf[:nr])
			done += int64(nw)
			if cfg.progress != nil {
				cfg.progress(done, total)
			}
			if werr != nil {
				return werr
			}
			if nw != nr {
				return io.ErrShortWrite
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}

	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	if h != nil {
		got := hex.EncodeToString(h.Sum(nil))
		if !matchChecksum(got, cfg.checksum) {
			return fmt.Errorf("downloader: checksum mismatch: got %s, want %s", got, cfg.checksum)
		}
	}

	return os.Rename(tmp, dest)
}

// matchChecksum 比较 sha256 十六进制串（忽略 "sha256:" 前缀，大小写不敏感）。
func matchChecksum(got, want string) bool {
	want = strings.TrimPrefix(want, "sha256:")
	return strings.EqualFold(got, want)
}
