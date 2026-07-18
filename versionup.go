package versionup

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Logger 最小日志接口，*log.Logger 直接满足（*slog.Logger 可用适配包装）。
// 默认 nil 表示静默。
type Logger interface {
	Printf(format string, args ...any)
}

// Updater 版本升级器：组合 Source / Reporter / Notifier / Checker，提供周期检查与即时检查。
type Updater struct {
	ToolID   string
	Current  string
	Source   Source
	Reporter Reporter
	Notifier Notifier
	Checker  *Checker

	interval time.Duration
	logger   Logger
	applier  Applier
	autoApply bool

	mu     sync.Mutex
	cancel context.CancelFunc
}

// Option 配置 Updater。
type Option func(*Updater)

// WithSource 设置版本源（必填，否则 Start/CheckNow 返回错误）。
func WithSource(s Source) Option { return func(u *Updater) { u.Source = s } }

// WithReporter 设置事件上报器（可选）。
func WithReporter(r Reporter) Option { return func(u *Updater) { u.Reporter = r } }

// WithNotifier 设置通知管理器（可选，默认 NewNotifier 且开启）。
func WithNotifier(n Notifier) Option { return func(u *Updater) { u.Notifier = n } }

// WithInterval 设置检查间隔（默认 24h）。
func WithInterval(d time.Duration) Option { return func(u *Updater) { u.interval = d } }

// WithLogger 设置日志输出（默认静默）。
func WithLogger(l Logger) Option { return func(u *Updater) { u.logger = l } }

// WithApplier 注入应用更新包的策略（安装器/解压/自替换）。
func WithApplier(a Applier) Option { return func(u *Updater) { u.applier = a } }

// WithAutoApply 控制「发现更新后是否自动调用 Applier.Apply」，默认 false（仅通知）。
func WithAutoApply(b bool) Option { return func(u *Updater) { u.autoApply = b } }

// New 创建 Updater。Source 必须通过 WithSource 提供，否则 Start/CheckNow 会返回错误。
func New(toolID, current string, opts ...Option) *Updater {
	u := &Updater{
		ToolID:   toolID,
		Current:  current,
		Notifier: NewNotifier(),
		interval: 24 * time.Hour,
	}
	for _, o := range opts {
		o(u)
	}
	u.Checker = newChecker(toolID, current, u.Source, u.Notifier, u.Reporter, u.interval, u.applier, u.autoApply)
	u.Checker.logger = u.logger
	return u
}

// Start 启动周期检查，阻塞直到 ctx 取消或 Stop 调用。
func (u *Updater) Start(ctx context.Context) error {
	if u.Source == nil {
		return fmt.Errorf("versionup: Source is required (use WithSource)")
	}
	u.mu.Lock()
	if u.cancel != nil {
		u.mu.Unlock()
		return fmt.Errorf("versionup: already started")
	}
	cctx, cancel := context.WithCancel(ctx)
	u.cancel = cancel
	u.mu.Unlock()

	u.Checker.Start(cctx)
	return nil
}

// Stop 停止周期检查。
func (u *Updater) Stop() {
	u.mu.Lock()
	cancel := u.cancel
	u.cancel = nil
	u.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// CheckNow 立即执行一次检查。
func (u *Updater) CheckNow(ctx context.Context) (*UpdateEvent, error) {
	if u.Source == nil {
		return nil, fmt.Errorf("versionup: Source is required (use WithSource)")
	}
	return u.Checker.CheckOnce(ctx)
}

// OnUpdate 订阅升级通知（便捷方法）。
func (u *Updater) OnUpdate(obs Observer) {
	u.Notifier.Subscribe(obs)
}
