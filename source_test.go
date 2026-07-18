package versionup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPSource_Latest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tool") != "demo" {
			t.Errorf("unexpected tool %q", r.URL.Query().Get("tool"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tool_id":"demo","version":"v2.1.0","stable":true,"download_url":"https://x/demo.zip","checksum":"sha256:abc"}`))
	}))
	defer srv.Close()

	src := NewHTTPSource(srv.URL)
	rel, err := src.Latest(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "v2.1.0" || !rel.Stable || rel.DownloadURL != "https://x/demo.zip" {
		t.Errorf("unexpected release: %+v", rel)
	}
	if rel.Checksum != "sha256:abc" {
		t.Errorf("checksum = %q", rel.Checksum)
	}
}

func TestHTTPSource_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := NewHTTPSource(srv.URL)
	if _, err := src.Latest(context.Background(), "demo"); err == nil {
		t.Error("expected error on 500")
	}
}

func TestHTTPSource_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := NewHTTPSource(srv.URL, WithSourceClient(&http.Client{Timeout: 10 * time.Millisecond}))
	if _, err := src.Latest(context.Background(), "demo"); err == nil {
		t.Error("expected timeout error")
	}
}

func TestManifestSource_Map(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"demo":{"tool_id":"demo","version":"v3.0.0","stable":true}}`))
	}))
	defer srv.Close()

	src := NewManifestSource(srv.URL)
	rel, err := src.Latest(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "v3.0.0" {
		t.Errorf("version = %q", rel.Version)
	}
}

func TestManifestSource_Single(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tool_id":"demo","version":"v1.0.0","stable":true}`))
	}))
	defer srv.Close()

	src := NewManifestSource(srv.URL)
	rel, err := src.Latest(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Version != "v1.0.0" {
		t.Errorf("version = %q", rel.Version)
	}
}

func TestManifestSource_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"other":{"tool_id":"other","version":"v1.0.0"}}`))
	}))
	defer srv.Close()

	src := NewManifestSource(srv.URL)
	if _, err := src.Latest(context.Background(), "demo"); err == nil {
		t.Error("expected not-found error")
	}
}
