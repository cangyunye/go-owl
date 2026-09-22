package serve

import (
	"context"
	"log"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/handler"
	"github.com/cangyunye/go-owl/internal/logfile"
)

// startExecLogsJanitor 启动执行日志的每日定期清理（ctx 取消即停止）：
// serve 启动时先清一次，此后每 24h 一次。保留期每次运行时从 settings
// 重读（logs.executions_retention_days，默认 30 天，0 = 关闭定期清理，
// 仅保留 admin 手动清理端点）。
func (s *Server) startExecLogsJanitor(ctx context.Context) {
	go func() {
		s.runExecLogsCleanupOnce("startup")
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runExecLogsCleanupOnce("daily")
			}
		}
	}()
}

// runExecLogsCleanupOnce 执行一轮过期批次清理；清理属后台尽力而为，
// 失败只记日志不中断服务。
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
