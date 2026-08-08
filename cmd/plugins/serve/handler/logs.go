package handler

import (
	"archive/zip"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/gin-gonic/gin"
)

type LogHandler struct{}

func NewLogHandler() *LogHandler {
	return &LogHandler{}
}

func validOpID(opID string) bool {
	if opID == "" || len(opID) > 64 {
		return false
	}
	for _, r := range opID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

func (h *LogHandler) opDir(opID string) (string, bool) {
	if !validOpID(opID) {
		return "", false
	}
	return filepath.Join(logfile.ExecutionsDir(), opID), true
}

// List GET /executions/:op_id/logs 返回批次目录内各节点日志文件信息。
func (h *LogHandler) List(c *gin.Context) {
	opID := c.Param("op_id")
	if !validOpID(opID) {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid op_id"})
		return
	}
	infos, err := logfile.ListExecutionLogs(opID)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusOK, gin.H{"data": []logfile.ExecutionLogInfo{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "list logs failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": infos})
}

// Download GET /executions/:op_id/logs/:node_id 下载单节点日志文件。
func (h *LogHandler) Download(c *gin.Context) {
	opID := c.Param("op_id")
	dir, ok := h.opDir(opID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid op_id"})
		return
	}
	nodeID := c.Param("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "node_id is required"})
		return
	}
	filename := logfile.SanitizeNodeID(nodeID) + ".log"
	path := filepath.Join(dir, filename)
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "log not found"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.File(path)
}

// Archive GET /executions/:op_id/logs/archive 打包批次内全部节点日志(zip,含 manifest.json)。
func (h *LogHandler) Archive(c *gin.Context) {
	opID := c.Param("op_id")
	dir, ok := h.opDir(opID)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "invalid op_id"})
		return
	}
	if _, err := os.Stat(dir); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "execution logs not found"})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "read logs failed"})
		return
	}

	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="executions-%s.zip"`, opID))
	zw := zip.NewWriter(c.Writer)
	defer zw.Close()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		f, err := zw.Create(e.Name())
		if err != nil {
			continue
		}
		if _, err := f.Write(data); err != nil {
			continue
		}
	}
}
