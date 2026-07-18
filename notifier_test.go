package versionup

import (
	"sync"
	"testing"
	"time"
)

func TestNotifier_DefaultEnabled(t *testing.T) {
	n := NewNotifier()
	if !n.Enabled() {
		t.Error("expected default enabled")
	}
}

func TestNotifier_DisableBlocksDispatch(t *testing.T) {
	n := NewNotifier()
	n.Disable()
	if n.Enabled() {
		t.Error("expected disabled")
	}
	var called bool
	n.Subscribe(func(UpdateEvent) { called = true })
	n.Dispatch(UpdateEvent{ToolID: "demo", LatestVersion: "v2.0.0"})
	if called {
		t.Error("dispatch should be blocked when disabled")
	}
}

func TestNotifier_SubscribeReceives(t *testing.T) {
	n := NewNotifier()
	var got UpdateEvent
	var wg sync.WaitGroup
	wg.Add(1)
	n.Subscribe(func(e UpdateEvent) {
		got = e
		wg.Done()
	})
	n.Dispatch(UpdateEvent{ToolID: "demo", LatestVersion: "v2.0.0"})
	wg.Wait()
	if got.LatestVersion != "v2.0.0" {
		t.Errorf("got %q", got.LatestVersion)
	}
}

func TestNotifier_EnableReEnables(t *testing.T) {
	n := NewNotifier()
	n.Disable()
	var called int
	n.Subscribe(func(UpdateEvent) { called++ })
	n.Enable()
	n.Dispatch(UpdateEvent{ToolID: "demo"})
	if called != 1 {
		t.Errorf("expected 1 call after re-enable, got %d", called)
	}
}

func TestNotifier_NotifyAt(t *testing.T) {
	n := NewNotifier()
	var wg sync.WaitGroup
	wg.Add(1)
	n.Subscribe(func(UpdateEvent) { wg.Done() })
	n.NotifyAt(time.Now().Add(20*time.Millisecond), UpdateEvent{ToolID: "demo"})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("NotifyAt did not fire in time")
	}
}
