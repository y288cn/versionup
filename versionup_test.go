package versionup

import (
	"context"
	"testing"
	"time"
)

type fakeSource struct {
	rel *Release
	err error
}

func (f *fakeSource) Latest(ctx context.Context, toolID string) (*Release, error) {
	return f.rel, f.err
}

type fakeReporter struct {
	got *UpdateEvent
}

func (f *fakeReporter) Report(ctx context.Context, e UpdateEvent) error {
	f.got = &e
	return nil
}

func TestUpdater_CheckNow_Upgrade(t *testing.T) {
	rel := &Release{ToolID: "demo", Version: "v2.0.0", Stable: true, DownloadURL: "http://x/v2.zip"}
	src := &fakeSource{rel: rel}
	rep := &fakeReporter{}

	var dispatched UpdateEvent
	var gotDispatch bool

	up := New("demo", "v1.0.0", WithSource(src), WithReporter(rep))
	up.OnUpdate(func(e UpdateEvent) { dispatched = e; gotDispatch = true })

	ev, err := up.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected upgrade event, got nil")
	}
	if ev.LatestVersion != "v2.0.0" {
		t.Errorf("LatestVersion = %q, want v2.0.0", ev.LatestVersion)
	}
	if !gotDispatch {
		t.Error("expected dispatch to subscriber")
	}
	if dispatched.LatestVersion != "v2.0.0" {
		t.Errorf("dispatched LatestVersion = %q, want v2.0.0", dispatched.LatestVersion)
	}
	if rep.got == nil {
		t.Error("expected reporter to be called")
	}
}

func TestUpdater_CheckNow_NoUpgrade(t *testing.T) {
	rel := &Release{ToolID: "demo", Version: "v1.0.0", Stable: true}
	src := &fakeSource{rel: rel}
	up := New("demo", "v1.0.0", WithSource(src))

	ev, err := up.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow error: %v", err)
	}
	if ev != nil {
		t.Errorf("expected no upgrade event, got %+v", ev)
	}
}

func TestUpdater_CheckNow_Unstable(t *testing.T) {
	rel := &Release{ToolID: "demo", Version: "v2.0.0", Stable: false}
	src := &fakeSource{rel: rel}
	up := New("demo", "v1.0.0", WithSource(src))

	ev, err := up.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("CheckNow error: %v", err)
	}
	if ev != nil {
		t.Error("unstable release should not trigger upgrade")
	}
}

func TestUpdater_StartStop(t *testing.T) {
	rel := &Release{ToolID: "demo", Version: "v2.0.0", Stable: true}
	src := &fakeSource{rel: rel}
	up := New("demo", "v1.0.0", WithSource(src), WithInterval(time.Hour))

	ctx := context.Background()
	done := make(chan struct{})
	go func() { _ = up.Start(ctx); close(done) }()

	time.Sleep(50 * time.Millisecond)
	up.Stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after Stop")
	}
}
