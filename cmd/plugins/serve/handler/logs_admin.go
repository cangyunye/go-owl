package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/internal/logfile"
	"github.com/gin-gonic/gin"
)

// ExecLogsAdminHandler 命令执行日志的运维管理（admin）：
// executions 目录按批次累积节点日志（zip 下载为流式打包，落盘的是这些
// 批次目录），此前没有任何清理机制，这里提供占用查看与手动/定期清理。
type ExecLogsAdminHandler struct {
	db *sql.DB // settings 表：logs.executions_retention_days
}

func NewExecLogsAdminHandler(db *sql.DB) *ExecLogsAdminHandler {
	return &ExecLogsAdminHandler{db: db}
}

// Summary GET /api/v1/logs/executions：全部批次的文件数与字节占用汇总。
func (h *ExecLogsAdminHandler) Summary(c *gin.Context) {
	sum, err := logfile.SummarizeExecutions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "summarize executions failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sum})
}

// Cleanup DELETE /api/v1/logs/executions：清理执行日志批次目录。
// 保留期取 older_than_days 查询参数，缺省读 settings 的
// logs.executions_retention_days（默认 30 天）；all=1 无视保留期全量清空。
func (h *ExecLogsAdminHandler) Cleanup(c *gin.Context) {
	var olderThan time.Duration

	if c.Query("all") == "1" || strings.EqualFold(c.Query("all"), "true") {
		olderThan = 0 // 0 = 立即全部过期
	} else {
		days := 0
		if raw := strings.TrimSpace(c.Query("older_than_days")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || n > 3650 {
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "older_than_days must be 0-3650"})
				return
			}
			days = n
		} else {
			days = ExecLogsRetentionDays(h.db)
		}
		olderThan = time.Duration(days) * 24 * time.Hour
	}

	removed, reclaimed, err := logfile.CleanupExecutions(olderThan)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "cleanup executions failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"removed":         removed,
		"reclaimed_bytes": reclaimed,
	}})
}

// ExecLogsRetentionDays 读取执行日志清理保留天数（settings 的
// logs.executions_retention_days）；未设置/非法回退默认 30，0 = 关闭定期清理。
// serve 的定期清理与手动端点共用此取值。
func ExecLogsRetentionDays(db *sql.DB) int {
	const def = 30
	if db == nil {
		return def
	}
	var v string
	if err := db.QueryRow(
		`SELECT value FROM settings WHERE key = 'logs.executions_retention_days'`).Scan(&v); err != nil {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 || n > 3650 {
		return def
	}
	return n
}
