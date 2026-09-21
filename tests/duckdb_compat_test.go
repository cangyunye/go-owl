//go:build duckdb
// +build duckdb

// DuckDB 兼容性验证：把 SQLite 承载的全部表对应的功能在 DuckDB 上跑一轮。
// 每个 store 一个子测试：Init（生产 DDL）→ 代表性写入 → 读回校验。
// 目标是产出兼容性矩阵：哪些表/操作可直接工作，哪些是方言缺口。
//
// 运行：go test -tags duckdb ./tests/ -run TestDuckDBCompat -v -timeout 30m
package integration

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
	"github.com/cangyunye/go-owl/cmd/plugins/serve/store"
	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/monitor"

	_ "github.com/duckdb/duckdb-go/v2"
)

func openDuck(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("duckdb", filepath.Join(t.TempDir(), "compat.duckdb"))
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// nodes/settings 的 DDL 复制自 cmd/plugins/serve/server.go 的
// initNodes/initSettings（包内未导出，兼容层仅验证同 DDL 在 DuckDB 的行为）。
func initNodesAndSettingsDDL(t *testing.T, db *sql.DB) error {
	t.Helper()
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			name TEXT,
			address TEXT,
			port INTEGER DEFAULT 22,
			user TEXT,
			password TEXT,
			ssh_key TEXT,
			status TEXT DEFAULT 'unknown',
			groups TEXT DEFAULT '[]',
			labels TEXT DEFAULT '{}',
			proxy_jump TEXT DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`)
	return err
}

func TestDuckDBCompat_ServeDomain(t *testing.T) {
	ctx := context.Background()
	duck := openDuck(t)

	t.Run("nodes_and_settings_基础表", func(t *testing.T) {
		if err := initNodesAndSettingsDDL(t, duck); err != nil {
			t.Errorf("DDL: %v", err)
			return
		}
		if _, err := duck.Exec(`INSERT INTO nodes (id, name, address, port, user, status)
			VALUES ('n1', 'node-1', '10.0.0.1', 22, 'root', 'online')`); err != nil {
			t.Errorf("insert node: %v", err)
		}
		var name string
		if err := duck.QueryRow(`SELECT name FROM nodes WHERE id='n1'`).Scan(&name); err != nil || name != "node-1" {
			t.Errorf("select node: %v (name=%q)", err, name)
		}
		// 生产 settings upsert SQL（handler/settings.go 同款）
		upsert := `INSERT INTO settings (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`
		if _, err := duck.Exec(upsert, "monitor.retention_days", "30"); err != nil {
			t.Errorf("settings upsert 1: %v", err)
		}
		if _, err := duck.Exec(upsert, "monitor.retention_days", "14"); err != nil {
			t.Errorf("settings upsert 2: %v", err)
		}
		var v string
		if err := duck.QueryRow(`SELECT value FROM settings WHERE key='monitor.retention_days'`).Scan(&v); err != nil || v != "14" {
			t.Errorf("settings readback: %v (v=%q)", err, v)
		}
	})

	t.Run("web_users", func(t *testing.T) {
		us := store.NewUserStore(duck)
		if err := us.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		if err := us.Create(ctx, &model.User{Username: "admin", Role: model.RoleAdmin, PasswordHash: "h"}); err != nil {
			t.Errorf("Create: %v", err)
		}
		u, err := us.FindByUsername(ctx, "admin")
		if err != nil || u == nil || u.Role != model.RoleAdmin {
			t.Errorf("FindByUsername: %v (u=%v)", err, u)
		}
		if u != nil {
			u.DisplayName = "管理员"
			if err := us.Update(ctx, u); err != nil {
				t.Errorf("Update: %v", err)
			}
			if err := us.Delete(ctx, u.ID); err != nil {
				t.Errorf("Delete: %v", err)
			}
		}
		if n, err := us.Count(ctx); err != nil || n != 0 {
			t.Errorf("Count after delete: %v (n=%d)", err, n)
		}
	})

	t.Run("tasks", func(t *testing.T) {
		ts := store.NewTaskStore(duck)
		if err := ts.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		task, err := ts.Create(ctx, "n1", "uptime")
		if err != nil {
			t.Errorf("Create: %v", err)
			return
		}
		if _, err := ts.Get(ctx, task.ID); err != nil {
			t.Errorf("Get: %v", err)
		}
		if _, _, err := ts.List(ctx, 10, 0); err != nil {
			t.Errorf("List: %v", err)
		}
		if _, err := ts.FailOrphaned(ctx); err != nil {
			t.Errorf("FailOrphaned: %v", err)
		}
	})

	t.Run("user_commands_快捷命令", func(t *testing.T) {
		cs := store.NewCommandStore(duck)
		if err := cs.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		if err := cs.Create(ctx, &model.UserCommand{UserID: 1, Name: "重启 nginx", Command: "systemctl restart nginx"}); err != nil {
			t.Errorf("Create: %v", err)
		}
		cmds, err := cs.ListByUser(ctx, 1)
		if err != nil || len(cmds) != 1 {
			t.Errorf("ListByUser: %v (n=%d)", err, len(cmds))
		}
		if len(cmds) == 1 {
			cmds[0].Name = "改名"
			if _, err := cs.Update(ctx, cmds[0]); err != nil {
				t.Errorf("Update: %v", err)
			}
			if _, err := cs.Delete(ctx, cmds[0].ID, 1); err != nil {
				t.Errorf("Delete: %v", err)
			}
		}
	})

	t.Run("playbooks", func(t *testing.T) {
		ps := store.NewPlaybookStore(duck)
		if err := ps.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		if err := ps.Upsert(ctx, &model.Playbook{ID: "pb1", Name: "巡检", FilePath: "/p/b.yml", TasksCount: 2}); err != nil {
			t.Errorf("Upsert: %v", err)
		}
		if _, err := ps.Get(ctx, "pb1"); err != nil {
			t.Errorf("Get: %v", err)
		}
		if _, err := ps.List(ctx); err != nil {
			t.Errorf("List: %v", err)
		}
	})

	t.Run("playbook_runs", func(t *testing.T) {
		rs := store.NewPlaybookRunStore(duck)
		if err := rs.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		run, err := rs.Create(ctx, "pb1", "巡检运行", "/p/b.yml", []string{"n1", "n2"},
			map[string]string{"k": "v"}, "tag1", false)
		if err != nil {
			t.Errorf("Create: %v", err)
			return
		}
		if err := rs.UpdateStatus(ctx, run.ID, model.PlaybookRunStatus("success"), ""); err != nil {
			t.Errorf("UpdateStatus: %v", err)
		}
		if err := rs.AppendResult(ctx, run.ID, &model.StepResult{
			TaskName: "t1", NodeID: "n1", Status: "success", ExitCode: 0, DurationMs: 12}); err != nil {
			t.Errorf("AppendResult: %v", err)
		}
		if _, _, err := rs.List(ctx, 10, 0); err != nil {
			t.Errorf("List: %v", err)
		}
	})

	t.Run("transfer_records", func(t *testing.T) {
		trs := store.NewTransferRecordStore(duck)
		if err := trs.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		rec, err := trs.Create(ctx, "/src/a.tar.gz", "/dst/a.tar.gz", "upload", "{}")
		if err != nil {
			t.Errorf("Create: %v", err)
			return
		}
		if _, err := trs.Get(ctx, rec.ID); err != nil {
			t.Errorf("Get: %v", err)
		}
	})

	t.Run("ai_audit_log", func(t *testing.T) {
		as := store.NewAIAuditStore(duck)
		if err := as.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		if err := as.Create(ctx, &store.AIAuditRecord{
			ID: "a1", UserID: "u1", Intent: "查看负载", Tool: "exec",
			ParamsSnapshot: "{}", Result: "ok", TargetType: "node", TargetIDs: "n1",
			LLMModel: "test-model", LLMDurationMs: 42,
		}); err != nil {
			t.Errorf("Create: %v", err)
		}
		if _, _, err := as.List(ctx, "", 0, 10); err != nil {
			t.Errorf("List: %v", err)
		}
	})

	t.Run("ai_pending_approvals", func(t *testing.T) {
		as := store.NewAIApprovalStore(duck)
		if err := as.Init(ctx); err != nil {
			t.Errorf("Init: %v", err)
			return
		}
		rec, _, err := as.CreateIfAbsent(ctx, &store.AIApproval{
			ID: "ap1", SessionKey: "sess1", UserID: "u1", Username: "admin",
			ToolName: "exec", ArgumentsJSON: "{}", Summary: "重启服务",
			Status: "pending", RequestedAt: time.Now().Unix(),
		})
		if err != nil {
			t.Errorf("CreateIfAbsent: %v", err)
		} else if rec == nil {
			t.Error("CreateIfAbsent returned nil record")
		}
		if rec != nil {
			if err := as.Decide(ctx, rec.ID, "approved", "admin", "ok"); err != nil {
				t.Errorf("Decide: %v", err)
			}
		}
	})

	t.Run("history_operations_executions_transfers", func(t *testing.T) {
		hs := store.NewHistoryStore(duck)
		if err := hs.Init(ctx); err != nil {
			t.Errorf("Init（AUTOINCREMENT 疑似方言缺口）: %v", err)
		}
		if err := hs.RecordOperation(ctx, &store.Operation{
			TaskID: "t1", OpType: "command", Command: "uptime",
			Targets: []string{"n1"}, Status: "success", Username: "admin",
		}); err != nil {
			t.Errorf("RecordOperation: %v", err)
		}
		if err := hs.RecordCommandExecution(ctx, &store.CommandExecution{
			TaskID: "t1", NodeID: "n1", Command: "uptime", ExitCode: 0, Success: true,
		}); err != nil {
			t.Errorf("RecordCommandExecution: %v", err)
		}
		if _, _, err := hs.Query(ctx, &store.QueryOptions{}); err != nil {
			t.Errorf("Query: %v", err)
		}
		if _, err := hs.Cleanup(ctx, 30); err != nil {
			t.Errorf("Cleanup: %v", err)
		}
	})

	t.Run("ai_sessions", func(t *testing.T) {
		ss, err := ai.NewSQLiteSessionStore(duck)
		if err != nil {
			t.Errorf("NewSQLiteSessionStore: %v", err)
			return
		}
		if err := ss.EnsureSchema(); err != nil {
			t.Errorf("EnsureSchema: %v", err)
		}
		now := time.Now()
		if err := ss.Save(&ai.SessionRecord{
			SessionID: "s1", Host: "web", Title: "排查磁盘",
			State: &ai.SessionState{}, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Errorf("Save: %v", err)
		}
		if _, err := ss.Load("s1", "web"); err != nil {
			t.Errorf("Load: %v", err)
		}
		if err := ss.Delete("s1", "web"); err != nil {
			t.Errorf("Delete: %v", err)
		}
	})
}

func TestDuckDBCompat_MonitorDomain(t *testing.T) {
	duck := openDuck(t)
	s := monitor.NewStoreFromDB(duck)

	t.Run("alert_types_注册表", func(t *testing.T) {
		if err := s.SeedAlertTypesIfEmpty(); err != nil {
			t.Errorf("SeedAlertTypesIfEmpty: %v", err)
		}
		types, err := s.ListAlertTypes()
		if err != nil {
			t.Errorf("ListAlertTypes: %v", err)
		}
		if err == nil && len(types) == 0 {
			t.Error("seeded alert_types is empty")
		}
	})

	t.Run("alerts_告警实例", func(t *testing.T) {
		a := &monitor.Alert{
			ID: "AL-1", AlertTypeID: "OWL-CPU-001", NodeID: "n1",
			Severity: monitor.SeverityCritical, Status: monitor.StatusOpen,
			Message: "cpu 高", MetricSnapshot: "{}",
			FirstSeen: time.Now().Unix(), LastSeen: time.Now().Unix(),
		}
		if err := s.InsertAlert(a); err != nil {
			t.Errorf("InsertAlert: %v", err)
		}
		got, exists, err := s.GetActiveAlert("OWL-CPU-001", "n1")
		if err != nil || !exists || got == nil {
			t.Errorf("GetActiveAlert: %v (exists=%v)", err, exists)
		}
	})

	t.Run("remedies_对策库", func(t *testing.T) {
		if err := s.EnsureRemedyTables(); err != nil {
			t.Errorf("EnsureRemedyTables: %v", err)
		}
		if err := s.SeedBuiltinRemediesIfEmpty(); err != nil {
			t.Errorf("SeedBuiltinRemediesIfEmpty: %v", err)
		}
		if err := s.UpsertRemedy(monitor.Remedy{
			ID: "r1", AlertTypeID: "OWL-CPU-001", Name: "查进程", Kind: "script",
			Content: "top -bn1", Risk: "low", Source: "user", CreatedAt: time.Now().Unix(),
		}); err != nil {
			t.Errorf("UpsertRemedy: %v", err)
		}
		if _, ok, err := s.GetRemedy("r1"); err != nil || !ok {
			t.Errorf("GetRemedy: %v (ok=%v)", err, ok)
		}
		if err := s.RecordExecution("r1", true); err != nil {
			t.Errorf("RecordExecution: %v", err)
		}
	})

	t.Run("notify_channels_通知渠道", func(t *testing.T) {
		if err := s.EnsureNotifyTables(); err != nil {
			t.Errorf("EnsureNotifyTables: %v", err)
			return
		}
		if err := s.UpsertNotifyChannel(monitor.NotifyChannel{
			ID: "ch1", Kind: "webhook", Name: "企微", SeverityMin: monitor.Severity("warn"),
			Enabled: true, CreatedAt: time.Now().Unix(),
		}); err != nil {
			t.Errorf("UpsertNotifyChannel: %v", err)
		}
		chs, err := s.ListNotifyChannels()
		if err != nil || len(chs) != 1 {
			t.Errorf("ListNotifyChannels: %v (n=%d)", err, len(chs))
		}
	})

	t.Run("remedy_runs_处置计划", func(t *testing.T) {
		if err := s.EnsureRemedyRunTables(); err != nil {
			t.Errorf("EnsureRemedyRunTables: %v", err)
			return
		}
		run := &monitor.RemedyRun{
			ID: "RUN-1", AlertID: "AL-1", NodeID: "n1",
			Status: monitor.RunPending, CreatedBy: "admin",
			CreatedAt: time.Now().Unix(), UpdatedAt: time.Now().Unix(),
			Steps: []monitor.RemedyStep{{
				Order: 0, RemedyID: "r1", Name: "查进程", Kind: "script",
				Content: "top -bn1", Status: monitor.StepPending, NodeID: "n1",
			}},
		}
		if err := s.CreateRemedyRun(run); err != nil {
			t.Errorf("CreateRemedyRun: %v", err)
		}
		if _, exists, err := s.GetRemedyRun("RUN-1"); err != nil || !exists {
			t.Errorf("GetRemedyRun: %v (exists=%v)", err, exists)
		}
		if err := s.UpdateRemedyRunStatus("RUN-1", monitor.RunDone); err != nil {
			t.Errorf("UpdateRemedyRunStatus: %v", err)
		}
	})

	t.Run("alert_bindings_告警绑定", func(t *testing.T) {
		if err := s.EnsureAlertBindingTables(); err != nil {
			t.Errorf("EnsureAlertBindingTables: %v", err)
			return
		}
		if err := s.CreateAlertBinding(monitor.AlertBinding{
			ID: "b1", AlertID: "OWL-CPU-001", Kind: "script", Name: "脚本处置",
			Content: "top -bn1", Risk: "low", ExecMode: "sequential",
			Seq: 0, CreatedBy: "admin", CreatedAt: time.Now().Unix(),
		}); err != nil {
			t.Errorf("CreateAlertBinding: %v", err)
		}
		if _, err := s.ListAlertBindings("OWL-CPU-001"); err != nil {
			t.Errorf("ListAlertBindings: %v", err)
		}
		if err := s.CreateAlertBindingRun(&monitor.AlertBindingRun{
			ID: "abr1", AlertID: "AL-1", BindingID: "b1", Kind: "script",
			Status: "running", CreatedBy: "admin", CreatedAt: time.Now().Unix(),
		}); err != nil {
			t.Errorf("CreateAlertBindingRun: %v", err)
		}
		if err := s.FinishAlertBindingRun("abr1", "success", ""); err != nil {
			t.Errorf("FinishAlertBindingRun: %v", err)
		}
		if err := s.FailStaleAlertBindingRuns(); err != nil {
			t.Errorf("FailStaleAlertBindingRuns: %v", err)
		}
	})

	t.Run("metrics_指标分区表", func(t *testing.T) {
		now := time.Now().Unix()
		samples := make([]monitor.Sample, 240)
		for i := range samples {
			samples[i] = monitor.Sample{
				NodeID: "n1", Metric: "cpu.usage",
				TS: now - int64(i)*60, Value: float64(i % 100),
			}
		}
		if err := s.InsertSamples(samples); err != nil {
			t.Errorf("InsertSamples（WITHOUT ROWID 疑似方言缺口）: %v", err)
			return
		}
		got, err := s.QuerySamples("n1", "cpu.usage", now-3600, now)
		if err != nil || len(got) != 60 {
			t.Errorf("QuerySamples: %v (n=%d, want 60)", err, len(got))
		}
		if _, err := s.Cleanup(30); err != nil {
			t.Errorf("Cleanup: %v", err)
		}
		if err := s.Vacuum(); err != nil {
			t.Errorf("Vacuum: %v", err)
		}
	})
}
