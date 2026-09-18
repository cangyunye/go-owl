package monitor

import (
	"log"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
)

// maybeRetryRemedy 疗效验证未通过时的自动换方案：从未尝试过的推荐对策中
// 取第一个创建重试计划（CreatedBy=auto-heal-retry）。重试次数受
// monitor.heal_retry_limit 约束防循环；任何失败只记日志不阻断。
func (s *Service) maybeRetryRemedy(alert *owlmonitor.Alert, at owlmonitor.AlertType,
	run *owlmonitor.RemedyRun, ver *owlmonitor.RunVerification) {
	if ver == nil || ver.Status != owlmonitor.VerifyNotRecovered {
		return
	}
	if !s.healRetryEnabled {
		return
	}
	limit := s.healRetryLimit
	if limit <= 0 {
		limit = 1
	}

	// 收集已尝试对策 + 统计历史重试次数
	runs, err := s.Store.ListRemedyRunsByAlert(alert.ID)
	if err != nil {
		log.Printf("monitor: 换方案重试查询历史失败 alert=%s: %v", alert.ID, err)
		return
	}
	tried := map[string]bool{}
	retries := 0
	for i := range runs {
		if runs[i].CreatedBy == "auto-heal-retry" {
			retries++
		}
		// 列表查询不含步骤：单独读取完整计划收集已尝试对策
		if fresh, ok, err := s.Store.GetRemedyRun(runs[i].ID); err == nil && ok {
			for _, st := range fresh.Steps {
				if st.RemedyID != "" {
					tried[st.RemedyID] = true
				}
			}
		}
	}
	if retries >= limit {
		log.Printf("monitor: 告警 %s 自动重试已达上限 %d，跳过换方案", alert.ID, limit)
		return
	}

	recs, err := s.Store.RecommendedRemedies(alert.AlertTypeID)
	if err != nil {
		return
	}
	var candidate string
	for _, rm := range recs {
		if !tried[rm.ID] {
			candidate = rm.ID
			break
		}
	}
	if candidate == "" {
		log.Printf("monitor: 告警 %s 无未尝试的推荐对策，跳过换方案", alert.ID)
		return
	}

	if _, err := s.StartRemedyRun(alert.ID, []string{candidate}, true, "auto-heal-retry"); err != nil {
		log.Printf("monitor: 换方案重试创建失败 alert=%s remedy=%s: %v", alert.ID, candidate, err)
		return
	}
	log.Printf("monitor: 告警 %s 疗效未确认，已自动换用对策 %s 重试", alert.ID, candidate)
}
