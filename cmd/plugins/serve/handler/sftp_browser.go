package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	owlssh "github.com/cangyunye/go-owl/internal/ssh"
	"github.com/gin-gonic/gin"
	"github.com/pkg/sftp"
)

// SFTPBrowserHandler 提供节点文件浏览器的目录操作接口（v1.8.0）。
// 凭证读取走 resolveNodeSSH（内部已解密），拨号统一走 internal/ssh.Dial 以支持 ProxyJump。
type SFTPBrowserHandler struct {
	db *sql.DB
	// openSFTP 是建立 SFTP 会话的接缝，测试可注入进程内 server。
	openSFTP func(info *nodeSSHInfo) (*sftp.Client, func() error, error)
	// runTar 在节点上执行 `tar -C dir -czf - base` 并返回 stdout 流；
	// 默认经 SSH exec 通道，测试可注入本机 tar。
	runTar func(info *nodeSSHInfo, dir, base string) (io.ReadCloser, func() error, error)
}

func NewSFTPBrowserHandler(db *sql.DB) *SFTPBrowserHandler {
	return &SFTPBrowserHandler{db: db, openSFTP: defaultOpenSFTP, runTar: defaultRunTar}
}

func defaultOpenSFTP(info *nodeSSHInfo) (*sftp.Client, func() error, error) {
	addr := net.JoinHostPort(info.Address, strconv.Itoa(info.Port))
	cli, err := owlssh.Dial(context.Background(), addr, owlssh.DialOptions{
		User:       info.User,
		Password:   info.Password,
		KeyContent: info.SSHKey,
		ProxyJump:  info.ProxyJump,
	})
	if err != nil {
		return nil, nil, err
	}
	sc, err := sftp.NewClient(cli.Client)
	if err != nil {
		cli.Close()
		return nil, nil, err
	}
	return sc, cli.Close, nil
}

// shellQuote 用单引号包裹参数，安全嵌入远端 shell 命令，防命令注入。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// defaultRunTar 经 SSH exec 在节点上执行 tar，流式返回 stdout。
func defaultRunTar(info *nodeSSHInfo, dir, base string) (io.ReadCloser, func() error, error) {
	addr := net.JoinHostPort(info.Address, strconv.Itoa(info.Port))
	cli, err := owlssh.Dial(context.Background(), addr, owlssh.DialOptions{
		User:       info.User,
		Password:   info.Password,
		KeyContent: info.SSHKey,
		ProxyJump:  info.ProxyJump,
	})
	if err != nil {
		return nil, nil, err
	}
	session, err := cli.NewSession()
	if err != nil {
		cli.Close()
		return nil, nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		cli.Close()
		return nil, nil, err
	}
	reader := io.NopCloser(stdout)
	cmd := "tar -C " + shellQuote(dir) + " -czf - " + shellQuote(base)
	if err := session.Start(cmd); err != nil {
		session.Close()
		cli.Close()
		return nil, nil, err
	}
	cleanup := func() error {
		waitErr := session.Wait()
		session.Close()
		cli.Close()
		return waitErr
	}
	return reader, cleanup, nil
}

// sanitizeRemotePath 校验并清洗远端路径：仅接受绝对路径；拒绝空字节。
// 绝对路径上的 ".." 只会在根方向收敛（path.Clean("/a/../..") == "/"），
// 不存在逃逸到非法父级的问题；真正的防护是拒绝相对路径注入。
func sanitizeRemotePath(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("path is required")
	}
	if len(raw) > 4096 {
		return "", errors.New("path too long")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] == 0 {
			return "", errors.New("invalid path")
		}
	}
	if raw[0] != '/' {
		return "", errors.New("absolute path required")
	}
	return path.Clean(raw), nil
}

type sftpEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	Mode  string `json:"mode"`
}

func entryFromInfo(p string, fi os.FileInfo) sftpEntry {
	return sftpEntry{
		Name:  fi.Name(),
		Path:  p,
		IsDir: fi.IsDir(),
		Size:  fi.Size(),
		MTime: fi.ModTime().Unix(),
		Mode:  fi.Mode().String(),
	}
}

