package versionup

import "time"

// Release 服务端返回的一个稳定发布版本。
type Release struct {
	ToolID       string    `json:"tool_id"`
	Version      string    `json:"version"`       // 语义化版本，如 v1.2.3
	Stable       bool      `json:"stable"`        // 是否稳定版
	DownloadURL  string    `json:"download_url"`  // 安装包下载地址
	ReleaseNotes string    `json:"release_notes"` // 更新说明
	PublishedAt  time.Time `json:"published_at"`
	Checksum     string    `json:"checksum"` // 可选，sha256 hex（可带 "sha256:" 前缀）
}

// UpdateEvent 版本升级事件。当比对发现「有新稳定版本」时由库构造并分发。
type UpdateEvent struct {
	ToolID         string
	CurrentVersion string
	LatestVersion  string
	DownloadURL    string
	ReleaseNotes   string
	PublishedAt    time.Time
	DetectedAt     time.Time
}

// newUpdateEvent 由 Release 与当前版本构造升级事件。
func newUpdateEvent(current string, r *Release) *UpdateEvent {
	return &UpdateEvent{
		ToolID:         r.ToolID,
		CurrentVersion: current,
		LatestVersion:  r.Version,
		DownloadURL:    r.DownloadURL,
		ReleaseNotes:   r.ReleaseNotes,
		PublishedAt:    r.PublishedAt,
		DetectedAt:     time.Now(),
	}
}
