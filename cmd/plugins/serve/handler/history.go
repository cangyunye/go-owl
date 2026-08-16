package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type HistoryHandler struct {
	history *store.HistoryStore
}

func NewHistoryHandler(history *store.HistoryStore) *HistoryHandler {
	return &HistoryHandler{history: history}
}

func parseHistoryDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	suffix := s[len(s)-1]
	switch suffix {
	case 'h', 'H':
		hours, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(hours * float64(time.Hour)), nil
	case 'd', 'D':
		days, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	default:
		return time.ParseDuration(s)
	}
}

func (h *HistoryHandler) parseOptions(c *gin.Context) *store.QueryOptions {
	opts := &store.QueryOptions{
		TaskID:  c.Query("task_id"),
		NodeID:  c.Query("node_id"),
		OpType:  c.Query("op_type"),
		Status:  c.Query("status"),
		Command: c.Query("command"),
		User:    c.Query("user"),
	}
	if v := c.Query("last"); v != "" {
		if d, err := parseHistoryDuration(v); err == nil && d > 0 {
			opts.StartTime = time.Now().UTC().Add(-d)
		}
	}
	if v := c.Query("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.StartTime = t
		}
	}
	if v := c.Query("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.EndTime = t
		}
	}
	opts.Limit = 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		if v > 1000 {
			v = 1000
		}
		opts.Limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		opts.Offset = v
	}
	return opts
}

func (h *HistoryHandler) List(c *gin.Context) {
	opts := h.parseOptions(c)
	records, total, err := h.history.Query(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "query failed"})
		return
	}
	if records == nil {
		records = []*store.Record{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data": records,
		"meta": gin.H{"total": total, "limit": opts.Limit, "offset": opts.Offset},
	})
}

func (h *HistoryHandler) Get(c *gin.Context) {
	taskID := c.Param("task_id")
	rec, err := h.history.GetByTaskID(c.Request.Context(), taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "record not found"})
		return
	}
	c.JSON(http.StatusOK, rec)
}

func (h *HistoryHandler) Stats(c *gin.Context) {
	st, err := h.history.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "stats failed"})
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *HistoryHandler) Export(c *gin.Context) {
	opts := h.parseOptions(c)
	opts.Limit = 1000
	opts.Offset = 0
	records, _, err := h.history.Query(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "export failed"})
		return
	}
	if records == nil {
		records = []*store.Record{}
	}
	ts := time.Now().UTC().Format("20060102-150405")
	switch c.DefaultQuery("format", "json") {
	case "yaml", "yml":
		data, err := yaml.Marshal(records)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "marshal failed"})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=history-%s.yaml", ts))
		c.Data(http.StatusOK, "application/x-yaml", data)
	default:
		data, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "marshal failed"})
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=history-%s.json", ts))
		c.Data(http.StatusOK, "application/json", data)
	}
}

func (h *HistoryHandler) Clean(c *gin.Context) {
	days, err := strconv.Atoi(c.Query("days"))
	if err != nil || days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "days must be a positive integer"})
		return
	}
	deleted, err := h.history.Cleanup(c.Request.Context(), days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cleanup failed"})
		return
	}
	logsRemoved := cleanExecutionLogs(days)
	c.JSON(http.StatusOK, gin.H{"deleted": deleted, "logs_removed": logsRemoved})
}

// cleanExecutionLogs 删除 ~/.owl/logs/executions 下最后写入早于 N 天的批次目录,
// 与历史 DB 清理保持一致。
func cleanExecutionLogs(days int) int {
	cutoff := time.Now().AddDate(0, 0, -days)
	dir := logfile.ExecutionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}
	return removed
}
