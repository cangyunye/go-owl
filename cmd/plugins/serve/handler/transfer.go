package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	nodeselect "github.com/cangyunye/go-owl/internal/node/select"
	"github.com/gin-gonic/gin"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type TransferHandler struct {
	db          *sql.DB
	task        *store.TaskStore
	recordStore *store.TransferRecordStore
	History     *store.HistoryStore
	Hub         *WSHub
}

type transferRequest struct {
	Action     string            `json:"action"` // "upload" or "download"
	NodeIDs    []string          `json:"node_ids"`
	Groups     []string          `json:"groups"`
	Labels     map[string]string `json:"labels"`
	SourcePath string            `json:"source_path"`
	DestPath   string            `json:"dest_path"`
	Direction  string            `json:"direction"` // "push" or "pull"
	Overwrite  bool              `json:"overwrite"`
	Mode       string            `json:"mode"`
	Parallel   *bool             `json:"parallel"`
	Resume     bool              `json:"resume"`
}

type transferOptions struct {
	Overwrite bool
	Mode      os.FileMode
	Resume    bool
}

type transferResponse struct {
	NodeID string `json:"node_id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func NewTransferHandler(db *sql.DB, ts *store.TaskStore, rs *store.TransferRecordStore) *TransferHandler {
	return &TransferHandler{db: db, task: ts, recordStore: rs}
}

func parseFileMode(s string) os.FileMode {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0
	}
	return os.FileMode(v)
}

func (h *TransferHandler) Create(c *gin.Context) {
	var req transferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
	}
	h.submit(c, &req)
}

// saveRecordPayload 持久化完整请求(含筛选条件与传输选项),
// 供"重新执行"时按原样重放,保证节点选择/参数与首次完全一致。
func (h *TransferHandler) saveRecordPayload(ctx context.Context, recordID string, req *transferRequest, direction string) error {
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if direction != "" && req.Direction == "" {
		req.Direction = direction
	}
	return h.recordStore.SetPayload(ctx, recordID, string(payload))
}

func (h *TransferHandler) submit(c *gin.Context, req *transferRequest) {
	if len(req.NodeIDs) == 0 {
		sel := nodeselect.NewSelector(&dbNodeSource{db: h.db})
		opts := nodeselect.SelectOptions{Groups: req.Groups, Labels: req.Labels}
		nodes, err := sel.SelectIntersect(c.Request.Context(), opts)
		if err != nil {
			respondErr(c, http.StatusBadRequest, "resolve target nodes failed", err)
			return
		}
		for _, n := range nodes {
			req.NodeIDs = append(req.NodeIDs, n.ID)
		}
	}
	if len(req.NodeIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "no target nodes specified"})
		return
	}
	if req.SourcePath == "" || req.DestPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "source_path and dest_path are required"})
		return
	}

	direction := req.Direction
	if direction == "" {
		direction = "push"
	}
	parallel := req.Parallel == nil || *req.Parallel
	opts := transferOptions{Overwrite: req.Overwrite, Mode: parseFileMode(req.Mode), Resume: req.Resume}

	transferRec, err := h.recordStore.Create(c.Request.Context(), req.SourcePath, req.DestPath, direction, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create record failed"})
		return
	}
	if err := h.saveRecordPayload(c.Request.Context(), transferRec.ID, req, direction); err != nil {
		log.Printf("save record payload: %v", err)
	}
	h.recordStore.SetNodeCount(c.Request.Context(), transferRec.ID, len(req.NodeIDs))

	op := &store.Operation{TaskID: transferRec.ID, OpType: "file_transfer", Command: fmt.Sprintf("transfer %s -> %s", req.SourcePath, req.DestPath), Targets: req.NodeIDs, Status: "running", CreatedAt: time.Now().UTC(), Username: c.GetString("username")}
	if err := h.History.RecordOperation(c.Request.Context(), op); err != nil {
		log.Printf("record history: %v", err)
	}
	if h.Hub != nil {
		h.Hub.BroadcastHistoryUpdate()
	}

	type transferItem struct {
		nodeID string
		info   *nodeSSHInfo
		taskID string
	}
	items := make([]transferItem, 0, len(req.NodeIDs))
	results := make([]transferResponse, 0, len(req.NodeIDs))

	for _, nid := range req.NodeIDs {
		info, err := resolveNodeSSH(h.db, nid)
		if err != nil {
			h.recordStore.UpdateNodeResult(c.Request.Context(), transferRec.ID, false)
			results = append(results, transferResponse{NodeID: nid, Status: "failed", Error: err.Error()})
			continue
		}

		rec, err := h.task.CreateWithRecord(c.Request.Context(), nid, fmt.Sprintf("transfer:%s -> %s", req.SourcePath, req.DestPath), transferRec.ID)
		if err != nil {
			h.recordStore.UpdateNodeResult(c.Request.Context(), transferRec.ID, false)
			results = append(results, transferResponse{NodeID: nid, Status: "failed", Error: err.Error()})
			continue
		}

		items = append(items, transferItem{nodeID: nid, info: info, taskID: rec.ID})
		results = append(results, transferResponse{NodeID: nid, Status: "queued"})
	}

	h.recordStore.MarkRunning(c.Request.Context(), transferRec.ID)

	if parallel {
		for _, it := range items {
			go h.runTransfer(it.nodeID, req.SourcePath, req.DestPath, direction, transferRec.ID, it.taskID, it.info, opts)
		}
	} else {
		go func() {
			for _, it := range items {
				h.runTransfer(it.nodeID, req.SourcePath, req.DestPath, direction, transferRec.ID, it.taskID, it.info, opts)
			}
		}()
	}

	c.JSON(http.StatusAccepted, gin.H{"record_id": transferRec.ID, "transfers": results})
}

// transferOutcome 依据单节点传输结果推导任务状态与任务输出。
// skipped 表示断点续传发现目标已不小于源、未做拷贝——输出必须如实写明跳过原因，
// 否则用户会看到传输"秒完成"却以为发生了完整拷贝。
func transferOutcome(err error, skipped bool, src, dst string) (store.TaskStatus, string) {
	if err != nil {
		return store.TaskStatusFailed, err.Error()
	}
	if skipped {
		return store.TaskStatusCompleted, "skipped: destination already up to date (resume)"
	}
	return store.TaskStatusCompleted, fmt.Sprintf("transfer %s -> %s completed", src, dst)
}

func (h *TransferHandler) runTransfer(nodeID, src, dst, dir, recordID, taskID string, info *nodeSSHInfo, opts transferOptions) {
	bg := context.Background()
	skipped, err := sftpTransfer(info, src, dst, dir, opts)
	taskStatus, output := transferOutcome(err, skipped, src, dst)
	h.task.UpdateStatus(bg, taskID, taskStatus, output, nil)
	h.recordStore.UpdateNodeResult(bg, recordID, err == nil)
	ftStatus := "completed"
	if err != nil {
		ftStatus = "failed"
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	ft := &store.FileTransfer{TaskID: recordID, NodeID: nodeID, FileName: filepath.Base(src), TransferType: dir, Status: ftStatus, Error: errMsg, CreatedAt: time.Now().UTC()}
	if e := h.History.RecordFileTransfer(bg, ft); e != nil {
		log.Printf("record file transfer: %v", e)
	}
	h.updateOpStatus(bg, recordID)
}

func resolveNodeSSH(db *sql.DB, nodeID string) (*nodeSSHInfo, error) {
	var info nodeSSHInfo
	var pw, key sql.NullString
	err := db.QueryRow(
		`SELECT address, port, user, password, ssh_key, COALESCE(proxy_jump, '') FROM nodes WHERE id = ?`, nodeID,
	).Scan(&info.Address, &info.Port, &info.User, &pw, &key, &info.ProxyJump)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	if pw.Valid {
		info.Password = pw.String
	}
	if key.Valid {
		info.SSHKey = key.String
	}
	if err := decryptNodeSSHInfo(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func dialSFTP(info *nodeSSHInfo) (*sftp.Client, *ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            info.User,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	if info.Password != "" {
		config.Auth = append(config.Auth, ssh.Password(info.Password))
	}
	if info.SSHKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(info.SSHKey))
		if err != nil {
			return nil, nil, fmt.Errorf("parse ssh key: %w", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}
	addr := net.JoinHostPort(info.Address, strconv.Itoa(info.Port))
	sshClient, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial: %w", err)
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, nil, fmt.Errorf("sftp client: %w", err)
	}
	return sftpClient, sshClient, nil
}

// sftpTransfer 执行单节点传输；skipped=true 表示断点续传发现目标已不小于源，
// 未做任何拷贝（需要向用户如实呈现"跳过"而不是伪装成成功拷贝）。
func sftpTransfer(info *nodeSSHInfo, src, dst, direction string, opts transferOptions) (bool, error) {
	sftpClient, sshClient, err := dialSFTP(info)
	if err != nil {
		return false, err
	}
	defer sshClient.Close()
	defer sftpClient.Close()
	if direction == "pull" {
		return sftpPull(sftpClient, src, dst, opts)
	}
	return sftpPush(sftpClient, src, dst, opts)
}

func resolveRemoteDest(client *sftp.Client, dst, base string) string {
	if strings.HasSuffix(dst, "/") {
		return path.Join(dst, base)
	}
	if fi, err := client.Stat(dst); err == nil && fi.IsDir() {
		return path.Join(dst, base)
	}
	return dst
}

func resolveLocalDest(dst, base string) string {
	if strings.HasSuffix(dst, "/") || strings.HasSuffix(dst, string(os.PathSeparator)) {
		return filepath.Join(dst, base)
	}
	if fi, err := os.Stat(dst); err == nil && fi.IsDir() {
		return filepath.Join(dst, base)
	}
	return dst
}

func sftpPush(client *sftp.Client, src, dst string, opts transferOptions) (bool, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return false, fmt.Errorf("open local: %w", err)
	}
	defer srcFile.Close()
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return false, fmt.Errorf("stat local: %w", err)
	}
	srcSize := srcInfo.Size()

	remotePath := resolveRemoteDest(client, dst, filepath.Base(src))

	var dstFile *sftp.File
	remoteInfo, statErr := client.Stat(remotePath)
	exists := statErr == nil

	switch {
	case exists && opts.Resume:
		rs := remoteInfo.Size()
		if rs >= srcSize {
			if opts.Mode != 0 {
				_ = client.Chmod(remotePath, opts.Mode)
			}
			return true, nil
		}
		dstFile, err = client.OpenFile(remotePath, os.O_WRONLY|os.O_APPEND)
		if err != nil {
			return false, fmt.Errorf("open remote for resume: %w", err)
		}
		if _, err := srcFile.Seek(rs, io.SeekStart); err != nil {
			dstFile.Close()
			return false, fmt.Errorf("seek local: %w", err)
		}
	case exists && !opts.Overwrite:
		return false, fmt.Errorf("remote file exists: %s (enable overwrite)", remotePath)
	default:
		dstFile, err = client.Create(remotePath)
		if err != nil {
			return false, fmt.Errorf("create remote: %w", err)
		}
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return false, fmt.Errorf("copy: %w", err)
	}
	if opts.Mode != 0 {
		if err := client.Chmod(remotePath, opts.Mode); err != nil {
			return false, fmt.Errorf("chmod: %w", err)
		}
	}
	return false, nil
}

func sftpPull(client *sftp.Client, src, dst string, opts transferOptions) (bool, error) {
	srcFile, err := client.Open(src)
	if err != nil {
		return false, fmt.Errorf("open remote: %w", err)
	}
	defer srcFile.Close()
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return false, fmt.Errorf("stat remote: %w", err)
	}
	srcSize := srcInfo.Size()

	localPath := resolveLocalDest(dst, filepath.Base(src))

	var dstFile *os.File
	localInfo, statErr := os.Stat(localPath)
	exists := statErr == nil

	switch {
	case exists && opts.Resume:
		ls := localInfo.Size()
		if ls >= srcSize {
			if opts.Mode != 0 {
				_ = os.Chmod(localPath, opts.Mode)
			}
			return true, nil
		}
		dstFile, err = os.OpenFile(localPath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return false, fmt.Errorf("open local for resume: %w", err)
		}
		if _, err := srcFile.Seek(ls, io.SeekStart); err != nil {
			dstFile.Close()
			return false, fmt.Errorf("seek remote: %w", err)
		}
	case exists && !opts.Overwrite:
		return false, fmt.Errorf("local file exists: %s (enable overwrite)", localPath)
	default:
		dstFile, err = os.Create(localPath)
		if err != nil {
			return false, fmt.Errorf("create local: %w", err)
		}
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return false, fmt.Errorf("copy: %w", err)
	}
	if opts.Mode != 0 {
		if err := os.Chmod(localPath, opts.Mode); err != nil {
			return false, fmt.Errorf("chmod: %w", err)
		}
	}
	return false, nil
}

func (h *TransferHandler) List(c *gin.Context) {
	// 必须在 SQL 层按前缀过滤后再分页：tasks 表与命令执行/监控共享，
	// 先取全量再内存过滤会让其它任务把传输任务挤出列表（列表间歇性变空）。
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	tasks, total, err := h.task.ListByCommandPrefix(c.Request.Context(), "transfer:", pageSize, (page-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "list failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": tasks,
		"meta": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

func (h *TransferHandler) Records(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	records, total, err := h.recordStore.List(c.Request.Context(), pageSize, (page-1)*pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "list records failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": records,
		"meta": gin.H{"total": total, "page": page, "page_size": pageSize},
	})
}

func (h *TransferHandler) RecordGet(c *gin.Context) {
	id := c.Param("id")
	rec, err := h.recordStore.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "record not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

// Rerun 按记录持久化的原始请求(含筛选条件/传输选项)重新执行传输,
// 不重新绑定参数,保证与首次提交完全一致。
func (h *TransferHandler) Rerun(c *gin.Context) {
	id := c.Param("id")
	rec, err := h.recordStore.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "record not found"})
		return
	}
	if rec.Payload == "" {
		// 兼容旧记录(未持久化 payload):用记录的源/目标/方向构造最小请求
		req := &transferRequest{
			SourcePath: rec.FileSource,
			DestPath:   rec.DestPath,
			Direction:  rec.Direction,
		}
		h.submit(c, req)
		return
	}
	var req transferRequest
	if err := json.Unmarshal([]byte(rec.Payload), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "corrupt record payload"})
		return
	}
	h.submit(c, &req)
}

func (h *TransferHandler) updateOpStatus(ctx context.Context, recordID string) {
	if recordID == "" || h.History == nil || h.recordStore == nil {
		return
	}
	rec, err := h.recordStore.Get(ctx, recordID)
	if err != nil {
		return
	}
	var status string
	switch rec.Status {
	case store.TransferCompleted:
		status = "completed"
	case store.TransferFailed, store.TransferPartialSuccess:
		status = "failed"
	default:
		return
	}
	if err := h.History.UpdateOperationStatus(ctx, recordID, status); err != nil {
		log.Printf("update op status: %v", err)
	}
	if h.Hub != nil {
		h.Hub.BroadcastHistoryUpdate()
	}
}
