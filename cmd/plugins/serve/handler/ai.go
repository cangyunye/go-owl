package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/gin-gonic/gin"
)

type AIHandler struct {
	db *sql.DB
}

type chatRequest struct {
	Message string `json:"message"`
	Session string `json:"session,omitempty"`
}

type chatResponse struct {
	Intent   string `json:"intent"`
	Reply    string `json:"reply"`
	Action   string `json:"action,omitempty"`
	ActionID string `json:"action_id,omitempty"`
}

func NewAIHandler(db *sql.DB) *AIHandler {
	return &AIHandler{db: db}
}

func (h *AIHandler) Chat(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "message is required"})
		return
	}

	classifier := ai.NewIntentClassifier()
	intent := classifier.Classify(req.Message)
	reply := h.buildReply(intent, req.Message)

	c.JSON(http.StatusOK, chatResponse{
		Intent: intent.String(),
		Reply:  reply,
	})
}

func (h *AIHandler) buildReply(intent ai.Intent, msg string) string {
	// Use the same intent-response patterns from the design prototype
	t := strings.ToLower(msg)

	switch intent {
	case ai.QueryNodes:
		return "当前节点查询功能已就绪，可在「节点管理」页面查看完整列表。"
	case ai.ExecuteCommand:
		return "命令执行功能已就绪，请前往「命令执行」页面操作，或直接在下方输入要执行的命令。"
	case ai.GeneratePlaybook:
		return "剧本管理功能已就绪，请前往「剧本管理」页面创建和运行剧本。"
	case ai.TransferFile:
		return "文件传输功能已就绪，请前往「文件传输」页面操作。"
	default:
		return "我是 OWL 运维助手。我可以帮你管理节点、执行命令、运行剧本和传输文件。请尝试输入具体指令或点击快捷建议。"
	}
}

type NodeStoreBridge struct {
	db *sql.DB
}

func (n *NodeStoreBridge) GetNodeNames() ([]string, error) {
	rows, err := n.db.Query("SELECT name FROM nodes WHERE name != '' AND name IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		names = append(names, name)
	}
	return names, nil
}

func (h *AIHandler) Status(_ context.Context) (int, int, int, error) {
	var total, online, offline int
	h.db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&total)
	h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'online'").Scan(&online)
	h.db.QueryRow("SELECT COUNT(*) FROM nodes WHERE status = 'offline' OR status = 'unknown'").Scan(&offline)
	return total, online, offline, nil
}
