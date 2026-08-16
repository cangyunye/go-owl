package ssh

import (
	"net"
	"testing"
	"time"
)

// TestNativeNodeExecutor_ExecuteWithConfig_BoundsDialByConnectTimeout
// 验证连接超时独立生效：面对只接受 TCP 连接、从不响应 SSH 握手的主机，
// ExecuteWithConfig 应在 ConnectTimeout 附近返回，而不是按
// ConnectTimeout + CommandTimeout（旧逻辑把总和当拨号超时）长时间挂起。
func TestNativeNodeExecutor_ExecuteWithConfig_BoundsDialByConnectTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// 接受连接但保持静默，模拟慢/挂起主机。
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// 不回送任何字节；测试结束时随进程回收。
			_ = conn
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	exec := NewNativeNodeExecutor(&ConnectionInfo{
		User:    "test",
		Address: "127.0.0.1",
		Port:    port,
	})

	connectTimeout := 2 * time.Second
	commandTimeout := 30 * time.Second

	start := time.Now()
	_, _, err = exec.ExecuteWithConfig("echo hi", &TimeoutConfig{
		ConnectTimeout: connectTimeout,
		CommandTimeout: commandTimeout,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected handshake timeout error, got nil")
	}
	// 修复后应约 ConnectTimeout(2s) 返回；旧逻辑会用 connect+command(32s) 当拨号超时。
	if elapsed > connectTimeout*3 {
		t.Errorf("ExecuteWithConfig took %v; dial must be bounded by ConnectTimeout %v", elapsed, connectTimeout)
	}
}

// TestNativeNodeExecutor_ExecuteWithConfig_NilConfigUsesDefaults 验证 nil 配置回退默认值。
func TestNativeNodeExecutor_ExecuteWithConfig_NilConfigUsesDefaults(t *testing.T) {
	exec := NewNativeNodeExecutor(&ConnectionInfo{User: "test", Address: "127.0.0.1", Port: 1})
	start := time.Now()
	_, _, err := exec.ExecuteWithConfig("echo hi", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected connection error to localhost:1")
	}
	// nil 配置应使用默认 ConnectTimeout=10s，不应超过 30s。
	if elapsed > 30*time.Second {
		t.Errorf("nil config took %v, too long", elapsed)
	}
}
