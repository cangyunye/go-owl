package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"
)

type TransferHandler struct {
	db          *sql.DB
	task        *store.TaskStore
	recordStore *store.TransferRecordStore
}

type transferRequest struct {
	Action      string   `json:"action"` // "upload" or "download"
	NodeIDs     []string `json:"node_ids"`
	SourcePath  string   `json:"source_path"`
	DestPath    string   `json:"dest_path"`
	Direction   string   `json:"direction"` // "push" or "pull"
}

type transferResponse struct {
	NodeID string `json:"node_id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func NewTransferHandler(db *sql.DB, ts *store.TaskStore, rs *store.TransferRecordStore) *TransferHandler {
	return &TransferHandler{db: db, task: ts, recordStore: rs}
}

func (h *TransferHandler) Create(c *gin.Context) {
	var req transferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid request body"})
		return
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

	transferRec, err := h.recordStore.Create(c.Request.Context(), req.SourcePath, req.DestPath, direction)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "create record failed"})
		return
	}
	h.recordStore.SetNodeCount(c.Request.Context(), transferRec.ID, len(req.NodeIDs))

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

		go func(nodeID, src, dst, dir string, conn *nodeSSHInfo, recordID string) {
			bg := context.Background()
			err := scpTransfer(bg, conn, src, dst, dir)
			taskStatus := store.TaskStatusCompleted
			errMsg := ""
			if err != nil {
				taskStatus = store.TaskStatusFailed
				errMsg = err.Error()
			}
			output := errMsg
			if output == "" {
				output = fmt.Sprintf("transfer %s -> %s completed", src, dst)
			}
			h.task.UpdateStatus(bg, rec.ID, taskStatus, output, nil)
			h.recordStore.UpdateNodeResult(bg, recordID, err == nil)
		}(nid, req.SourcePath, req.DestPath, direction, info, transferRec.ID)

		results = append(results, transferResponse{NodeID: nid, Status: "queued"})
	}

	h.recordStore.MarkRunning(c.Request.Context(), transferRec.ID)

	c.JSON(http.StatusAccepted, gin.H{"record_id": transferRec.ID, "transfers": results})
}

func resolveNodeSSH(db *sql.DB, nodeID string) (*nodeSSHInfo, error) {
	var info nodeSSHInfo
	var pw, key sql.NullString
	err := db.QueryRow(
		`SELECT address, port, user, password, ssh_key FROM nodes WHERE id = ?`, nodeID,
	).Scan(&info.Address, &info.Port, &info.User, &pw, &key)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	if pw.Valid {
		info.Password = pw.String
	}
	if key.Valid {
		info.SSHKey = key.String
	}
	return &info, nil
}

func scpTransfer(ctx context.Context, info *nodeSSHInfo, src, dst, direction string) error {
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
			return fmt.Errorf("parse ssh key: %w", err)
		}
		config.Auth = append(config.Auth, ssh.PublicKeys(signer))
	}

	addr := fmt.Sprintf("%s:%d", info.Address, info.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var cmd string
	if direction == "push" {
		// SCP from local to remote: echo content | ssh remote "cat > dest"
		// Simplified: use the session's stdin pipe to send file content
		cmd = fmt.Sprintf("scp -q %s %s@%s:%s", src, info.User, info.Address, dst)
	} else {
		cmd = fmt.Sprintf("scp -q %s@%s:%s %s", info.User, info.Address, src, dst)
	}

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("scp exec: %w", err)
	}
	return nil
}

func (h *TransferHandler) List(c *gin.Context) {
	tasks, _, err := h.task.List(c.Request.Context(), 50, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "list failed"})
		return
	}

	var transfers []*store.Task
	for _, t := range tasks {
		if len(t.Command) > 9 && t.Command[:9] == "transfer:" {
			transfers = append(transfers, t)
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": transfers})
}

func (h *TransferHandler) Records(c *gin.Context) {
	records, total, err := h.recordStore.List(c.Request.Context(), 50, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "list records failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": records, "total": total})
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
