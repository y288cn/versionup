# versionup

[![Go Reference](https://pkg.go.dev/badge/github.com/y288cn/versionup.svg)](https://pkg.go.dev/github.com/y288cn/versionup)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

`versionup` 是一个纯 Go 实现的桌面软件版本升级公共库。它帮助客户端**周期性向服务端询问工具的最新稳定版本**，与当前版本比对，发现更新时**发出升级事件**（含下载地址、版本号、工具 ID 等），并提供**事件上报、通知开关、定时通知、版本下载**等能力。

> 设计边界：库负责「发现 + 通知 + 下载」，不内置「自替换 / 重启 / 提权 / 配置迁移 / 回滚」等平台相关执行逻辑——这些通过 `Applier` 接口由调用方注入（详见[应用层](#应用层-applier自动更新)）。

## 特性

- 周期检查 / 立即检查（`Start` / `CheckNow`）
- 语义化版本比对（SemVer 2.0，预发布不参与稳定升级）
- 升级事件分发（`Notifier`）与事件上报（`Reporter`）
- 通知开关（`Enable` / `Disable`）与指定时间通知（`NotifyAt`）
- HTTP 下载（进度回调 + sha256 校验 + 原子落盘）
- 零 CGO、仅依赖标准库，跨平台编译
- 可组合：所有组件均为接口，可自定义实现

## Install

```bash
go get github.com/y288cn/versionup
```

要求 Go 1.21+（本仓库使用 `go 1.21`）。

## Quick Start

```go
package main

import (
    "context"
    "log"
    "time"

    versionup "github.com/y288cn/versionup"
)

func main() {
    up := versionup.New(
        "github-speed", "v1.3.0",
        versionup.WithSource(versionup.NewHTTPSource("https://api.example.com")),
        versionup.WithReporter(versionup.NewHTTPReporter("https://api.example.com/report")),
        versionup.WithInterval(24*time.Hour),
        versionup.WithLogger(log.Default()),
    )

    // 订阅升级事件（事件含下载地址、版本号、工具 id 等）
    up.OnUpdate(func(ev versionup.UpdateEvent) {
        log.Printf("发现新版本 %s -> %s, 下载: %s", ev.CurrentVersion, ev.LatestVersion, ev.DownloadURL)
    })

    // 阻塞运行，直到 ctx 取消
    if err := up.Start(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

更多场景见 [`examples/basic`](examples/basic/main.go)（可直接 `go run ./examples/basic` 运行）。

## 服务端协议

`NewHTTPSource` 默认访问：

```
GET {baseURL}/version?tool={toolID}
```

返回 JSON：

```json
{
  "tool_id": "github-speed",
  "version": "v1.4.0",
  "stable": true,
  "download_url": "https://example.com/github-speed-v1.4.0.zip",
  "release_notes": "修复若干 bug",
  "published_at": "2026-07-01T00:00:00Z",
  "checksum": "sha256:9f86d0818..."
}
```

- `stable=false` 的版本不会触发升级。
- `checksum` 可选，下载时可用 `WithChecksum` 校验。
- 也可用 `NewManifestSource` 指向一个静态 `version.json`（支持 `map[string]Release` 或单个 `Release`）。

## 接口一览

| 接口 | 职责 | 默认实现 |
| --- | --- | --- |
| `Source` | 拉取某工具最新稳定版 | `NewHTTPSource` / `NewManifestSource` |
| `Reporter` | 上报升级事件 | `NewHTTPReporter` |
| `Notifier` | 通知开关 / 订阅 / 定时 | `NewNotifier` |
| `Downloader` | 下载安装包（进度/校验） | `NewHTTPDownloader` |
| `Updater` | 顶层聚合（周期/即时检查） | `New` |

### 升级事件 `UpdateEvent`

```go
type UpdateEvent struct {
    ToolID         string    // 工具 ID
    CurrentVersion string    // 当前版本
    LatestVersion  string    // 最新稳定版本
    DownloadURL    string    // 下载地址
    ReleaseNotes   string    // 更新说明
    PublishedAt    time.Time // 发布时间
    DetectedAt     time.Time // 检测到的时间
}
```

## 应用层（Applier / 自动更新）

下载到的包可能是**安装器 / 压缩包 / 单个可执行文件**三种形态。库提供抽象接口与跨平台安全原语，具体「如何应用」由调用方注入，避免把平台相关逻辑固化进公共包。

```go
// Applier 应用（安装）下载到的更新包。
type Applier interface {
    Apply(ctx context.Context, pkgPath string, ev UpdateEvent) error
}
```

库提供的原语（纯 Go，零 CGO）：

- `ExtractArchive(src, dest string, opts ...ArchiveOption) error`：解压 zip / tar / tar.gz / tar.xz；`WithFileHook` 可在解压每个条目时跳过（用于保留用户配置）。
- `RunInstaller(path string, args ...string) error`：启动安装器进程。
- `VerifyChecksum(path, sum string) error`：sha256 校验。

三种形态对应策略（建议放项目侧或可选子包 `versionup/apply`，核心库不内置）：

- 安装器：`InstallerStrategy` 直接 `RunInstaller`
- 压缩包：`ArchiveStrategy` 解压 + `Preserve(rel)` hook 跳过用户配置
- 单可执行文件：`SelfReplaceStrategy` 平台相关（Windows 文件锁需改名/重启），推荐参考 `go-update` / Squirrel 等成熟方案

通过 `WithApplier` / `WithAutoApply(bool)` 接入 `Updater`（`AutoApply` 默认 `false`，即仅通知）。

## License

[MIT](LICENSE) – 可自由使用、修改、再发布，仅需保留版权与许可声明。
