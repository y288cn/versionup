package versionup

import (
	"sync"
	"time"
)

// Observer 事件观察者（通知订阅者）。
type Observer func(UpdateEvent)

// Notifier 通知管理器：控制通知开关与定时。
type Notifier interface {
	Enable()                        // 开启事件通知
	Disable()                       // 关闭事件通知
	Enabled() bool                  // 查询当前是否开启
	Subscribe(obs Observer)         // 订阅升级通知
	NotifyAt(t time.Time, e UpdateEvent) // 指定时间投递一次通知
	Dispatch(e UpdateEvent)         // 若开启则广播给订阅者
}

type defaultNotifier struct {
	mu        sync.RWMutex
	enabled   bool
	observers []Observer
}

// NewNotifier 创建默认通知管理器（默认开启通知）。
func NewNotifier() Notifier {
	return &defaultNotifier{enabled: true}
}

func (n *defaultNotifier) Enable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled = true
}

func (n *defaultNotifier) Disable() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.enabled = false
}

func (n *defaultNotifier) Enabled() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.enabled
}

func (n *defaultNotifier) Subscribe(obs Observer) {
	if obs == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.observers = append(n.observers, obs)
}

// NotifyAt 在指定时间 t 投递一次通知（到时调用 Dispatch，受当时开关状态约束）。
func (n *defaultNotifier) NotifyAt(t time.Time, e UpdateEvent) {
	d := time.Until(t)
	if d < 0 {
		d = 0
	}
	time.AfterFunc(d, func() { n.Dispatch(e) })
}

// Dispatch 若通知开启，则并发调用所有订阅者。
func (n *defaultNotifier) Dispatch(e UpdateEvent) {
	n.mu.RLock()
	enabled := n.enabled
	obs := make([]Observer, len(n.observers))
	copy(obs, n.observers)
	n.mu.RUnlock()

	if !enabled {
		return
	}
	for _, o := range obs {
		o(e)
	}
}
