// Command basic 演示 versionup 的最小可运行用法：
// 启动一个本地假服务端（httptest），周期检查更新、订阅通知、下载安装包、并演示定时通知。
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	versionup "github.com/y288cn/versionup"
)

func main() {
	// 1) 启动一个本地假服务端，模拟「下载地址」返回的 Release JSON。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tool_id": "demo",
			"version": "v2.0.0",
			"stable": true,
			"download_url": "https://example.com/demo-v2.0.0.zip",
			"release_notes": "修复若干 bug，提升稳定性",
			"checksum": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		}`))
	}))
	defer srv.Close()

	// 2) 构造 Updater：周期检查 + 上报（这里用同一个假端点，仅演示）。
	up := versionup.New(
		"demo", "v1.3.0",
		versionup.WithSource(versionup.NewHTTPSource(srv.URL)),
		versionup.WithReporter(versionup.NewHTTPReporter(srv.URL+"/report")),
		versionup.WithInterval(30*time.Second),
		versionup.WithLogger(log.Default()),
	)

	// 3) 订阅升级通知（事件中包含下载地址、版本号、工具 id 等）。
	up.OnUpdate(func(ev versionup.UpdateEvent) {
		fmt.Printf("[通知] 发现新版本 %s -> %s\n", ev.CurrentVersion, ev.LatestVersion)
		fmt.Printf("        下载地址: %s\n", ev.DownloadURL)
		fmt.Printf("        更新说明: %s\n", ev.ReleaseNotes)

		// 4) 触发下载（实际项目中可在此调用 Downloader）。
		dl := versionup.NewHTTPDownloader()
		dest := filepath.Join(os.TempDir(), "demo-"+ev.LatestVersion+".zip")
		fmt.Printf("[下载] 保存到 %s ...\n", dest)
		if err := dl.Download(context.Background(), ev.DownloadURL, dest); err != nil {
			log.Printf("下载失败（演示地址无效，属正常）: %v", err)
			return
		}
		fmt.Println("[下载] 完成")
	})

	// 5) 演示「指定时间通知」：10 秒后再次投递一次通知。
	up.Notifier.NotifyAt(time.Now().Add(10*time.Second), versionup.UpdateEvent{
		ToolID:        "demo",
		LatestVersion: "v2.0.0",
		DownloadURL:   "https://example.com/demo-v2.0.0.zip",
	})

	// 6) 启动周期检查（阻塞；用 signal 或 ctx 取消来停止）。
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	fmt.Println("[启动] 开始周期检查（每 30s 一次，45s 后自动退出演示）")
	if err := up.Start(ctx); err != nil {
		log.Fatalf("start: %v", err)
	}
	fmt.Println("[退出] 演示结束")
}