// browserSession 完成请求级前置：参数校验 → 范围授权 → 凭证 → 拨号。
// 返回 cleanup 释放会话；ok=false 时响应已写出，调用方直接 return。
func (h *SFTPBrowserHandler) browserSession(c *gin.Context, nodeID, rawPath string) (*sftp.Client, *nodeSSHInfo, func(), bool) {
	noop := func() {}
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "node_id is required"})
		return nil, nil, noop, false
	}
	cleaned, err := sanitizeRemotePath(rawPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return nil, nil, noop, false
	}
	c.Set("sftp_path", cleaned)
	if filtered := NewScopeChecker(h.db).FilterNodeIDs(c.Request.Context(), c.GetString("username"), []string{nodeID}); len(filtered) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "node out of your scope"})
		return nil, nil, noop, false
	}
	info, err := resolveNodeSSH(h.db, nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "node not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		}
		return nil, nil, noop, false
	}
	client, closeSSH, err := h.openSFTP(info)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "sftp connect failed: " + err.Error()})
		return nil, nil, noop, false
	}
	cleanup := func() {
		_ = client.Close()
		_ = closeSSH()
	}
	return client, info, cleanup, true
}

// respondPathError 把 sftp/OS 层错误映射为 HTTP 语义。
func respondPathError(c *gin.Context, p string, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "path not found: " + p})
	case errors.Is(err, fs.ErrPermission):
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "permission denied: " + p})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
	}
}

func (h *SFTPBrowserHandler) List(c *gin.Context) {
	client, _, cleanup, ok := h.browserSession(c, c.Query("node_id"), c.Query("path"))
	if !ok {
		return
	}
	defer cleanup()
	dir := c.GetString("sftp_path")

	fis, err := client.ReadDir(dir)
	if err != nil {
		respondPathError(c, dir, err)
		return
	}
	items := make([]sftpEntry, 0, len(fis))
	for _, fi := range fis {
		items = append(items, entryFromInfo(path.Join(dir, fi.Name()), fi))
	}
	c.JSON(http.StatusOK, gin.H{"path": dir, "items": items})
}

func (h *SFTPBrowserHandler) Stat(c *gin.Context) {
	client, _, cleanup, ok := h.browserSession(c, c.Query("node_id"), c.Query("path"))
	if !ok {
		return
	}
	defer cleanup()
	p := c.GetString("sftp_path")

	fi, err := client.Stat(p)
	if err != nil {
		respondPathError(c, p, err)
		return
	}
	c.JSON(http.StatusOK, entryFromInfo(p, fi))
}

