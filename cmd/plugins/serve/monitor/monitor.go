// Package monitor 是 go-owl 监控体系在 owl-serve 插件中的集成层：
// 提供节点目标源、引擎生命周期与静默配置，接线 internal/monitor 核心。
package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	owlmonitor "github.com/cangyunye/go-owl/internal/monitor"
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
func (s *NodesTargetSource) ListTargets() ([]owlmonitor.Target, error) {
	rows, err := s.db.Query(`SELECT id, name, address, port, user,
		COALESCE(password, ''), COALESCE(ssh_key, ''), COALESCE(proxy_jump, '')
		FROM nodes`)
	if err != nil {
		return nil, fmt.Errorf("monitor: 读取节点列表失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var targets []owlmonitor.Target
	for rows.Next() {
		var t owlmonitor.Target
		if err := rows.Scan(&t.ID, &t.Name, &t.Address, &t.Port, &t.User,
			&t.SSHPassword, &t.SSHKey, &t.ProxyJump); err != nil {
			return nil, fmt.Errorf("monitor: 扫描节点失败: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// Service 监控服务（serve 集成）：持有核心组件与生命周期。
type Service struct {
	Store      *owlmonitor.Store
	Engine     *owlmonitor.Engine
	Manager    *owlmonitor.AlertManager
	Dispatcher *owlmonitor.Dispatcher
	runner     *owlmonitor.RunExecutor
	db         *sql.DB // settings 表访问（静默配置）
	webURL     string

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

	// 静默配置存于 settings 表（monitor.silence_until，0 = 不静默）
	cfg := owlmonitor.EngineConfig{
		Interval:      time.Minute,
		RetentionDays: 30,
		Concurrency:   10,
		SilenceUntil:  func() int64 { return readSilenceUntil(db) },
		WebURL:        webURL,
	}
	engine := owlmonitor.NewEngine(cfg, store, collector, source, manager, dispatcher)

	return &Service{
		Store:      store,
		Engine:     engine,
		Manager:    manager,
		Dispatcher: dispatcher,
		runner:     owlmonitor.NewRunExecutor(owlmonitor.NewSSHExecerFactory(), resolveTarget(db)),
		db:         db,
		webURL:     webURL,
	}, nil
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

func writeSilenceUntil(db *sql.DB, until int64) error {
	_, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('monitor.silence_until', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, strconv.FormatInt(until, 10))
	if err != nil {
		return fmt.Errorf("monitor: 写入静默配置失败: %w", err)
	}
	return nil
}
