package versionup

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"
)

// Checker 周期检查器：按间隔向 Source 询问最新版本，发现更新则分发事件并上报。
type Checker struct {
	toolID    string
	current   string
	interval  time.Duration
	source    Source
	notifier  Notifier
	reporter  Reporter
	applier   Applier
	autoApply bool
	downloader Downloader
	logger    Logger
}

func newChecker(toolID, current string, src Source, n Notifier, r Reporter, interval time.Duration, applier Applier, autoApply bool) *Checker {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	c := &Checker{
		toolID:    toolID,
		current:   current,
		interval:  interval,
		source:    src,
		notifier:  n,
		reporter:  r,
		applier:   applier,
		autoApply: autoApply,
	}
	if c.downloader == nil {
		c.downloader = NewHTTPDownloader()
	}
	return c
}

// CheckOnce 立即执行一次检查。返回发现的升级事件（无更新时为 nil, nil）。
func (c *Checker) CheckOnce(ctx context.Context) (*UpdateEvent, error) {
	rel, err := c.source.Latest(ctx, c.toolID)
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("[versionup] check %q failed: %v", c.toolID, err)
		}
		return nil, err
	}
	if !NeedsUpgrade(c.current, rel.Version, rel.Stable) {
		return nil, nil
	}
	ev := newUpdateEvent(c.current, rel)
	if c.notifier != nil {
		c.notifier.Dispatch(*ev)
	}
	if c.reporter != nil {
		if rerr := c.reporter.Report(ctx, *ev); rerr != nil && c.logger != nil {
			c.logger.Printf("[versionup] report %q failed: %v", c.toolID, rerr)
		}
	}

	// 可选自动应用：发现升级且开启 AutoApply 时，下载并交给 Applier 应用。
	if c.autoApply && c.applier != nil && ev.DownloadURL != "" {
		if err := c.autoApplyUpdate(ctx, ev, rel); err != nil && c.logger != nil {
			c.logger.Printf("[versionup] auto-apply %q failed: %v", c.toolID, err)
		}
	}
	return ev, nil
}

// autoApplyUpdate 下载安装包（含校验）并调用 Applier.Apply。失败仅返回错误，不阻塞检查循环。
func (c *Checker) autoApplyUpdate(ctx context.Context, ev *UpdateEvent, rel *Release) error {
	dest := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s%s", c.toolID, ev.LatestVersion, extFromURL(ev.DownloadURL)))
	opts := []DownloadOption{}
	if rel.Checksum != "" {
		opts = append(opts, WithChecksum(rel.Checksum))
	}
	if err := c.downloader.Download(ctx, ev.DownloadURL, dest, opts...); err != nil {
		return err
	}
	return c.applier.Apply(ctx, dest, *ev)
}

// extFromURL 从下载地址路径取扩展名，无则默认 .bin。
func extFromURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		if e := path.Ext(u.Path); e != "" {
			return e
		}
	}
	return ".bin"
}

// Start 启动周期检查，阻塞直到 ctx 取消。启动后立即检查一次，之后按 interval 周期检查。
func (c *Checker) Start(ctx context.Context) {
	_, _ = c.CheckOnce(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = c.CheckOnce(ctx)
		}
	}
}
