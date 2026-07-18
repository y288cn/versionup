package versionup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Source 版本源：拉取某工具最新稳定版。
type Source interface {
	Latest(ctx context.Context, toolID string) (*Release, error)
}

type sourceConfig struct {
	client    *http.Client
	userAgent string
}

// SourceOption 配置版本源（如自定义 HTTP Client）。
type SourceOption func(*sourceConfig)

// WithSourceClient 设置自定义 *http.Client。
func WithSourceClient(c *http.Client) SourceOption {
	return func(sc *sourceConfig) { sc.client = c }
}

// WithSourceUserAgent 设置 User-Agent。
func WithSourceUserAgent(ua string) SourceOption {
	return func(sc *sourceConfig) { sc.userAgent = ua }
}

func newSourceConfig(opts ...SourceOption) sourceConfig {
	sc := sourceConfig{
		client:    &http.Client{Timeout: 15 * time.Second},
		userAgent: "versionup-client",
	}
	for _, o := range opts {
		o(&sc)
	}
	return sc
}

// httpSource 通过 HTTP 端点查询最新版本。
type httpSource struct {
	baseURL string
	sourceConfig
}

// NewHTTPSource 创建基于 HTTP 端点的版本源。
// 端点约定：GET {baseURL}/version?tool={toolID} 返回 Release JSON（见文档 §7）。
func NewHTTPSource(baseURL string, opts ...SourceOption) Source {
	return &httpSource{
		baseURL:      strings.TrimRight(baseURL, "/"),
		sourceConfig: newSourceConfig(opts...),
	}
}

func (s *httpSource) Latest(ctx context.Context, toolID string) (*Release, error) {
	u := fmt.Sprintf("%s/version?tool=%s", s.baseURL, url.QueryEscape(toolID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("source: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("source: decode: %w", err)
	}
	return &r, nil
}

// manifestSource 从静态 version.json 清单拉取最新版本。
// 清单格式支持：map[string]Release（按 toolID 索引）或单个 Release（其 ToolID 须匹配）。
type manifestSource struct {
	url string
	sourceConfig
}

// NewManifestSource 创建基于静态清单文件的版本源。
func NewManifestSource(rawURL string, opts ...SourceOption) Source {
	return &manifestSource{
		url:         rawURL,
		sourceConfig: newSourceConfig(opts...),
	}
}

func (s *manifestSource) Latest(ctx context.Context, toolID string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	// 优先按 map[string]Release 解析
	var m map[string]Release
	if err := json.Unmarshal(body, &m); err == nil {
		r, ok := m[toolID]
		if !ok {
			return nil, fmt.Errorf("manifest: tool %q not found", toolID)
		}
		return &r, nil
	}

	// 退化为单个 Release
	var r Release
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("manifest: decode: %w", err)
	}
	if r.ToolID != toolID {
		return nil, fmt.Errorf("manifest: tool %q not found", toolID)
	}
	return &r, nil
}
