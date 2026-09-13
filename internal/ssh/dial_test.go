package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func genHostKey(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate ed25519 host key: %v", err)
	}
	signer, err := gossh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("create signer from host key: %v", err)
	}
	return signer
}

// startSSHServerCfg 启动一个使用给定 ServerConfig 的最简 SSH server，
// 对 exec 请求回 "ok"；allowForward=true 时支持 direct-tcpip 转发。
func startSSHServerCfg(t *testing.T, cfg *gossh.ServerConfig, allowForward bool) string {
	t.Helper()
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
					switch newChan.ChannelType() {
					case "session":
						ch, chReqs, err := newChan.Accept()
						if err != nil {
							continue
						}
						go func() {
							for req := range chReqs {
								if req.Type == "exec" {
									req.Reply(true, nil)
									ch.Write([]byte("ok"))
									ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
									ch.Close()
									return
								}
								req.Reply(false, nil)
							}
						}()
					case "direct-tcpip":
						if !allowForward {
							newChan.Reject(gossh.Prohibited, "forwarding disabled")
							continue
						}
						// direct-tcpip payload: addr(4+n) port(4) srcAddr(4+n) srcPort(4)
						raw := newChan.ExtraData()
						if len(raw) < 4 {
							newChan.Reject(gossh.ConnectionFailed, "bad payload")
							continue
						}
						addrLen := binary.BigEndian.Uint32(raw)
						if uint32(len(raw)) < 4+addrLen+4 {
							newChan.Reject(gossh.ConnectionFailed, "bad payload")
							continue
						}
						destAddr := string(raw[4 : 4+addrLen])
						destPort := binary.BigEndian.Uint32(raw[4+addrLen : 8+addrLen])
						ch, reqs, err := newChan.Accept()
						if err != nil {
							continue
						}
						go gossh.DiscardRequests(reqs)
						go func() {
							dst, err := net.Dial("tcp", net.JoinHostPort(destAddr, strconv.Itoa(int(destPort))))
							if err != nil {
								ch.Close()
								return
							}
							go io.Copy(dst, ch)
							io.Copy(ch, dst)
						}()
					default:
						newChan.Reject(gossh.UnknownChannelType, "unsupported")
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func startSSHServer(t *testing.T, allowForward bool) string {
	t.Helper()
	cfg := &gossh.ServerConfig{
		PasswordCallback: func(c gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			if string(pass) == "pass" {
				return nil, nil
			}
			return nil, errors.New("bad password")
		},
	}
	return startSSHServerCfg(t, cfg, allowForward)
}

func TestDial_Basic(t *testing.T) {
	addr := startSSHServer(t, false)
	client, err := Dial(context.Background(), addr, DialOptions{
		User: "u", Password: "pass", ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()
	out, err := session.CombinedOutput("uptime")
	if err != nil {
		t.Fatalf("CombinedOutput: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("expected output %q, got %q", "ok", string(out))
	}
}

func TestDial_AuthFailed(t *testing.T) {
	addr := startSSHServer(t, false)
	_, err := Dial(context.Background(), addr, DialOptions{
		User: "u", Password: "wrong", ConnectTimeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestDial_NoAuthMethods(t *testing.T) {
	// 隔离 HOME/USERPROFILE（os.UserHomeDir 在 Windows 走 USERPROFILE），
	// 避免 tryDefaultKeys 加载本机 ~/.ssh 密钥导致测试不确定
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	addr := startSSHServer(t, false)
	_, err := Dial(context.Background(), addr, DialOptions{
		User: "u", ConnectTimeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error when no auth methods")
	}
	var authErr *SSHAuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected *SSHAuthError, got %T: %v", err, err)
	}
}

func TestDial_ProxyJump(t *testing.T) {
	target := startSSHServer(t, false)
	jump := startSSHServer(t, true) // 跳板需允许 direct-tcpip

	client, err := Dial(context.Background(), target, DialOptions{
		User: "u", Password: "pass", ProxyJump: jump, ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial via jump: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()
	out, err := session.CombinedOutput("uptime")
	if err != nil {
		t.Fatalf("CombinedOutput: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("expected output %q, got %q", "ok", string(out))
	}
}

func TestDial_ConnectTimeout(t *testing.T) {
	// 不可达地址应在超时内返回错误而不是无限等待
	_, err := Dial(context.Background(), "10.255.255.1:22", DialOptions{
		User: "u", Password: "pass", ConnectTimeout: 500 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// startSilentTCPServer 启动一个只接受 TCP 连接、不发送任何数据也不关闭连接的
// 假 server（模拟 sshd MaxStartups tarpit：TCP 连上但 SSH banner 永远不来）。
func startSilentTCPServer(t *testing.T) string {
	t.Helper()
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
				buf := make([]byte, 256)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestDial_HandshakeTimeout_NoBanner(t *testing.T) {
	// TCP 可达但对端不发 SSH banner：Dial 应在约 ConnectTimeout 后返回错误而非挂起
	addr := startSilentTCPServer(t)
	start := time.Now()
	_, err := Dial(context.Background(), addr, DialOptions{
		User: "u", Password: "pass", ConnectTimeout: 500 * time.Millisecond,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected handshake timeout error, got nil")
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("Dial returned too fast (%v), handshake timeout likely not engaged", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Dial hung too long: %v", elapsed)
	}
}

func TestDial_ProxyJump_HandshakeTimeout_NoBanner(t *testing.T) {
	// 经跳板转发到不发 banner 的目标：跳板转发路径同样受握手超时约束
	jump := startSSHServer(t, true)
	silent := startSilentTCPServer(t)
	start := time.Now()
	_, err := Dial(context.Background(), silent, DialOptions{
		User: "u", Password: "pass", ProxyJump: jump, ConnectTimeout: 500 * time.Millisecond,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected handshake timeout error, got nil")
	}
	if elapsed < 400*time.Millisecond {
		t.Fatalf("Dial returned too fast (%v), handshake timeout likely not engaged", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Dial hung too long: %v", elapsed)
	}
}

func TestDial_HandshakeCtxDeadline(t *testing.T) {
	// ctx deadline 早于 ConnectTimeout 时应取更早者
	addr := startSilentTCPServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Dial(ctx, addr, DialOptions{
		User: "u", Password: "pass", ConnectTimeout: 5 * time.Second,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error when ctx deadline expires during handshake")
	}
	if elapsed < 250*time.Millisecond {
		t.Fatalf("Dial returned too fast (%v), ctx deadline likely not engaged", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("Dial hung too long: %v", elapsed)
	}
}

func TestDial_AuthMethodsOverride(t *testing.T) {
	// AuthMethods 非空时直接使用，跳过内建认证链（无需隔离 HOME）
	addr := startSSHServer(t, false)
	client, err := Dial(context.Background(), addr, DialOptions{
		User:           "u",
		AuthMethods:    []gossh.AuthMethod{gossh.Password("pass")},
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial with AuthMethods: %v", err)
	}
	defer client.Close()
}

func TestDial_AuthMethodsOverride_WrongPassword(t *testing.T) {
	addr := startSSHServer(t, false)
	_, err := Dial(context.Background(), addr, DialOptions{
		User:           "u",
		AuthMethods:    []gossh.AuthMethod{gossh.Password("wrong")},
		ConnectTimeout: 5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected auth failure with wrong password in AuthMethods")
	}
}

func TestDial_AuthMethodsOverride_ProxyJump(t *testing.T) {
	// 经跳板拨号时 AuthMethods 必须同样传递到跳板连接
	target := startSSHServer(t, false)
	jump := startSSHServer(t, true)

	client, err := Dial(context.Background(), target, DialOptions{
		User:           "u",
		AuthMethods:    []gossh.AuthMethod{gossh.Password("pass")},
		ProxyJump:      jump,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial via jump with AuthMethods: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()
	out, err := session.CombinedOutput("uptime")
	if err != nil {
		t.Fatalf("CombinedOutput: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("expected output %q, got %q", "ok", string(out))
	}
}

func TestDial_ProxyJump_NoPortDefaultsTo22(t *testing.T) {
	// 隔离 HOME，避免本机 ~/.ssh 默认密钥意外对 127.0.0.1:22 认证成功
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	// 验证 ProxyJump 不带端口时补 :22（22 端口无权限认证，应失败而非 panic）
	target := startSSHServer(t, false)
	_, err := Dial(context.Background(), target, DialOptions{
		User: "u", Password: "pass", ProxyJump: "127.0.0.1", ConnectTimeout: 1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected jump dial failure when port 22 unreachable")
	}
}

func TestSplitJumpSpec(t *testing.T) {
	cases := []struct{ spec, wantUser, wantAddr string }{
		{"host", "", "host"},
		{"host:2222", "", "host:2222"},
		{"10.0.0.1", "", "10.0.0.1"},
		{"root@host", "root", "host"},
		{"root@host:2222", "root", "host:2222"},
		{"root@10.0.0.1", "root", "10.0.0.1"},
	}
	for _, c := range cases {
		user, addr := splitJumpSpec(c.spec)
		if user != c.wantUser || addr != c.wantAddr {
			t.Fatalf("splitJumpSpec(%q) = (%q, %q), want (%q, %q)",
				c.spec, user, addr, c.wantUser, c.wantAddr)
		}
	}
}

// TestDial_ProxyJump_IndependentUserAndKeys 验证"跳板密钥 + 目标密码"组合：
// proxy_jump 支持 user@ 前缀指定跳板用户，跳板认证在目标认证链之外追加
// 本机默认密钥（目标仅密码时，默认密钥不会进入跳板认证链，跳板若只收
// 公钥将无法登录——这是修复前的缺口）。
func TestDial_ProxyJump_IndependentUserAndKeys(t *testing.T) {
	// 隔离 HOME/USERPROFILE（Windows 用 USERPROFILE 解析默认密钥目录）
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// 生成本机默认密钥 ~/.ssh/id_ed25519
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	block, err := gossh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}
	keyPath := filepath.Join(home, ".ssh", "id_ed25519")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write client key: %v", err)
	}
	authorized, err := gossh.NewPublicKey(priv.Public())
	if err != nil {
		t.Fatal(err)
	}

	// 跳板只接受 jumpuser + 上述公钥（公钥认证），目标接受 u/pass（密码认证）
	jumpCfg := &gossh.ServerConfig{
		PublicKeyCallback: func(c gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if c.User() == "jumpuser" && bytes.Equal(key.Marshal(), authorized.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("denied")
		},
	}
	jump := startSSHServerCfg(t, jumpCfg, true)
	target := startSSHServer(t, false)

	client, err := Dial(context.Background(), target, DialOptions{
		User:           "u",
		Password:       "pass",
		ProxyJump:      "jumpuser@" + jump,
		ConnectTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Dial via jump with independent credentials: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()
	out, err := session.CombinedOutput("uptime")
	if err != nil {
		t.Fatalf("CombinedOutput: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("expected output %q, got %q", "ok", string(out))
	}
}
