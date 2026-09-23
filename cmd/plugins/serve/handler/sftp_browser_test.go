package handler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// sftpBrowserTestSetup 构造带 writer 组路由的测试环境，并返回 handler 以便
// 替换 openSFTP 拨号缝（注入进程内 sftp server，绕开真实 sshd）。
func sftpBrowserTestSetup(t *testing.T) (*SFTPBrowserHandler, *gin.Engine, string, string, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS nodes (
		id TEXT PRIMARY KEY, name TEXT, address TEXT, port INTEGER DEFAULT 22,
		user TEXT, password TEXT, ssh_key TEXT, status TEXT DEFAULT 'unknown',
		groups TEXT DEFAULT '[]', labels TEXT DEFAULT '{}',
		proxy_jump TEXT DEFAULT '', created_at TIMESTAMP, updated_at TIMESTAMP
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO nodes (id, address, port, user) VALUES ('node-1', '10.0.0.1', 22, 'test')`)
	require.NoError(t, err)

	us := store.NewUserStore(db)
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	ah := NewAuthHandler(us, as)

	h := NewSFTPBrowserHandler(db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(ah.AuthMiddleware(), ah.RBACMiddleware(model.RoleEditor, model.RoleOperator, model.RoleAdmin))
	auth.GET("/sftp/ls", h.List)
	auth.GET("/sftp/stat", h.Stat)
	auth.GET("/sftp/file", h.Download)
	auth.PUT("/sftp/file", h.Upload)
	auth.GET("/sftp/archive", h.Archive)
	auth.POST("/sftp/mkdir", h.Mkdir)
	auth.POST("/sftp/rename", h.Rename)
	auth.POST("/sftp/delete", h.Delete)

	editorToken, _ := as.GenerateToken("editor", "editor")
	viewerToken, _ := as.GenerateToken("viewer", "viewer")
	return h, r, editorToken, viewerToken, db
}

func doJSON(t *testing.T, router *gin.Engine, method, target, token string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req, _ = http.NewRequest(method, target, nil)
	} else {
		buf, err := json.Marshal(body)
		require.NoError(t, err)
		req, _ = http.NewRequest(method, target, bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// injectInProcSFTP 把 handler 的拨号缝替换为进程内 sftp server（服务真实 OS 文件系统）。
func injectInProcSFTP(t *testing.T, h *SFTPBrowserHandler) {
	t.Helper()
	h.openSFTP = func(info *nodeSSHInfo) (*sftp.Client, func() error, error) {
		client := newInProcSFTPClient(t)
		return client, func() error { return nil }, nil
	}
}

// GET /sftp/ls 列出远端目录条目：名称/完整路径/类型/大小/修改时间。
func TestSFTPBrowserList_ListsRemoteEntries(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte("hello world"), 0o644))

	w := doJSON(t, router, "GET", "/api/v1/sftp/ls?node_id=node-1&path="+url.QueryEscape(dir), token, nil)
	require.Equal(t, 200, w.Code, w.Body.String())

	var resp struct {
		Path  string `json:"path"`
		Items []struct {
			Name  string `json:"name"`
			Path  string `json:"path"`
			IsDir bool   `json:"is_dir"`
			Size  int64  `json:"size"`
			MTime int64  `json:"mtime"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, dir, resp.Path)
	require.Len(t, resp.Items, 2, "应列出 config/ 与 app.log")

	byName := map[string]bool{}
	for _, it := range resp.Items {
		byName[it.Name] = it.IsDir
		if it.Name == "app.log" {
			assert.Equal(t, int64(len("hello world")), it.Size)
			assert.Equal(t, filepath.Join(dir, "app.log"), it.Path)
			assert.Greater(t, it.MTime, int64(0))
		}
	}
	assert.True(t, byName["config"], "config 应为目录")
	assert.False(t, byName["app.log"], "app.log 应为文件")
}