func (h *SFTPBrowserHandler) Mkdir(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id"`
		Path   string `json:"path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	client, _, cleanup, ok := h.browserSession(c, req.NodeID, req.Path)
	if !ok {
		return
	}
	defer cleanup()
	p := c.GetString("sftp_path")

	if _, err := client.Stat(p); err == nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "path already exists: " + p})
		return
	}
	if err := client.Mkdir(p); err != nil {
		respondPathError(c, p, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": p})
}

func (h *SFTPBrowserHandler) Rename(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id"`
		From   string `json:"from"`
		To     string `json:"to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	from, err := sanitizeRemotePath(req.From)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	client, _, cleanup, ok := h.browserSession(c, req.NodeID, req.To)
	if !ok {
		return
	}
	defer cleanup()
	to := c.GetString("sftp_path")

	if _, err := client.Stat(from); err != nil {
		respondPathError(c, from, err)
		return
	}
	if _, err := client.Stat(to); err == nil {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "target already exists: " + to})
		return
	}
	if err := client.Rename(from, to); err != nil {
		respondPathError(c, from, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"from": from, "to": to})
}

// removeRemote 删除远端路径；recursive=false 时非空目录返回 errDirNotEmpty。
func removeRemote(client *sftp.Client, p string, recursive bool) error {
	fi, err := client.Stat(p)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		entries, err := client.ReadDir(p)
		if err != nil {
			return err
		}
		if len(entries) > 0 && !recursive {
			return errDirNotEmpty
		}
		if recursive {
			for _, e := range entries {
				if err := removeRemote(client, path.Join(p, e.Name()), true); err != nil {
					return err
				}
			}
		}
		return client.RemoveDirectory(p)
	}
	return client.Remove(p)
}

var errDirNotEmpty = errors.New("directory not empty")

func (h *SFTPBrowserHandler) Delete(c *gin.Context) {
	var req struct {
		NodeID    string `json:"node_id"`
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	client, _, cleanup, ok := h.browserSession(c, req.NodeID, req.Path)
	if !ok {
		return
	}
	defer cleanup()
	p := c.GetString("sftp_path")

	if err := removeRemote(client, p, req.Recursive); err != nil {
		if errors.Is(err, errDirNotEmpty) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "directory not empty: " + p})
			return
		}
		respondPathError(c, p, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": p})
}

func (h *SFTPBrowserHandler) Download(c *gin.Context) {
	client, _, cleanup, ok := h.browserSession(c, c.Query("node_id"), c.Query("path"))
	if !ok {
		return
	}
	defer cleanup()
	p := c.GetString("sftp_path")

	fi, err := client.Stat(p)
	if err != nil {
		respondPathError(c, p, err)
		return
	}
	if fi.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "path is a directory; use archive download"})
		return
	}
	remote, err := client.Open(p)
	if err != nil {
		respondPathError(c, p, err)
		return
	}
	defer remote.Close()

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", `attachment; filename="`+path.Base(p)+`"`)
	c.Header("Content-Length", strconv.FormatInt(fi.Size(), 10))
	c.Status(http.StatusOK)
	if _, err := io.CopyN(c.Writer, remote, fi.Size()); err != nil {
		log.Printf("sftp download %s: %v", p, err)
	}
}

func (h *SFTPBrowserHandler) Archive(c *gin.Context) {
	client, info, cleanup, ok := h.browserSession(c, c.Query("node_id"), c.Query("path"))
	if !ok {
		return
	}
	defer cleanup()
	p := c.GetString("sftp_path")

	fi, err := client.Stat(p)
	if err != nil {
		respondPathError(c, p, err)
		return
	}
	if !fi.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "archive requires a directory"})
		return
	}
	stream, closeTar, err := h.runTar(info, path.Dir(p), path.Base(p))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "tar failed: " + err.Error()})
		return
	}
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", `attachment; filename="`+path.Base(p)+`.tar.gz"`)
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, stream); err != nil {
		log.Printf("sftp archive %s: copy: %v", p, err)
	}
	if err := closeTar(); err != nil {
		log.Printf("sftp archive %s: tar exit: %v", p, err)
	}
}

// insertSeq 在扩展名之前插入序号：rpt.txt → rpt_1.txt；无扩展名（或以点开头）→ name_1。
func insertSeq(name string, n int) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return fmt.Sprintf("%s_%d%s", name[:i], n, name[i:])
	}
	return fmt.Sprintf("%s_%d", name, n)
}

// resolveAvailableName 找第一个可用名：base.ext → base_1.ext → base_2.ext …
func resolveAvailableName(client *sftp.Client, target string) (string, error) {
	if _, err := client.Stat(target); err != nil {
		return target, nil // stat 不了（通常是不存在）→ 原样返回，真正错误交给 Create 呈现
	}
	dir, base := path.Dir(target), path.Base(target)
	for n := 1; n < 10000; n++ {
		cand := path.Join(dir, insertSeq(base, n))
		if _, err := client.Stat(cand); err != nil {
			return cand, nil
		}
	}
	return "", fmt.Errorf("no available name for %s", base)
}

const defaultSFTPMaxUploadBytes = 2048 << 20 // 2GiB

func (h *SFTPBrowserHandler) Upload(c *gin.Context) {
	mode := c.Query("mode")
	client, _, cleanup, ok := h.browserSession(c, c.Query("node_id"), c.Query("path"))
	if !ok {
		return
	}
	defer cleanup()
	target := c.GetString("sftp_path")

	// rename 模式：用 new_name 替换目标文件名（仅允许纯基名），仍冲突则续用序号规则
	if newName := c.Query("new_name"); newName != "" {
		if strings.ContainsAny(newName, `/\`) || strings.IndexByte(newName, 0) >= 0 || newName == "." || newName == ".." {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid new_name"})
			return
		}
		target = path.Join(path.Dir(target), newName)
	}

	if _, err := client.Stat(target); err == nil {
		switch mode {
		case "overwrite":
			// 截断重写
		case "skip":
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "file exists: " + target})
			return
		case "auto_rename", "rename":
			var rerr error
			target, rerr = resolveAvailableName(client, target)
			if rerr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": rerr.Error()})
				return
			}
		default:
			c.JSON(http.StatusConflict, gin.H{"code": 409, "message": "file exists, specify mode: " + target})
			return
		}
	}

	if c.Request.ContentLength > defaultSFTPMaxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 413, "message": "file exceeds upload limit"})
		return
	}
	remote, err := client.Create(target)
	if err != nil {
		respondPathError(c, target, err)
		return
	}
	written, cerr := io.Copy(remote, io.LimitReader(c.Request.Body, defaultSFTPMaxUploadBytes+1))
	closeFile := func() error {
		_ = remote.Sync() // fsync 非必需且部分 server 不支持；Close 才是权威落盘点
		return remote.Close()
	}
	if written > defaultSFTPMaxUploadBytes {
		closeFile()
		_ = client.Remove(target)
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 413, "message": "file exceeds upload limit"})
		return
	}
	if cerr != nil {
		closeFile()
		_ = client.Remove(target)
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "upload write failed: " + cerr.Error()})
		return
	}
	if ferr := closeFile(); ferr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"code": 502, "message": "upload close failed: " + ferr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"final_path": target, "size": written})
}
