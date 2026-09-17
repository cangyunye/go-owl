package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 断点续传的一致性：远端已存在的文件未必是本源文件的中途副本（可能是
// 同名异容的旧文件/更新前的版本）。盲目从远端长度处追加会拼出"大小一致
// 但内容损坏"的文件（用户实测：xml 无法解析、jar 无法 jar -tf）；
// 盲目跳过（远端 >= 源）会让远端停留在过期内容上。两者都必须避免：
// 前缀一致才续传/跳过，不一致就丢弃远端全量重传。
func TestSftpPush_ResumeMismatchedPrefixFreshCopy(t *testing.T) {
	client := newInProcSFTPClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.xml")
	dst := filepath.Join(dir, "dst.xml")
	require.NoError(t, os.WriteFile(src, []byte("0123456789ABCDEF"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("XXXXOLDContent"), 0o644)) // 同名异容且更小

	skipped, err := sftpPush(client, src, dst, transferOptions{Resume: true})
	require.NoError(t, err)
	assert.False(t, skipped, "内容不一致不得视为已最新")

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "0123456789ABCDEF", string(got), "前缀不一致应全量重传，而不是拼接出损坏文件")
}

// 远端与源同尺寸但内容不同：不得当作"已是最新"跳过
func TestSftpPush_ResumeSameSizeDifferentContentFreshCopy(t *testing.T) {
	client := newInProcSFTPClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.xml")
	dst := filepath.Join(dir, "dst.xml")
	require.NoError(t, os.WriteFile(src, []byte("0123456789"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("9876543210"), 0o644)) // 同尺寸异容

	skipped, err := sftpPush(client, src, dst, transferOptions{Resume: true})
	require.NoError(t, err)
	assert.False(t, skipped, "同尺寸异容不得跳过")

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "0123456789", string(got))
}

// 真续传（远端是源文件的前缀）语义保持：追加剩余部分
// pull 方向同样校验
func TestSftpPull_ResumeMismatchedPrefixFreshCopy(t *testing.T) {
	client := newInProcSFTPClient(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "remote.xml") // "远端"文件（进程内 sftp 服务的 os 视角）
	dst := filepath.Join(dir, "local.xml")
	require.NoError(t, os.WriteFile(src, []byte("0123456789ABCDEF"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("XXXXOLD"), 0o644)) // 同名异容且更小

	skipped, err := sftpPull(client, src, dst, transferOptions{Resume: true})
	require.NoError(t, err)
	assert.False(t, skipped)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "0123456789ABCDEF", string(got))
}