// ls 访问不存在的目录 → 404，而不是 500。
func TestSFTPBrowserList_PathNotFound404(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	w := doJSON(t, router, "GET", "/api/v1/sftp/ls?node_id=node-1&path="+url.QueryEscape("/no/such/dir-x"), token, nil)
	assert.Equal(t, 404, w.Code, w.Body.String())
}

// ls 拒绝相对路径注入：必须是绝对路径 → 400。
func TestSFTPBrowserList_RejectsRelativePath(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	w := doJSON(t, router, "GET", "/api/v1/sftp/ls?node_id=node-1&path=etc/passwd", token, nil)
	assert.Equal(t, 400, w.Code, w.Body.String())
}

// ls 不带 path → 定位到 SFTP 用户主目录（浏览器初始位置）。
func TestSFTPBrowserList_EmptyPathMeansHome(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	w := doJSON(t, router, "GET", "/api/v1/sftp/ls?node_id=node-1", token, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var resp struct {
		Path string `json:"path"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp.Path, "应返回解析出的 home 目录")
	wd, _ := os.Getwd()
	assert.Equal(t, wd, resp.Path, "进程内 server 的 home 即当前工作目录")
}

// stat 已存在的文件 → 200 与元信息。
func TestSFTPBrowserStat_FileMeta(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	f := filepath.Join(dir, "data.txt")
	require.NoError(t, os.WriteFile(f, []byte("12345"), 0o600))

	w := doJSON(t, router, "GET", "/api/v1/sftp/stat?node_id=node-1&path="+url.QueryEscape(f), token, nil)
	require.Equal(t, 200, w.Code, w.Body.String())

	var resp struct {
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, f, resp.Path)
	assert.False(t, resp.IsDir)
	assert.Equal(t, int64(5), resp.Size)
}

// stat 不存在 → 404。
func TestSFTPBrowserStat_NotFound(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	w := doJSON(t, router, "GET", "/api/v1/sftp/stat?node_id=node-1&path="+url.QueryEscape("/missing-file-x"), token, nil)
	assert.Equal(t, 404, w.Code, w.Body.String())
}

// mkdir 创建目录成功，并真实落盘。
func TestSFTPBrowserMkdir_Creates(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := filepath.Join(t.TempDir(), "newdir")
	w := doJSON(t, router, "POST", "/api/v1/sftp/mkdir", token, map[string]string{
		"node_id": "node-1", "path": dir,
	})
	require.Equal(t, 200, w.Code, w.Body.String())

	fi, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, fi.IsDir())
}

// mkdir 目标已存在 → 409。
func TestSFTPBrowserMkdir_Exists409(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	w := doJSON(t, router, "POST", "/api/v1/sftp/mkdir", token, map[string]string{
		"node_id": "node-1", "path": dir,
	})
	assert.Equal(t, 409, w.Code, w.Body.String())
}

// rename 成功移动文件，内容保留，源消失。
func TestSFTPBrowserRename_Success(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(src, []byte("keep me"), 0o644))

	w := doJSON(t, router, "POST", "/api/v1/sftp/rename", token, map[string]string{
		"node_id": "node-1", "from": src, "to": dst,
	})
	require.Equal(t, 200, w.Code, w.Body.String())

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "keep me", string(got))
	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err))
}

// rename 源不存在 → 404。
func TestSFTPBrowserRename_SourceMissing404(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	w := doJSON(t, router, "POST", "/api/v1/sftp/rename", token, map[string]string{
		"node_id": "node-1", "from": filepath.Join(dir, "ghost"), "to": filepath.Join(dir, "x"),
	})
	assert.Equal(t, 404, w.Code, w.Body.String())
}

// rename 目标已存在 → 409（不得静默覆盖）。
func TestSFTPBrowserRename_TargetExists409(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(src, []byte("new"), 0o644))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0o644))

	w := doJSON(t, router, "POST", "/api/v1/sftp/rename", token, map[string]string{
		"node_id": "node-1", "from": src, "to": dst,
	})
	assert.Equal(t, 409, w.Code, w.Body.String())

	got, _ := os.ReadFile(dst)
	assert.Equal(t, "old", string(got), "409 时目标内容不得被覆盖")
}

// delete 删除文件。
func TestSFTPBrowserDelete_File(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	f := filepath.Join(dir, "x.txt")
	require.NoError(t, os.WriteFile(f, []byte("del"), 0o644))

	w := doJSON(t, router, "POST", "/api/v1/sftp/delete", token, map[string]any{
		"node_id": "node-1", "path": f,
	})
	require.Equal(t, 200, w.Code, w.Body.String())
	_, err := os.Stat(f)
	assert.True(t, os.IsNotExist(err))
}

// delete 非空目录默认拒绝（409），recursive=true 才级联删除。
func TestSFTPBrowserDelete_DirNeedsRecursive(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "f.txt"), []byte("1"), 0o644))

	w := doJSON(t, router, "POST", "/api/v1/sftp/delete", token, map[string]any{
		"node_id": "node-1", "path": sub,
	})
	assert.Equal(t, 409, w.Code, w.Body.String())
	_, err := os.Stat(sub)
	assert.NoError(t, err, "409 时目录不得被删除")

	w = doJSON(t, router, "POST", "/api/v1/sftp/delete", token, map[string]any{
		"node_id": "node-1", "path": sub, "recursive": true,
	})
	require.Equal(t, 200, w.Code, w.Body.String())
	_, err = os.Stat(sub)
	assert.True(t, os.IsNotExist(err))
}

// delete 路径不存在 → 404。
func TestSFTPBrowserDelete_NotFound(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	w := doJSON(t, router, "POST", "/api/v1/sftp/delete", token, map[string]any{
		"node_id": "node-1", "path": "/definitely/missing-x",
	})
	assert.Equal(t, 404, w.Code, w.Body.String())
}

// viewer 角色不能打开/使用 SFTP 浏览器（含只读的 ls）→ 403。
func TestSFTPBrowser_RBAC_ViewerForbidden(t *testing.T) {
	_, router, _, viewerToken, _ := sftpBrowserTestSetup(t)

	w := doJSON(t, router, "GET", "/api/v1/sftp/ls?node_id=node-1&path=/tmp", viewerToken, nil)
	assert.Equal(t, 403, w.Code, w.Body.String())
}

// 授权范围外节点 → 403，且不得发起拨号。
func TestSFTPBrowser_NodeOutOfScope(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)

	_, err := h.db.Exec(`CREATE TABLE web_users (username TEXT PRIMARY KEY, role TEXT, node_scope TEXT)`)
	require.NoError(t, err)
	_, err = h.db.Exec(`INSERT INTO web_users VALUES ('editor', 'editor', '{"nodes":["node-9"]}')`)
	require.NoError(t, err)

	dialed := false
	h.openSFTP = func(info *nodeSSHInfo) (*sftp.Client, func() error, error) {
		dialed = true
		return nil, nil, errors.New("must not dial")
	}
	w := doJSON(t, router, "GET", "/api/v1/sftp/ls?node_id=node-1&path=/tmp", token, nil)
	assert.Equal(t, 403, w.Code, w.Body.String())
	assert.False(t, dialed, "范围外节点不得触达拨号")
}

// 节点不存在 → 404（而非拨号失败 502）。
func TestSFTPBrowser_NodeNotFound(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	w := doJSON(t, router, "GET", "/api/v1/sftp/ls?node_id=ghost&path=/tmp", token, nil)
	assert.Equal(t, 404, w.Code, w.Body.String())
}

// GET /sftp/file 流式下载：内容一致 + attachment 语义。
func TestSFTPBrowserDownload_StreamsFile(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	f := filepath.Join(dir, "report.txt")
	require.NoError(t, os.WriteFile(f, []byte("line1\nline2\n"), 0o644))

	w := doJSON(t, router, "GET", "/api/v1/sftp/file?node_id=node-1&path="+url.QueryEscape(f), token, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, "line1\nline2\n", w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Disposition"), `attachment`)
	assert.Contains(t, w.Header().Get("Content-Disposition"), `report.txt`)
}

// 下载目录 → 400（目录应走 archive 打包）。
func TestSFTPBrowserDownload_DirRejected(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	w := doJSON(t, router, "GET", "/api/v1/sftp/file?node_id=node-1&path="+url.QueryEscape(dir), token, nil)
	assert.Equal(t, 400, w.Code, w.Body.String())
}

// injectFakeTar 注入 tar 执行缝：直接本机对 dir/base 打包，返回流。
func injectFakeTar(t *testing.T, h *SFTPBrowserHandler) {
	t.Helper()
	h.runTar = func(info *nodeSSHInfo, dir, base string) (io.ReadCloser, func() error, error) {
		cmd := exec.Command("tar", "-C", dir, "-czf", "-", base)
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		return out, func() error { return cmd.Wait() }, nil
	}
}

// 目录打包下载：以 <目录名>.tar.gz 附件流出，内容可解包还原。
func TestSFTPBrowserArchive_StreamsTarGz(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)
	injectFakeTar(t, h)

	parent := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(parent, "myproj"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(parent, "myproj", "a.txt"), []byte("AAA"), 0o644))

	w := doJSON(t, router, "GET", "/api/v1/sftp/archive?node_id=node-1&path="+url.QueryEscape(filepath.Join(parent, "myproj")), token, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Header().Get("Content-Disposition"), `myproj.tar.gz`)

	// 解包验证内容
	gzr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err, "响应体应是合法 gzip")
	tr := tar.NewReader(gzr)
	var found string
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		// macOS bsdtar 会附带 ._* AppleDouble 头，跳过，只看真实文件
		if strings.HasPrefix(filepath.Base(hdr.Name), "._") {
			continue
		}
		if strings.HasSuffix(hdr.Name, "a.txt") {
			data, _ := io.ReadAll(tr)
			assert.Equal(t, "AAA", string(data))
			found = "ok"
			break
		}
	}
	assert.Equal(t, "ok", found, "tar 包内应含 myproj/a.txt")
}

// archive 指向文件（非目录）→ 400；不存在 → 404。
func TestSFTPBrowserArchive_NonDirAndMissing(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)
	injectFakeTar(t, h)

	dir := t.TempDir()
	f := filepath.Join(dir, "plain.txt")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o644))

	w := doJSON(t, router, "GET", "/api/v1/sftp/archive?node_id=node-1&path="+url.QueryEscape(f), token, nil)
	assert.Equal(t, 400, w.Code, w.Body.String())

	w = doJSON(t, router, "GET", "/api/v1/sftp/archive?node_id=node-1&path="+url.QueryEscape("/missing-dir-x"), token, nil)
	assert.Equal(t, 404, w.Code, w.Body.String())
}

// 含 shell 元字符的路径打包时不得被解释执行（单引号包裹 + 内引号转义）。
func TestShellQuoteNeutralizesMetacharacters(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwned")
	quoted := shellQuote("/tmp/it's a ; touch " + marker + " #")
	cmd := exec.Command("/bin/sh", "-c", "echo "+quoted)
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/it's a ; touch "+marker+" #", strings.TrimRight(string(out), "\n"))
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "注入命令不得执行")
}

// doUpload 发一个 PUT 流式上传请求，body 为文件内容，mode/新名走 query（默认 node-1）。
func doUpload(t *testing.T, router *gin.Engine, token, remotePath, mode, newName, content string) *httptest.ResponseRecorder {
	return doUploadNode(t, router, token, "node-1", remotePath, mode, newName, content)
}

func doUploadNode(t *testing.T, router *gin.Engine, token, nodeID, remotePath, mode, newName, content string) *httptest.ResponseRecorder {
	t.Helper()
	q := url.Values{"node_id": {nodeID}, "path": {remotePath}}
	if mode != "" {
		q.Set("mode", mode)
	}
	if newName != "" {
		q.Set("new_name", newName)
	}
	req, _ := http.NewRequest("PUT", "/api/v1/sftp/file?"+q.Encode(), strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// 全新路径上传 → 200，文件落盘，final_path 等于目标。
func TestSFTPBrowserUpload_NewFile(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	dst := filepath.Join(dir, "fresh.txt")
	w := doUpload(t, router, token, dst, "", "", "payload")
	require.Equal(t, 200, w.Code, w.Body.String())

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "payload", string(got))
	var resp struct {
		FinalPath string `json:"final_path"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, dst, resp.FinalPath)
}

// 已存在 + overwrite → 覆盖内容。
func TestSFTPBrowserUpload_Overwrite(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	dst := filepath.Join(dir, "dup.txt")
	require.NoError(t, os.WriteFile(dst, []byte("OLDOLD"), 0o644))

	w := doUpload(t, router, token, dst, "overwrite", "", "NEW")
	require.Equal(t, 200, w.Code, w.Body.String())
	got, _ := os.ReadFile(dst)
	assert.Equal(t, "NEW", string(got), "overwrite 应截断旧内容")
}

// 已存在 + skip → 409，原文件不动。
func TestSFTPBrowserUpload_Skip409(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	dst := filepath.Join(dir, "keep.txt")
	require.NoError(t, os.WriteFile(dst, []byte("ORIGINAL"), 0o644))

	w := doUpload(t, router, token, dst, "skip", "", "IGNORED")
	assert.Equal(t, 409, w.Code, w.Body.String())
	got, _ := os.ReadFile(dst)
	assert.Equal(t, "ORIGINAL", string(got))
}

// 已存在 + auto_rename → 写入 name_1.ext，原文件保留。
func TestSFTPBrowserUpload_AutoRename(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	dst := filepath.Join(dir, "rpt.txt")
	require.NoError(t, os.WriteFile(dst, []byte("v1"), 0o644))

	w := doUpload(t, router, token, dst, "auto_rename", "", "v2")
	require.Equal(t, 200, w.Code, w.Body.String())
	var resp struct {
		FinalPath string `json:"final_path"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, filepath.Join(dir, "rpt_1.txt"), resp.FinalPath, "序号应插在扩展名之前")
	got, _ := os.ReadFile(resp.FinalPath)
	assert.Equal(t, "v2", string(got))
	orig, _ := os.ReadFile(dst)
	assert.Equal(t, "v1", string(orig), "原文件不动")
}

// auto_rename 冲突时递增到第一个可用名（rpt.txt, rpt_1.txt 都在 → rpt_2.txt）。
func TestSFTPBrowserUpload_AutoRenameIncrements(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rpt.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rpt_1.txt"), []byte("b"), 0o644))

	w := doUpload(t, router, token, filepath.Join(dir, "rpt.txt"), "auto_rename", "", "c")
	require.Equal(t, 200, w.Code, w.Body.String())
	var resp struct {
		FinalPath string `json:"final_path"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, filepath.Join(dir, "rpt_2.txt"), resp.FinalPath)
}

// 已存在 + rename 指定新名 → 写入新名。
func TestSFTPBrowserUpload_RenameTo(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	dir := t.TempDir()
	dst := filepath.Join(dir, "orig.txt")
	require.NoError(t, os.WriteFile(dst, []byte("v1"), 0o644))

	w := doUpload(t, router, token, dst, "rename", "chosen.log", "v2")
	require.Equal(t, 200, w.Code, w.Body.String())
	got, err := os.ReadFile(filepath.Join(dir, "chosen.log"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got))
}

// 序号插在扩展名前；无扩展名与隐藏文件（.gitignore）追加到末尾。
func TestInsertSeq(t *testing.T) {
	assert.Equal(t, "rpt_1.txt", insertSeq("rpt.txt", 1))
	assert.Equal(t, "archive.tar_2.gz", insertSeq("archive.tar.gz", 2)) // 按最后一个扩展名切
	assert.Equal(t, "README_1", insertSeq("README", 1))
	assert.Equal(t, ".gitignore_1", insertSeq(".gitignore", 1)) // 隐藏文件整名无扩展
}

// new_name 含路径分隔符 → 400（防穿越）。
func TestSFTPBrowserUpload_RejectsPathTraversalNewName(t *testing.T) {
	h, router, token, _, _ := sftpBrowserTestSetup(t)
	injectInProcSFTP(t, h)

	w := doUpload(t, router, token, filepath.Join(t.TempDir(), "x.txt"), "rename", "../escape.txt", "data")
	assert.Equal(t, 400, w.Code, w.Body.String())
}

// —— 真实链路 E2E（localhost sshd，缺密钥时自动跳过）——

// localhost 注册为 node 行，返回 nodeID。
func localhostNodeRow(t *testing.T, db *sql.DB) string {
	t.Helper()
	info := localhostNodeInfo(t) // 无 ~/.ssh/id_rsa 或 sshd 不可达时 skip
	_, err := db.Exec(`INSERT INTO nodes (id, address, port, user, ssh_key) VALUES ('e2e-node', ?, ?, ?, ?)`,
		info.Address, info.Port, info.User, info.SSHKey)
	require.NoError(t, err)
	return "e2e-node"
}

// E2E：默认拨号（internal/ssh.Dial，支持 ProxyJump 链路）+ 真实 sftp-server 上传/下载往返。
func TestSFTPBrowserE2E_UploadDownloadRoundTrip(t *testing.T) {
	_, router, token, _, db := sftpBrowserTestSetup(t)
	nodeID := localhostNodeRow(t, db)

	remoteDir := filepath.Join(os.TempDir(), fmt.Sprintf("owl-sftp-e2e-%d", os.Getpid()))
	require.NoError(t, os.MkdirAll(remoteDir, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll(remoteDir) })

	content := "e2e payload 端到端\n"
	dst := filepath.Join(remoteDir, "up.txt")
	w := doUploadNode(t, router, token, nodeID, dst, "overwrite", "", content)
	require.Equal(t, 200, w.Code, w.Body.String())

	w = doJSON(t, router, "GET", "/api/v1/sftp/file?node_id="+nodeID+"&path="+url.QueryEscape(dst), token, nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Equal(t, content, w.Body.String())
}

// E2E：目录打包下载走真实 SSH exec tar 管道。
func TestSFTPBrowserE2E_ArchiveViaSshTar(t *testing.T) {
	_, router, token, _, db := sftpBrowserTestSetup(t)
	nodeID := localhostNodeRow(t, db)

	parent := filepath.Join(os.TempDir(), fmt.Sprintf("owl-sftp-e2e-ar-%d", os.Getpid()))
	proj := filepath.Join(parent, "proj with space") // 含空格路径验证 shell 引号
	require.NoError(t, os.MkdirAll(proj, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "msg.txt"), []byte("tar over ssh"), 0o644))
	t.Cleanup(func() { _ = os.RemoveAll(parent) })

	w := doJSON(t, router, "GET", "/api/v1/sftp/archive?node_id="+nodeID+"&path="+url.QueryEscape(proj), token, nil)
	require.Equal(t, 200, w.Code, w.Body.String())

	gzr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err, "响应应是 gzip(tar)，检查本机是否可用 GNU tar；got="+w.Body.String())
	tr := tar.NewReader(gzr)
	var ok bool
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if strings.HasPrefix(filepath.Base(hdr.Name), "._") {
			continue
		}
		if strings.HasSuffix(hdr.Name, "msg.txt") {
			data, _ := io.ReadAll(tr)
			assert.Equal(t, "tar over ssh", string(data))
			ok = true
		}
	}
	assert.True(t, ok, "含空格目录应被完整打包")
}
