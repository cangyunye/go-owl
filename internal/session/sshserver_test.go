package session

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// startTestSSHServer 启动最简进程内 SSH server（密码 "pass"，exec 回 "ok"），
// allowForward=true 时支持 direct-tcpip 转发（可作跳板）。
// 模式与 internal/ssh/dial_test.go 的 startSSHServer 一致。
func startTestSSHServer(t *testing.T, allowForward bool) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostKey, err := gossh.NewSignerFromSigner(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	cfg := &gossh.ServerConfig{
		PasswordCallback: func(c gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			if string(pass) == "pass" {
				return nil, nil
			}
			return nil, errors.New("bad password")
		},
	}
	cfg.AddHostKey(hostKey)

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
