package node

import (
	"bytes"
	"crypto/ed25519"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	gossh "golang.org/x/crypto/ssh"
)

// startCheckTestSSHServer 启动最简进程内 SSH server：接受密码 "pass" 或与
// allowedKey 匹配的公钥认证，认证后直接关闭连接。
func startCheckTestSSHServer(t *testing.T, allowedKey gossh.PublicKey) string {
	t.Helper()

	_, hostPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostKey, err := gossh.NewSignerFromSigner(hostPriv)
	if err != nil {
		t.Fatalf("create host signer: %v", err)
	}

	cfg := &gossh.ServerConfig{
		PasswordCallback: func(c gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			if string(pass) == "pass" {
				return nil, nil
			}
			return nil, errors.New("bad password")
		},
		PublicKeyCallback: func(c gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if allowedKey != nil && bytes.Equal(key.Marshal(), allowedKey.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("bad key")
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
				sconn, _, _, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					c.Close()
					return
				}
				sconn.Close()
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestCheckNodeSSH_ProbeMethods(t *testing.T) {
	_, clientPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientSigner, err := gossh.NewSignerFromSigner(clientPriv)
	if err != nil {
		t.Fatalf("create client signer: %v", err)
	}

	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	keyPEMBlock, err := gossh.MarshalPrivateKey(clientPriv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(keyPEMBlock), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	addr := startCheckTestSSHServer(t, clientSigner.PublicKey())
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	oldTimeout := checkTimeout
	checkTimeout = 5 * time.Second
	t.Cleanup(func() { checkTimeout = oldTimeout })

	t.Run("password success", func(t *testing.T) {
		r := checkNodeSSH(&common.NodeInfo{ID: "n1", Address: host, Port: port, User: "u", Password: "pass"})
		if !r.success || r.method != "password" {
			t.Fatalf("expected password success, got success=%v method=%q err=%v", r.success, r.method, r.err)
		}
	})

	t.Run("password failure", func(t *testing.T) {
		r := checkNodeSSH(&common.NodeInfo{ID: "n1", Address: host, Port: port, User: "u", Password: "wrong"})
		if r.success {
			t.Fatal("expected failure with wrong password")
		}
	})

	t.Run("key success", func(t *testing.T) {
		r := checkNodeSSH(&common.NodeInfo{ID: "n1", Address: host, Port: port, User: "u", SSHKey: keyFile})
		if !r.success || r.method != "key" {
			t.Fatalf("expected key success, got success=%v method=%q err=%v", r.success, r.method, r.err)
		}
	})

	t.Run("no credentials", func(t *testing.T) {
		r := checkNodeSSH(&common.NodeInfo{ID: "n1", Address: host, Port: port, User: "u"})
		if r.success || r.err == nil {
			t.Fatalf("expected no-auth failure, got %+v", r)
		}
	})
}
