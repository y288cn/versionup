package versionup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Reporter 版本升级事件上报接口。
type Reporter interface {
	Report(ctx context.Context, event UpdateEvent) error
}

type httpReporter struct {
	endpoint  string
	client    *http.Client
	userAgent string
}

// ReporterOption 配置上报器。
type ReporterOption func(*httpReporter)

// WithReporterClient 设置自定义 *http.Client。
func WithReporterClient(c *http.Client) ReporterOption {
	return func(r *httpReporter) { r.client = c }
}

// NewHTTPReporter 创建默认 HTTP 上报器：POST 事件 JSON 到 endpoint。
// 上报失败仅返回错误，由调用方决定是否忽略（检查循环不因此中断）。
func NewHTTPReporter(endpoint string, opts ...ReporterOption) Reporter {
	r := &httpReporter{
		endpoint:  endpoint,
		client:    &http.Client{Timeout: 15 * time.Second},
		userAgent: "versionup-reporter",
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r *httpReporter) Report(ctx context.Context, event UpdateEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("reporter: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", r.userAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("reporter: unexpected status %d", resp.StatusCode)
	}
	return nil
}
