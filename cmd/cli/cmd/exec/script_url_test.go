package exec

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestFetchScriptFromURL：URL 拉取脚本必须校验状态码、带超时、限制大小。
func TestFetchScriptFromURL(t *testing.T) {
	t.Run("200 返回内容", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("echo ok"))
		}))
		defer srv.Close()
		got, err := fetchScriptFromURL(srv.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "echo ok" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("404 必须报错", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer srv.Close()
		_, err := fetchScriptFromURL(srv.URL)
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Fatalf("error should mention status, got %v", err)
		}
	})

	t.Run("超时报错", func(t *testing.T) {
		old := scriptHTTPClient
		scriptHTTPClient = &http.Client{Timeout: 50 * time.Millisecond}
		defer func() { scriptHTTPClient = old }()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
		}))
		defer srv.Close()
		if _, err := fetchScriptFromURL(srv.URL); err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})

	t.Run("超过大小上限报错", func(t *testing.T) {
		old := maxScriptURLBytes
		maxScriptURLBytes = 8
		defer func() { maxScriptURLBytes = old }()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("123456789"))
		}))
		defer srv.Close()
		if _, err := fetchScriptFromURL(srv.URL); err == nil {
			t.Fatal("expected size-limit error, got nil")
		}
	})
}
