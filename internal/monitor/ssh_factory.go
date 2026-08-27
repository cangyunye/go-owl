package monitor

import (
	"github.com/cangyunye/go-owl/internal/ssh"
)

// SSHExecerFactory 基于 internal/ssh 的默认执行器工厂：
// 复用节点 SSH 凭据与连接解析（~/.ssh/config、密钥优先、密码兜底、跳板机）。
type SSHExecerFactory struct {
	factory *ssh.NodeExecutorFactory
}

// NewSSHExecerFactory 创建 SSH 执行器工厂。
func NewSSHExecerFactory() *SSHExecerFactory {
	return &SSHExecerFactory{factory: ssh.NewNodeExecutorFactory()}
}

// NewExecer 按采集目标创建执行器。Target 为空地址时走本地执行器路径。
func (f *SSHExecerFactory) NewExecer(t *Target) (Execer, error) {
	return f.factory.GetExecutorForNode(
		t.ID, t.Address, t.Port, t.User, t.SSHKey, t.SSHPassword, t.ProxyJump)
}

// 编译期断言：SSH 执行器满足 monitor 的 Execer 契约。
var _ Execer = (ssh.NodeExecutor)(nil)
var _ ExecerFactory = (*SSHExecerFactory)(nil)
