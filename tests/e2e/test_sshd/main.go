// test_sshd 是 E2E 专用的最小 SSH 服务：接受任意密码登录，
// exec 请求用 /bin/sh -c 真实执行命令并回传合并输出与退出码。
// 用途：owl-serve 剧本执行的端到端验证需要真实 SSH 链路，
// 但不依赖外部机器。用法：go run ./tests/e2e/test_sshd -port 2222
package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"net"
	"os/exec"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func main() {
	port := flag.Int("port", 2222, "listen port")
	flag.Parse()

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Fatal(err)
	}
	signer, err := gossh.NewSignerFromSigner(priv)
	if err != nil {
		log.Fatal(err)
	}

	cfg := &gossh.ServerConfig{
		PasswordCallback: func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			log.Printf("login user=%q", conn.User())
			return &gossh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("test sshd listening on %s", ln.Addr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConn(conn, cfg)
	}
}

func handleConn(c net.Conn, cfg *gossh.ServerConfig) {
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
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go handleSession(ch, chReqs)
	}
}

func handleSession(ch gossh.Channel, chReqs <-chan *gossh.Request) {
	defer ch.Close()
	for req := range chReqs {
		if req.Type != "exec" {
			req.Reply(false, nil)
			continue
		}
		// payload: 4 字节长度 + 命令
		if len(req.Payload) < 4 {
			req.Reply(false, nil)
			return
		}
		n := int(req.Payload[0])<<24 | int(req.Payload[1])<<16 | int(req.Payload[2])<<8 | int(req.Payload[3])
		if n > len(req.Payload)-4 {
			n = len(req.Payload) - 4
		}
		command := string(req.Payload[4 : 4+n])
		req.Reply(true, nil)

		log.Printf("exec %q", command)
		cmd := exec.Command("/bin/sh", "-c", command)
		output, err := cmd.CombinedOutput()
		if len(output) > 0 {
			if _, err := ch.Write(output); err != nil {
				return
			}
		}
		exitCode := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			} else {
				exitCode = 127
			}
		}
		if _, err := ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Code uint32 }{uint32(exitCode)})); err != nil {
			return
		}
		// 给输出留一点冲刷时间后关闭
		go func() {
			time.Sleep(100 * time.Millisecond)
			ch.Close()
		}()
		return
	}
}
