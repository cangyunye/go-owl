package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/gin-gonic/gin"
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
		TaskID: c.Query("task_id"),
		NodeID: c.Query("node_id"),
		OpType: c.Query("op_type"),
		Status: c.Query("status"),
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
