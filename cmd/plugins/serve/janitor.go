package serve

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/handler"
	"github.com/cangyunye/go-owl/internal/logfile"
)

// startJanitor 启动每日定期清理（ctx 取消即停止）：serve 启动时先清一次，
// 此后每 24h 一次。各保留期每次运行时从 settings 重读，改完即时生效：
// - 执行日志批次：logs.executions_retention_days（默认 30，0 = 关闭）
// - owl.db 历史表：history.retention_days（默认 90，0 = 关闭）
func (s *Server) startJanitor(ctx context.Context) {
	go func() {
		s.runExecLogsCleanupOnce("startup")
		s.runHistoryCleanupOnce(ctx, "startup")
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runExecLogsCleanupOnce("daily")
				s.runHistoryCleanupOnce(ctx, "daily")
			}
		}
	}()
}

// runExecLogsCleanupOnce 清理过期的执行日志批次目录。
func (s *Server) runExecLogsCleanupOnce(trigger string) {
	days := handler.ExecLogsRetentionDays(s.DB)
	if days <= 0 {
		return
	}
	removed, reclaimed, err := logfile.CleanupExecutions(time.Duration(days) * 24 * time.Hour)
	if err != nil {
		log.Printf("janitor: 执行日志清理失败(%s): %v", trigger, err)
		return
	}
	if removed > 0 {
		log.Printf("janitor: 执行日志清理(%s) 删除 %d 个批次，回收 %d 字节", trigger, removed, reclaimed)
	}
}

// runHistoryCleanupOnce 清理 owl.db 历史表（operations/command_executions/
// file_transfers）中超过保留期的行——命令输出同时存文件与 DB 两份，
// 两边都要按各自保留期回收，否则库体积只涨不缩。
func (s *Server) runHistoryCleanupOnce(ctx context.Context, trigger string) {
	if s.History == nil {
		return
	}
	days := readHistoryRetentionDays(s.DB)
	if days <= 0 {
		return
	}
	deleted, err := s.History.Cleanup(ctx, days)
	if err != nil {
		log.Printf("janitor: 历史记录清理失败(%s): %v", trigger, err)
		return
	}
	if deleted > 0 {
		log.Printf("janitor: 历史记录清理(%s) 删除 %d 行", trigger, deleted)
	}
}

// readHistoryRetentionDays 读取历史表保留天数（settings 的
// history.retention_days）；未设置/非法回退默认 90，0 = 关闭定期清理。
func readHistoryRetentionDays(db *sql.DB) int {
	const def = 90
	if db == nil {
		return def
	}
	var v string
	if err := db.QueryRow(
		`SELECT value FROM settings WHERE key = 'history.retention_days'`).Scan(&v); err != nil {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 || n > 3650 {
		return def
	}
	return n
}
