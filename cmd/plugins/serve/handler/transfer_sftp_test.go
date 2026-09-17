package handler

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInProcSFTPClient 在进程内起一个直接服务 os 文件系统的 sftp server，
// 免去真实 sshd 依赖即可验证 push/pull 的传输语义。
func newInProcSFTPClient(t *testing.T) *sftp.Client {
	t.Helper()
	c1, c2 := net.Pipe()
	srv, err := sftp.NewServer(c1)
	require.NoError(t, err)
	go func() { _ = srv.Serve() }()
	client, err := sftp.NewClientPipe(c2, c2)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
		_ = srv.Close()
		_ = c1.Close()
		_ = c2.Close()
	})
	return client
}

// 断点续传时目标已不小于源 → 应显式报告"跳过"，而不是伪装成一次成功拷贝。
func TestSftpPush_ResumeUpToDateSkips(t *testing.T) {
	client := newInProcSFTPClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("hello world"), 0o644))

	skipped, err := sftpPush(client, src, dst, transferOptions{Resume: true})
	require.NoError(t, err)
	assert.True(t, skipped, "目标不小于源时应报告 skipped")
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(got), "跳过时不应改写目标文件")
}

// 注意：断点续传的"部分文件续传后内容完整"无法在本 harness 验证——pkg/sftp
// 内置 server 把 SSH_FXF_APPEND 视为 no-op(server.go: "the client sends the
// offsets")，写入会从偏移 0 覆盖；生产路径对接的 OpenSSH sftp-server 则支持
// append。该语义由真实链路的 TestSFTPTransfer_ResumeE2E(localhost SSH)覆盖。

func TestSftpPush_FreshCopyNotSkipped(t *testing.T) {
	client := newInProcSFTPClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("fresh copy"), 0o644))

	skipped, err := sftpPush(client, src, dst, transferOptions{})
	require.NoError(t, err)
	assert.False(t, skipped, "全新拷贝不应报告 skipped")
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "fresh copy", string(got))
}

func TestSftpPush_ExistsNoOverwriteErrors(t *testing.T) {
	client := newInProcSFTPClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("xxx"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("existing"), 0o644))

	skipped, err := sftpPush(client, src, dst, transferOptions{})
	assert.Error(t, err, "目标已存在且未开覆盖/续传时应报错")
	assert.False(t, skipped)
}

// localhost 可用 SSH 时（与 transfer_test.go 的 E2E 同环境），用真实 sftp 链路验证：
// 远端存在"更大但内容已过期"的同名文件时，续传校验发现前缀不一致 → 全量重传，
// 让远端收敛到源文件内容（旧行为是直接跳过、保留过期内容）。
func TestSFTPTransfer_ResumeStaleLargerFreshCopyE2E(t *testing.T) {
	info := localhostNodeInfo(t)

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "f.txt")
	require.NoError(t, os.WriteFile(src, []byte("short"), 0644))

	remotePath := fmt.Sprintf("/tmp/owl_sftp_stale_%d.txt", os.Getpid())
	remoteCleanup(info, remotePath)
	defer remoteCleanup(info, remotePath)

	c, sc, err := dialSFTP(info)
	require.NoError(t, err)
	pf, err := c.Create(remotePath)
	require.NoError(t, err)
	_, err = pf.Write([]byte("much longer existing stale content"))
	require.NoError(t, err)
	pf.Close()
	c.Close()
	sc.Close()

	skipped, err := sftpTransfer(info, src, remotePath, "push", transferOptions{Resume: true})
	require.NoError(t, err)
	assert.False(t, skipped, "前缀不一致不得跳过")

	c2, sc2, err := dialSFTP(info)
	require.NoError(t, err)
	defer sc2.Close()
	defer c2.Close()
	rf, err := c2.Open(remotePath)
	require.NoError(t, err)
	data, _ := io.ReadAll(rf)
	rf.Close()
	assert.Equal(t, "short", string(data), "内容不一致时应全量重传收敛到源内容")
}

// runTransfer 依据 (err, skipped) 推导任务状态与输出——跳过必须如实呈现，不得伪装成成功拷贝。
func TestTransferOutcome(t *testing.T) {
	status, output := transferOutcome(nil, false, "/a/src.txt", "/b/")
	assert.Equal(t, store.TaskStatusCompleted, status)
	assert.Contains(t, output, "completed")

	status, output = transferOutcome(nil, true, "/a/src.txt", "/b/")
	assert.Equal(t, store.TaskStatusCompleted, status)
	assert.Contains(t, output, "skipped")
	assert.Contains(t, output, "up to date")

	status, output = transferOutcome(errors.New("ssh dial: boom"), false, "/a/src.txt", "/b/")
	assert.Equal(t, store.TaskStatusFailed, status)
	assert.Contains(t, output, "boom")
}
