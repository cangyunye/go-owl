// Package monitor 是 go-owl 监控体系在 owl-serve 插件中的集成层：
// 提供节点目标源、引擎生命周期与静默配置，接线 internal/monitor 核心。
package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	ai2 "github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/logger"
	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
	"github.com/cangyunye/go-owl/internal/secrets"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// NodesTargetSource 从 serve 的 nodes 表读取采集目标（含 SSH 凭据）。
type NodesTargetSource struct {
	db *sql.DB
}

// NewNodesTargetSource 创建节点目标源。
func NewNodesTargetSource(db *sql.DB) *NodesTargetSource {
	return &NodesTargetSource{db: db}
}

// ListTargets 列出全部已注册节点为采集目标。
// label monitor.services（逗号分隔的 unit 名）映射为采集的受监控服务列表。
// 同一 address:port 登记多个节点（不同用户）时只保留一个采集目标，
// 优先 user=root 的条目，避免同一台机器被不同用户重复 SSH 查询；
// 被合并的节点 ID 记 warn 日志。
func (s *NodesTargetSource) ListTargets() ([]owlmonitor.Target, error) {
	rows, err := s.db.Query(`SELECT id, name, address, port, user,
		COALESCE(password, ''), COALESCE(ssh_key, ''), COALESCE(proxy_jump, ''),
		COALESCE(labels, '{}'), COALESCE(groups, '[]')
		FROM nodes ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("monitor: 读取节点列表失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type dedupKey struct {
		addr string
		port int
	}
	var targets []owlmonitor.Target
	merged := map[int][]string{} // 代表目标下标 → 被合并的节点 ID
	index := map[dedupKey]int{}
	for rows.Next() {
		var t owlmonitor.Target
		var labels, groups string
		if err := rows.Scan(&t.ID, &t.Name, &t.Address, &t.Port, &t.User,
			&t.SSHPassword, &t.SSHKey, &t.ProxyJump, &labels, &groups); err != nil {
			return nil, fmt.Errorf("monitor: 扫描节点失败: %w", err)
		}
		t.Services = servicesFromLabels(labels)
		// groups JSON 解析与非法归一收敛到 store 包（labels 解析为
		// monitor.services 列表属监控域逻辑，保留本地）
		t.Groups = store.ParseNodeGroups(groups)
		if pw, err := secrets.Decrypt(t.SSHPassword); err != nil {
			return nil, fmt.Errorf("monitor: 节点 %s 凭据解密失败: %w", t.ID, err)
		} else {
			t.SSHPassword = pw
		}
		if key, err := secrets.Decrypt(t.SSHKey); err != nil {
			return nil, fmt.Errorf("monitor: 节点 %s 凭据解密失败: %w", t.ID, err)
		} else {
			t.SSHKey = key
		}
		k := dedupKey{t.Address, t.Port}
		if i, ok := index[k]; ok {
			// 已有代表目标：root 用户优先担任代表
			if t.User == "root" && targets[i].User != "root" {
				prev := targets[i].ID
				ids := append([]string{prev}, merged[i]...)
				merged[i] = append(ids, t.ID)
				targets[i] = t
			} else {
				merged[i] = append(merged[i], t.ID)
			}
			continue
		}
		index[k] = len(targets)
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, ids := range merged {
		logger.Warn("同地址节点合并采集（仅采集代表节点）",
			logger.WithOperation("monitor_targets"),
			logger.WithField("representative", targets[i].ID),
			logger.WithField("merged_nodes", strings.Join(ids, ",")))
	}
	return targets, nil
}

// servicesFromLabels 从节点 labels JSON 中读取 monitor.services
// （逗号分隔的受监控服务 unit 列表），非法 JSON 视为未配置。
func servicesFromLabels(raw string) []string {
	if raw == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	v := strings.TrimSpace(m["monitor.services"])
	if v == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Service 监控服务（serve 集成）：持有核心组件与生命周期。
type Service struct {
	Store          *owlmonitor.Store
	Engine         *owlmonitor.Engine
	Manager        *owlmonitor.AlertManager
	Dispatcher     *owlmonitor.Dispatcher
	runner         *owlmonitor.RunExecutor
	playbookRunner PlaybookRunner                                                           // 剧本执行入口（告警绑定执行用，装配时注入）
	debugExec      func(nodeID, command string, timeout time.Duration) (string, int, error) // 规则调试执行
	db             *sql.DB                                                                  // settings 表访问（静默配置）
	webURL         string
	// 疗效未确认时的自动换方案重试（monitor.heal_retry_*，默认关闭）
	healRetryEnabled bool
	healRetryLimit   int
	verifier         *owlmonitor.RunVerifier // 绑定执行的疗效验证（Setup 注入）

	cancel context.CancelFunc
}

// Setup 初始化监控服务：复用 serve 的 owl.db 文件（WAL 并发安全），
// 内置节点目标源与 SSH 采集器。不启动后台循环（由 Start 控制）。
func Setup(dbPath string, db *sql.DB, webURL string) (*Service, error) {
	store, err := owlmonitor.OpenStore(dbPath)
	if err != nil {
		return nil, err
	}

	source := NewNodesTargetSource(db)
	collector := owlmonitor.NewCollector(owlmonitor.NewSSHExecerFactory())
	manager := owlmonitor.NewAlertManager(store)
	dispatcher := owlmonitor.NewDispatcher(store)
	runner := owlmonitor.NewRunExecutor(owlmonitor.NewSSHExecerFactory(), resolveTarget(db))

	// 自愈管线：配置可用时用 AI 建议器（LLM 失败自动降级规则兜底）
	advisor := newAdvisor(store)
	healer := owlmonitor.NewAutoHealer(store, advisor, runner)

	// 静默配置存于 settings 表（monitor.silence_until，0 = 不静默）；
	// warn 升级时长存于 settings 表（monitor.escalate_after_minutes，浮点分钟，
	// 未设置/非法 = 默认 1h），每轮采集前重读，支持运行期调整；
	// 重复告警合并窗口存于 settings 表（monitor.realert_window_minutes，
	// 浮点分钟，未设置/非法 = 默认 24h，0 = 关闭合并）
	cfg := owlmonitor.EngineConfig{
		Interval:      time.Minute,
		RetentionDays: 30,
		Concurrency:   10,
		SilenceUntil:  func() int64 { return readSilenceUntil(db) },
		EscalateAfter: func() time.Duration {
			return time.Duration(readEscalateAfterMinutes(db) * float64(time.Minute))
		},
		RealertWindow: func() time.Duration {
			return time.Duration(readRealertWindowMinutes(db) * float64(time.Minute))
		},
		Enabled:            func() bool { return readMonitorEnabled(db) },
		CollectWindow:      func() string { return readCollectWindow(db) },
		AlertRetentionDays: func() int { return readAlertRetentionDays(db) },
		WebURL:             webURL,
	}
	applyStorageSettings(db, &cfg)
	// 失败自动回滚开关（monitor.rollback_enabled，默认开）
	runner.SetRollbackEnabled(readRollbackEnabled(db))
	// 疗效未确认自动换方案（monitor.heal_retry_*，默认关闭 + 次数上限防循环）
	svcHealRetryEnabled := readHealRetryEnabled(db)
	svcHealRetryLimit := readHealRetryLimit(db)

	// 恢复闭环验证：处置计划完成后定向采集并重新评估告警规则（monitor.verify_*），
	// svc 指针先行声明（回调闭包引用），稍后赋值
	// 结果写入 remedy_run_verifications 供前端展示；验证失败不阻断主链路
	var svc *Service
	var verifier *owlmonitor.RunVerifier
	if readVerifyEnabled(db) {
		verifier = owlmonitor.NewRunVerifier(collector, resolveTarget(db), store)
		verifier.SetDelay(time.Duration(readVerifyDelaySeconds(db)) * time.Second)
		runner.OnRunFinished = func(run *owlmonitor.RemedyRun) {
			ver, err := verifier.VerifyRun(context.Background(), run.ID)
			if err != nil {
				log.Printf("monitor: 处置验证失败 run=%s: %v", run.ID, err)
				return
			}
			// 疗效确认：对计划中成功执行的对策累加 healed_count（推荐排序加权）
			if ver != nil && ver.Status == owlmonitor.VerifyRecovered {
				if fresh, exists, err := store.GetRemedyRun(run.ID); err == nil && exists {
					seen := map[string]bool{}
					for i := range fresh.Steps {
						st := &fresh.Steps[i]
						if st.Kind == "script" && st.Status == owlmonitor.StepSuccess && st.RemedyID != "" && !seen[st.RemedyID] {
							seen[st.RemedyID] = true
							if err := store.IncrementRemedyHealed(st.RemedyID); err != nil {
								log.Printf("monitor: 疗效计数失败 remedy=%s: %v", st.RemedyID, err)
							}
						}
					}
				}
			}
			// 疗效未确认：发送 verify_failed 通知（复用告警渠道过滤）
			if ver != nil && ver.Status == owlmonitor.VerifyNotRecovered {
				if alert, exists, err := store.GetAlert(ver.AlertID); err == nil && exists {
					if at, ok, err := store.GetAlertType(alert.AlertTypeID); err == nil && ok {
						if target, err := resolveTarget(db)(run.NodeID); err == nil {
							notifyURL := strings.TrimSuffix(webURL, "/") + "/alerts/" + alert.ID
							for _, e := range dispatcher.NotifyVerification(context.Background(), alert, at, run, ver, target.Name, notifyURL) {
								log.Printf("monitor: 疗效通知失败: %v", e)
							}
						}
						// 疗效未确认：按配置自动换方案重试（默认关闭）
						go func(alert *owlmonitor.Alert, at owlmonitor.AlertType, run *owlmonitor.RemedyRun, ver *owlmonitor.RunVerification) {
							defer func() { _ = recover() }()
							svc.maybeRetryRemedy(alert, at, run, ver)
						}(alert, at, run, ver)
					}
				}
			}
		}
	}

	engine := owlmonitor.NewEngine(cfg, store, collector, source, manager, dispatcher)
	engine.SetAutoHealer(healer)

	// 启动对账：重启导致执行 goroutine 丢失的绑定运行记录标记为失败
	if err := store.FailStaleAlertBindingRuns(); err != nil {
		log.Printf("monitor: 对账悬挂绑定运行失败: %v", err)
	}

	svc = &Service{
		Store:            store,
		Engine:           engine,
		Manager:          manager,
		Dispatcher:       dispatcher,
		runner:           runner,
		db:               db,
		webURL:           webURL,
		healRetryEnabled: svcHealRetryEnabled,
		healRetryLimit:   svcHealRetryLimit,
	}
	svc.verifier = verifier
	// 调试执行入口：告警规则的「执行一次调试」（handler 经 DebugCheck 调用）
	svc.debugExec = func(nodeID, command string, timeout time.Duration) (string, int, error) {
		t, err := resolveTarget(db)(nodeID)
		if err != nil {
			return "", -1, err
		}
		return collector.ExecCommand(context.Background(), t, command, timeout)
	}
	// 告警打开/重开钩子：执行 alert_bindings 中 auto_exec 的专属指令
	engine.OnAlertOpened = svc.handleAlertOpened
	return svc, nil
}

// newAdvisor 按 ~/.owl/config.yaml 创建处置建议器：
// AI 配置就绪 → AIAdvisor（LLM 失败降级规则兜底）；否则规则推荐。
func newAdvisor(store *owlmonitor.Store) owlmonitor.Advisor {
	ruleBased := owlmonitor.NewRuleBasedAdvisor(store, 3)

	home, err := os.UserHomeDir()
	if err != nil {
		return ruleBased
	}
	cfg, err := ai2.LoadConfig(filepath.Join(home, ".owl", "config.yaml"))
	if err != nil || cfg.AI.APIKey == "" {
		return ruleBased
	}
	client, err := ai2.CreateLLMClient(cfg)
	if err != nil {
		return ruleBased
	}
	return owlmonitor.NewAIAdvisor(client, store, 3)
}

// resolveTarget 按节点 ID 查询 nodes 表构造执行目标（含 SSH 凭据）。
func resolveTarget(db *sql.DB) owlmonitor.TargetResolver {
	return func(nodeID string) (*owlmonitor.Target, error) {
		row := db.QueryRow(`SELECT id, name, address, port, user,
			COALESCE(password, ''), COALESCE(ssh_key, ''), COALESCE(proxy_jump, '')
			FROM nodes WHERE id = ?`, nodeID)
		var t owlmonitor.Target
		if err := row.Scan(&t.ID, &t.Name, &t.Address, &t.Port, &t.User,
			&t.SSHPassword, &t.SSHKey, &t.ProxyJump); err != nil {
			return nil, fmt.Errorf("monitor: 节点 %s 不存在或缺少连接信息: %w", nodeID, err)
		}
		if pw, err := secrets.Decrypt(t.SSHPassword); err != nil {
			return nil, fmt.Errorf("monitor: 节点 %s 凭据解密失败: %w", nodeID, err)
		} else {
			t.SSHPassword = pw
		}
		if key, err := secrets.Decrypt(t.SSHKey); err != nil {
			return nil, fmt.Errorf("monitor: 节点 %s 凭据解密失败: %w", nodeID, err)
		} else {
			t.SSHKey = key
		}
		return &t, nil
	}
}

// StartRemedyRun 基于告警与对策快照创建处置计划并异步串行执行。
func (s *Service) StartRemedyRun(alertID string, remedyIDs []string, stopOnError bool, createdBy string) (*owlmonitor.RemedyRun, error) {
	alert, exists, err := s.Store.GetAlert(alertID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("告警 %s 不存在", alertID)
	}

	// 去重 + 校验对策属于该告警类型
	seen := map[string]bool{}
	steps := make([]owlmonitor.RemedyStep, 0, len(remedyIDs))
	for _, id := range remedyIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		rm, ok, err := s.Store.GetRemedy(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("对策 %s 不存在", id)
		}
		if rm.AlertTypeID != alert.AlertTypeID {
			return nil, fmt.Errorf("对策 %s 属于 %s，不适用于告警 %s", id, rm.AlertTypeID, alert.AlertTypeID)
		}
		steps = append(steps, owlmonitor.RemedyStep{
			Order:    len(steps),
			RemedyID: id,
			Name:     rm.Name,
			Kind:     rm.Kind,
			Content:  rm.Content,
			Rollback: rm.Rollback,
			Status:   owlmonitor.StepPending,
			NodeID:   alert.NodeID,
		})
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("未选择有效对策")
	}

	now := time.Now().Unix()
	run := &owlmonitor.RemedyRun{
		ID:          fmt.Sprintf("RUN-%d", now),
		AlertID:     alertID,
		NodeID:      alert.NodeID,
		Status:      owlmonitor.RunPending,
		StopOnError: stopOnError,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
		Steps:       steps,
	}
	if err := s.Store.CreateRemedyRun(run); err != nil {
		return nil, err
	}
	go func() { _ = s.runner.ExecuteRun(context.Background(), run.ID, s.Store) }()
	return run, nil
}

// ExecuteRemedyRun 异步执行处置计划（审批恢复/拒绝收尾用）。
func (s *Service) ExecuteRemedyRun(runID string) {
	go func() { _ = s.runner.ExecuteRun(context.Background(), runID, s.Store) }()
}

// WebURL 返回告警处理入口链接前缀。
func (s *Service) WebURL() string {
	return s.webURL
}

// Start 后台启动采集循环。
func (s *Service) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go func() { _ = s.Engine.Run(ctx) }()
}

// Stop 停止采集循环。
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Close 停止采集循环并关闭指标库连接（进程退出前调用）。
func (s *Service) Close() {
	s.Stop()
	if s.Store != nil {
		if err := s.Store.Close(); err != nil {
			log.Printf("monitor: 关闭指标库失败: %v", err)
		}
	}
}

// GetSilenceUntil 读取静默截止时间戳（0 = 不静默）。
func (s *Service) GetSilenceUntil() int64 {
	return readSilenceUntil(s.db)
}

// SetSilenceUntil 设置静默截止时间戳（0 = 取消静默）。
func (s *Service) SetSilenceUntil(until int64) error {
	return writeSilenceUntil(s.db, until)
}

func readSilenceUntil(db *sql.DB) int64 {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.silence_until'`).Scan(&v); err != nil {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// readEscalateAfterMinutes 读取 warn 升级时长（浮点分钟；未设置/非法/<=0
// 返回 0，调用方据此保持默认值）。支持小数便于测试（如 0.05 = 3 秒）。
func readEscalateAfterMinutes(db *sql.DB) float64 {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.escalate_after_minutes'`).Scan(&v); err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return f
}

// readRealertWindowMinutes 读取重复告警合并窗口（浮点分钟）。
// 未设置/非法/负数 = 默认 24h（1440 分钟）；显式 0 = 关闭合并。
func readRealertWindowMinutes(db *sql.DB) float64 {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.realert_window_minutes'`).Scan(&v); err != nil {
		return 1440
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f < 0 {
		return 1440
	}
	return f
}

func writeSilenceUntil(db *sql.DB, until int64) error {
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('monitor.silence_until', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, strconv.FormatInt(until, 10))
	if err != nil {
		return fmt.Errorf("monitor: 写入静默配置失败: %w", err)
	}
	return nil
}

// readMonitorEnabled 读取监控总开关（monitor.enabled）。未设置 = 开启；
// "false"/"0"/"no"/"off"（大小写不敏感）= 关闭。
func readMonitorEnabled(db *sql.DB) bool {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.enabled'`).Scan(&v); err != nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// readCollectWindow 读取采集时段窗口（monitor.collect_window，"HH:MM-HH:MM"）。
// 未设置 = 全天采集。
func readCollectWindow(db *sql.DB) string {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.collect_window'`).Scan(&v); err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// readAlertRetentionDays 读取告警记录保留天数（monitor.alert_retention_days）。
// 未设置/非法/负数 = 0（不启用清理）。
// readHealRetryEnabled 读取 monitor.heal_retry_enabled（默认 false，保守关闭）。
func readHealRetryEnabled(db *sql.DB) bool {
	var v string
	_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.heal_retry_enabled'`).Scan(&v)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// readHealRetryLimit 读取 monitor.heal_retry_limit（默认 1，上限 5）。
func readHealRetryLimit(db *sql.DB) int {
	var v string
	_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.heal_retry_limit'`).Scan(&v)
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 1
	}
	if n > 5 {
		n = 5
	}
	return n
}

// readRollbackEnabled 读取 monitor.rollback_enabled（默认 true）。
func readRollbackEnabled(db *sql.DB) bool {
	var v string
	_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.rollback_enabled'`).Scan(&v)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// readVerifyEnabled 读取 monitor.verify_enabled（默认 true）。
func readVerifyEnabled(db *sql.DB) bool {
	var v string
	_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.verify_enabled'`).Scan(&v)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// readVerifyDelaySeconds 读取 monitor.verify_delay_seconds（处置生效等待，默认 30）。
func readVerifyDelaySeconds(db *sql.DB) int {
	var v string
	_ = db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.verify_delay_seconds'`).Scan(&v)
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 30
	}
	if n > 600 {
		n = 600
	}
	return n
}

func readAlertRetentionDays(db *sql.DB) int {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.alert_retention_days'`).Scan(&v); err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// applyStorageSettings 把指标保留期、清理档位与采集间隔（写入频率）接入
// 引擎配置：每轮/每次清理前经 settings 表重读，运行期调整即时生效。
func applyStorageSettings(db *sql.DB, cfg *owlmonitor.EngineConfig) {
	cfg.RetentionDaysFn = func() int { return readMetricsRetentionDays(db) }
	cfg.IntervalFn = func() time.Duration {
		return time.Duration(readIntervalSeconds(db)) * time.Second
	}
	cfg.CleanupScheduleFn = func() string { return readCleanupSchedule(db) }
}

// readMetricsRetentionDays 读取指标保留天数（monitor.retention_days）。
// 未设置/非法 = 默认 30；越界收敛到 1..3650。
func readMetricsRetentionDays(db *sql.DB) int {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.retention_days'`).Scan(&v); err != nil {
		return 30
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 30
	}
	if n < 1 {
		return 1
	}
	if n > 3650 {
		return 3650
	}
	return n
}

// readCleanupSchedule 读取清理计划档位（monitor.cleanup_schedule）。
// 未设置/非法 = daily；大小写与首尾空白归一。
func readCleanupSchedule(db *sql.DB) string {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.cleanup_schedule'`).Scan(&v); err != nil {
		return "daily"
	}
	switch s := strings.ToLower(strings.TrimSpace(v)); s {
	case "weekly", "monthly":
		return s
	default:
		return "daily"
	}
}

// readIntervalSeconds 读取采集间隔秒数（monitor.interval_seconds）。
// 未设置/非法 = 默认 60；越界收敛到 10..86400。
func readIntervalSeconds(db *sql.DB) int {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'monitor.interval_seconds'`).Scan(&v); err != nil {
		return 60
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 60
	}
	if n < 10 {
		return 10
	}
	if n > 86400 {
		return 86400
	}
	return n
}
