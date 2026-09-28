package ssh

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// startHangingExecServer 起 exec 请求后永久挂死的进程内 SSH server，
// 模拟远端命令阻塞（交互输入、nohup 守护等）。
func startHangingExecServer(t *testing.T) string {
	t.Helper()
	cfg := &gossh.ServerConfig{PasswordCallback: func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
		return &gossh.Permissions{}, nil
	}}
	cfg.AddHostKey(genHostKey(t))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sconn, chans, reqs, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					c.Close()
					return
				}
				defer sconn.Close()
				go gossh.DiscardRequests(reqs)
				for newChan := range chans {
					if newChan.ChannelType() != "session" {
						newChan.Reject(gossh.UnknownChannelType, "unsupported")
						continue
					}
					_, chReqs, err := newChan.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range chReqs {
							if req.Type == "exec" {
								req.Reply(true, nil)
								// 收到 exec 后什么都不做：远端命令挂死
								return
							}
							req.Reply(false, nil)
						}
					}()
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func newHangingExecutor(t *testing.T) *NativeNodeExecutor {
	t.Helper()
	addr := startHangingExecServer(t)
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return NewNativeNodeExecutor(&ConnectionInfo{
		Address:  host,
		Port:     port,
		User:     "test",
		Password: "unused",
	})
}

// 运行中取消必须真实终止脚本执行：ExecuteContext 只发 SIGTERM 不等
// 断连的旧实现对 nohup/忽略 SIGHUP 的远端进程无效，取消后连接必须断开。
func TestNativeExecutor_ExecuteContext_CancelTerminates(t *testing.T) {
	exec := newHangingExecutor(t)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, _, err := exec.ExecuteContext(ctx, "hang-forever", 10*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("取消后 ExecuteContext 应返回错误")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("取消后应立即返回（命令超时 10s 兜底前），实际 %v", elapsed)
	}
}

// 预先取消的 ctx 不应再发起 SSH 拨号/执行，立即返回取消错误。
func TestNativeExecutor_ExecuteContext_PreCancelled(t *testing.T) {
	exec := newHangingExecutor(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, _, err := exec.ExecuteContext(ctx, "hang-forever", 10*time.Second)

	if err == nil {
		t.Fatal("预取消 ctx 应返回错误")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("预取消 ctx 应立即返回，不得发起连接")
	}
}
