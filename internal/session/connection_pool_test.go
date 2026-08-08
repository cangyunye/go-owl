package session

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func testPool() *SSHConnectionPool {
	return NewSSHConnectionPool(&PoolConfig{
		MaxConnections: 10,
		ConnectTimeout: 5 * time.Second,
		IdleTimeout:    time.Minute,
	})
}

func splitHostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port %q: %v", portStr, err)
	}
	return host, port
}

func TestPoolConnect_SuccessAndReuse(t *testing.T) {
	addr := startTestSSHServer(t, false)
	host, port := splitHostPort(t, addr)
	auth := []gossh.AuthMethod{gossh.Password("pass")}

	pool := testPool()
	defer pool.CloseAll()

	conn1, err := pool.Connect("n1", host, port, "u", auth, "")
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	conn2, err := pool.Connect("n1", host, port, "u", auth, "")
	if err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	if conn2 != conn1 {
		t.Fatal("expected existing connection to be reused")
	}
	if !conn1.IsConnected() {
		t.Fatal("connection should be connected")
	}

	code, out, err := conn1.Execute("echo hi", 5*time.Second)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if code != 0 || out != "ok" {
		t.Fatalf("expected exit 0 output %q, got %d %q", "ok", code, out)
	}
}

func TestPoolConnect_ProxyJumpEndToEnd(t *testing.T) {
	target := startTestSSHServer(t, false)
	jump := startTestSSHServer(t, true)
	host, port := splitHostPort(t, target)
	auth := []gossh.AuthMethod{gossh.Password("pass")}

	pool := testPool()
	defer pool.CloseAll()

	conn, err := pool.Connect("n1", host, port, "u", auth, jump)
	if err != nil {
		t.Fatalf("Connect via jump: %v", err)
	}

	code, out, err := conn.Execute("echo hi", 5*time.Second)
	if err != nil {
		t.Fatalf("Execute via jump: %v", err)
	}
	if code != 0 || out != "ok" {
		t.Fatalf("expected exit 0 output %q, got %d %q", "ok", code, out)
	}
}

func TestPoolConnect_ProxyJumpFailurePropagated(t *testing.T) {
	target := startTestSSHServer(t, false)
	host, port := splitHostPort(t, target)
	auth := []gossh.AuthMethod{gossh.Password("pass")}

	pool := testPool()
	defer pool.CloseAll()

	// 跳板不可达：错误必须体现 ProxyJump 参数确实被消费（走了跳板路径）
	_, err := pool.Connect("n1", host, port, "u", auth, "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error when jump host unreachable")
	}
	if !strings.Contains(err.Error(), "跳板机") {
		t.Fatalf("expected jump host error, got: %v", err)
	}
}

func TestPoolConnect_AuthFailed(t *testing.T) {
	addr := startTestSSHServer(t, false)
	host, port := splitHostPort(t, addr)
	auth := []gossh.AuthMethod{gossh.Password("wrong")}

	pool := testPool()
	defer pool.CloseAll()

	if _, err := pool.Connect("n1", host, port, "u", auth, ""); err == nil {
		t.Fatal("expected auth failure with wrong password")
	}
}
