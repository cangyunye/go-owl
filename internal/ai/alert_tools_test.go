package ai

import (
	"context"
	"strings"
	"testing"
)

// ---------- 告警码归一化 ----------

func TestNormalizeAlertCode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"OWL-DSK-001", "OWL-DSK-001"},
		{"OWL_DSK_001", "OWL-DSK-001"},
		{"owl dsk 001", "OWL-DSK-001"},
		{" owl_mem_002 ", "OWL-MEM-002"},
		{"owl-oss-001", "OWL-OSS-001"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeAlertCode(c.in); got != c.want {
			t.Errorf("NormalizeAlertCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

var alertTypeFixture = []AlertTypeRow{
	{ID: "OWL-DSK-001", Category: "disk", Name: "磁盘使用率过高", DefaultSeverity: "warn", Rule: "disk.usage > 90 持续 2 次", Enabled: true},
	{ID: "OWL-DSK-002", Category: "disk", Name: "inode 使用率过高", DefaultSeverity: "warn", Enabled: true},
	{ID: "OWL-MEM-001", Category: "mem", Name: "内存使用率过高", DefaultSeverity: "warn", Enabled: true},
	{ID: "OWL-OSS-001", Category: "avail", Name: "节点失联", DefaultSeverity: "critical", Enabled: true},
}

// ---------- 告警码/类别匹配 ----------

func TestMatchAlertTypeCodes(t *testing.T) {
	t.Run("无过滤条件返回nil", func(t *testing.T) {
		codes, err := MatchAlertTypeCodes(alertTypeFixture, "", "")
		if err != nil || codes != nil {
			t.Errorf("expected nil codes and nil err, got %v, %v", codes, err)
		}
	})

	t.Run("精确匹配", func(t *testing.T) {
		codes, err := MatchAlertTypeCodes(alertTypeFixture, "OWL-DSK-001", "")
		if err != nil || len(codes) != 1 || codes[0] != "OWL-DSK-001" {
			t.Errorf("got %v, %v", codes, err)
		}
	})

	t.Run("下划线写法归一化", func(t *testing.T) {
		codes, err := MatchAlertTypeCodes(alertTypeFixture, "OWL_OSS_001", "")
		if err != nil || len(codes) != 1 || codes[0] != "OWL-OSS-001" {
			t.Errorf("got %v, %v", codes, err)
		}
	})

	t.Run("缺前缀后缀匹配", func(t *testing.T) {
		codes, err := MatchAlertTypeCodes(alertTypeFixture, "MEM-001", "")
		if err != nil || len(codes) != 1 || codes[0] != "OWL-MEM-001" {
			t.Errorf("got %v, %v", codes, err)
		}
	})

	t.Run("按类别匹配多个", func(t *testing.T) {
		codes, err := MatchAlertTypeCodes(alertTypeFixture, "", "disk")
		if err != nil || len(codes) != 2 {
			t.Errorf("got %v, %v", codes, err)
		}
	})

	t.Run("未知告警码报错并列出可用码", func(t *testing.T) {
		_, err := MatchAlertTypeCodes(alertTypeFixture, "OWL-XXX-999", "")
		if err == nil {
			t.Fatal("expected error for unknown code")
		}
		if !strings.Contains(err.Error(), "OWL-DSK-001") {
			t.Errorf("error should list available codes, got %q", err.Error())
		}
	})

	t.Run("未知类别报错", func(t *testing.T) {
		_, err := MatchAlertTypeCodes(alertTypeFixture, "", "nosuch")
		if err == nil {
			t.Fatal("expected error for unknown category")
		}
	})
}

// ---------- 渲染 ----------

func TestRenderAlertList(t *testing.T) {
	rows := []AlertRow{
		{ID: "AL-1", TypeID: "OWL-DSK-001", NodeName: "web-01", Severity: "warn", Status: "open", Message: "磁盘使用率过高: / 95.2%", FirstSeen: 1758247200, LastSeen: 1758252600},
		{ID: "AL-2", TypeID: "OWL-OSS-001", NodeName: "db-01", Severity: "critical", Status: "open", Message: "节点失联", FirstSeen: 1758247200, LastSeen: 1758252600},
		{ID: "AL-3", TypeID: "OWL-MEM-001", NodeName: "web-01", Severity: "warn", Status: "acked", Message: "内存使用率过高", FirstSeen: 1758247200, LastSeen: 1758252600},
	}
	out := RenderAlertList(rows, 3)
	for _, want := range []string{"OWL-DSK-001", "web-01", "db-01", "critical", "磁盘使用率过高: / 95.2%", "3 条"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderAlertList output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "按节点汇总") || !strings.Contains(out, "web-01(2)") {
		t.Errorf("RenderAlertList should aggregate by node:\n%s", out)
	}
}

func TestRenderAlertListEmpty(t *testing.T) {
	out := RenderAlertList(nil, 0)
	if !strings.Contains(out, "没有") {
		t.Errorf("empty list should say no alerts, got %q", out)
	}
}

func TestRenderAlertTypes(t *testing.T) {
	out := RenderAlertTypes(alertTypeFixture)
	for _, want := range []string{"OWL-DSK-001", "磁盘使用率过高", "disk", "warn", "OWL-OSS-001"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderAlertTypes output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderRemedies(t *testing.T) {
	typeRow := &alertTypeFixture[0]
	remedies := []RemedyRow{
		{ID: "R-DSK-001-SOP", Name: "磁盘使用率过高排查指引", Kind: "sop", Risk: "low", Source: "builtin",
			Rollback: "无需回滚", Content: "1. du -h --max-depth=1 / 定位大目录"},
	}
	out := RenderRemedies("OWL-DSK-001", typeRow, remedies)
	for _, want := range []string{"OWL-DSK-001", "磁盘使用率过高", "du -h", "low", "无需回滚", "SOP"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderRemedies output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderRemediesUnknownCode(t *testing.T) {
	out := RenderRemedies("OWL-XXX-999", nil, nil)
	if !strings.Contains(out, "OWL-XXX-999") {
		t.Errorf("unknown code should be echoed, got %q", out)
	}
}

// ---------- 工具 ----------

type stubAlertExecutor struct {
	Executor // 未覆盖的方法被调用时 panic（测试只覆盖告警方法）

	listParams []AlertListParams
	listRows   []AlertRow
	listTotal  int

	remedyCode     string
	remedyTypeRows []AlertTypeRow
	remedyRows     []RemedyRow
}

func (s *stubAlertExecutor) ListAlerts(ctx context.Context, p AlertListParams) (*AlertListResult, error) {
	s.listParams = append(s.listParams, p)
	return &AlertListResult{Text: RenderAlertList(s.listRows, s.listTotal)}, nil
}

func (s *stubAlertExecutor) ListAlertTypes(ctx context.Context) (*AlertTypesResult, error) {
	return &AlertTypesResult{Text: RenderAlertTypes(s.remedyTypeRows)}, nil
}

func (s *stubAlertExecutor) GetAlertRemedies(ctx context.Context, p AlertRemedyParams) (*AlertRemedyResult, error) {
	s.remedyCode = p.AlertTypeID
	var typeRow *AlertTypeRow
	for i := range s.remedyTypeRows {
		if s.remedyTypeRows[i].ID == p.AlertTypeID {
			typeRow = &s.remedyTypeRows[i]
		}
	}
	return &AlertRemedyResult{Text: RenderRemedies(p.AlertTypeID, typeRow, s.remedyRows)}, nil
}

func TestAlertListToolExecute(t *testing.T) {
	stub := &stubAlertExecutor{listRows: []AlertRow{{ID: "AL-1", TypeID: "OWL-DSK-001", NodeName: "web-01"}}, listTotal: 1}
	tool := NewAlertListTool(stub)

	params := map[string]interface{}{
		"alert_type_id": "OWL_DSK_001",
		"node":          "web-01",
		"severity":      "warn",
		"limit":         float64(10),
	}
	if err := tool.Validate(params); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	out, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if len(stub.listParams) != 1 {
		t.Fatalf("expected 1 executor call, got %d", len(stub.listParams))
	}
	p := stub.listParams[0]
	if p.AlertTypeID != "OWL-DSK-001" {
		t.Errorf("alert_type_id should be normalized, got %q", p.AlertTypeID)
	}
	if p.Node != "web-01" || p.Severity != "warn" || p.Limit != 10 {
		t.Errorf("params not passed through: %+v", p)
	}
	if !strings.Contains(out, "OWL-DSK-001") {
		t.Errorf("output should contain rendered alerts, got %q", out)
	}
}

func TestAlertListToolValidate(t *testing.T) {
	tool := NewAlertListTool(nil)
	if err := tool.Validate(map[string]interface{}{"severity": "bogus"}); err == nil {
		t.Error("expected invalid severity to fail validation")
	}
	if err := tool.Validate(map[string]interface{}{"status": "bogus"}); err == nil {
		t.Error("expected invalid status to fail validation")
	}
	if err := tool.Validate(map[string]interface{}{}); err != nil {
		t.Errorf("empty params should be valid, got %v", err)
	}
}

func TestAlertTypesToolExecute(t *testing.T) {
	stub := &stubAlertExecutor{remedyTypeRows: alertTypeFixture}
	tool := NewAlertTypesTool(stub)
	if err := tool.Validate(map[string]interface{}{}); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	out, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !strings.Contains(out, "OWL-MEM-001") {
		t.Errorf("output should list alert codes, got %q", out)
	}
}

func TestAlertRemedyToolValidate(t *testing.T) {
	tool := NewAlertRemedyTool(nil)
	if err := tool.Validate(map[string]interface{}{}); err == nil {
		t.Error("alert_type_id is required")
	}
	if err := tool.Validate(map[string]interface{}{"alert_type_id": "OWL-DSK-001"}); err != nil {
		t.Errorf("valid params rejected: %v", err)
	}
}

func TestAlertRemedyToolExecute(t *testing.T) {
	stub := &stubAlertExecutor{
		remedyTypeRows: alertTypeFixture,
		remedyRows:     []RemedyRow{{ID: "R-1", Name: "SOP", Kind: "sop", Content: "du -h"}},
	}
	tool := NewAlertRemedyTool(stub)
	out, err := tool.Execute(context.Background(), map[string]interface{}{"alert_type_id": "owl-dsk-001"})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if stub.remedyCode != "OWL-DSK-001" {
		t.Errorf("code should be normalized before executor call, got %q", stub.remedyCode)
	}
	if !strings.Contains(out, "du -h") {
		t.Errorf("output should contain remedy content, got %q", out)
	}
}

func TestAlertToolsNilExecutor(t *testing.T) {
	tool := NewAlertListTool(nil)
	if _, err := tool.Execute(context.Background(), map[string]interface{}{}); err == nil {
		t.Error("expected error when executor is nil")
	}
}
