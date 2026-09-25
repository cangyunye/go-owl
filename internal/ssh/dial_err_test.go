package ssh

import (
	"errors"
	"strings"
	"testing"
)

// 认证失败的报错必须能看出「用的是哪个账号」和「该查什么」。
// 线上案例：节点 user 配成 root（树莓派 sshd 默认禁止 root 密码登录），
// 原报错只有 "unable to authenticate, attempted methods [none password]"，
// 既没有账号也不给排查方向，只能靠外部探针才知道是账号配错。
func TestConnErr_AuthFailureMentionsUserAndHint(t *testing.T) {
	cause := errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain")
	err := connErr("192.168.31.100:22", "root", cause)

	if err.ErrorType != ErrorTypeAuth {
		t.Fatalf("应归类为认证错误，实际 %v", err.ErrorType)
	}
	msg := err.Error()
	for _, want := range []string{"192.168.31.100:22", "user=root", "认证失败"} {
		if !strings.Contains(msg, want) {
			t.Errorf("报错应包含 %q，实际: %s", want, msg)
		}
	}
}

// 连接类错误（拒绝连接/超时）不应带认证提示，避免误导排查方向；用户名照常带上
func TestConnErr_ConnFailureHasNoAuthHint(t *testing.T) {
	err := connErr("10.0.0.1:22", "kali", errors.New("dial tcp 10.0.0.1:22: connect: connection refused"))

	if err.ErrorType != ErrorTypeConnection {
		t.Fatalf("应归类为连接错误，实际 %v", err.ErrorType)
	}
	msg := err.Error()
	if strings.Contains(msg, "认证失败") {
		t.Errorf("连接类错误不应出现认证提示: %s", msg)
	}
	if !strings.Contains(msg, "user=kali") {
		t.Errorf("报错应带用户名: %s", msg)
	}
}

// 未提供用户名时不应渲染空的 user= 片段
func TestConnErr_EmptyUserOmitsField(t *testing.T) {
	err := connErr("10.0.0.1:22", "", errors.New("i/o timeout"))
	if strings.Contains(err.Error(), "user=") {
		t.Errorf("无用户名时不应出现 user= 片段: %s", err.Error())
	}
}
