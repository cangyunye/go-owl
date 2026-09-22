package node

import (
	"net"
	"strconv"
	"testing"
	"time"
)

// TestPingNodeTCP_Reachable：对真实监听端口探测应成功并返回延迟样本。
func TestPingNodeTCP_Reachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	host := addr.IP.String()
	port := addr.Port

	res := pingNodeTCP(host, port, 2*time.Second, 3)
	if !res.reachable {
		t.Fatal("expected reachable")
	}
	if len(res.latencies) != 3 {
		t.Fatalf("expected 3 latency samples, got %d", len(res.latencies))
	}
}

// TestPingNodeTCP_Unreachable：对无监听端口探测应失败且无样本。
func TestPingNodeTCP_Unreachable(t *testing.T) {
	// 占用后立即关闭的端口，通常无监听
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	res := pingNodeTCP("127.0.0.1", port, 500*time.Millisecond, 1)
	if res.reachable {
		t.Fatal("expected unreachable")
	}
	if len(res.latencies) != 0 {
		t.Fatalf("expected no samples, got %d", len(res.latencies))
	}
	_ = strconv.Itoa(port)
}
