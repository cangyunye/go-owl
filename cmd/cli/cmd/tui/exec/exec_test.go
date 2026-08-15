package exec

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/command"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
func runeKey(r rune) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func newTestModel(t *testing.T) ExecModel {
	t.Helper()
	store := common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
	for _, n := range []*common.NodeInfo{
		{ID: "n1", Name: "web-1", Groups: []string{"web"}, Labels: map[string]string{"env": "prod"}},
		{ID: "n2", Name: "db-1", Groups: []string{"db"}, Labels: map[string]string{"env": "dev"}},
		{ID: "n3", Name: "cache-1", Groups: []string{"web", "cache"}, Labels: map[string]string{"env": "prod", "role": "cache"}},
	} {
		if err := store.Add(n); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := NewModel(store)
	nodes, _ := store.List()
	m.CaptureTargets(nodes)
	return m
}

func TestNewModel_DefaultState(t *testing.T) {
	m := newTestModel(t)
	if m.format != "simple" {
		t.Fatalf("expected format simple, got %s", m.format)
	}
	if m.current() != LocRun {
		t.Fatalf("expected stack top LocRun, got %v", m.current())
	}
	if got := m.Path(); len(got) != 2 || got[0] != "exec" || got[1] != "run" {
		t.Fatalf("unexpected path: %v", got)
	}
	if m.Mode() != ModeNormal || m.InsertMode() {
		t.Fatal("expected ModeNormal")
	}
	if m.IsDirty() {
		t.Fatal("exec panel never dirty")
	}
}

func TestFormatCycle_FToJsonToSimple(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "detail" {
		t.Fatalf("expected detail after first f, got %s", m.format)
	}
	nm, _ = m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "json" {
		t.Fatalf("expected json after second f, got %s", m.format)
	}
	nm, _ = m.Update(runeKey('f'))
	m = nm.(ExecModel)
	if m.format != "simple" {
		t.Fatalf("expected simple after third f, got %s", m.format)
	}
}

func TestResolve_ExplicitNodes(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n1,n3")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_Groups(t *testing.T) {
	m := newTestModel(t)
	m.groupsInput.SetValue("web")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_Labels(t *testing.T) {
	m := newTestModel(t)
	m.labelsInput.SetValue("env=prod")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestResolve_EmptyFallsBackToSnapshot(t *testing.T) {
	m := newTestModel(t)
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 from snapshot, got %d", len(nodes))
	}
}

func TestResolve_PriorityNodesOverGroups(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n2")
	m.groupsInput.SetValue("web")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "n2" {
		t.Fatalf("expected [n2] (nodes wins), got %v", nodeIDs(nodes))
	}
}

func TestResolve_DedupeAndSort(t *testing.T) {
	m := newTestModel(t)
	m.nodesInput.SetValue("n3,n1,n3")
	nodes, err := m.resolveTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 || nodes[0].ID != "n1" || nodes[1].ID != "n3" {
		t.Fatalf("expected deduped sorted [n1 n3], got %v", nodeIDs(nodes))
	}
}

func TestRunView_ShowsFourFieldsAndFormat(t *testing.T) {
	m := newTestModel(t)
	got := m.View()
	for _, want := range []string{"命令", "节点", "分组", "标签", "simple", "目标", "3 台"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestEscAtRootEmitsLeavePanel(t *testing.T) {
	m := newTestModel(t)
	nm, cmd := m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	if _, ok := msg.(LeavePanelMsg); !ok {
		t.Fatalf("expected LeavePanelMsg, got %T", msg)
	}
}

func TestEnterEditsFieldAndEscRestores(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(ExecModel)
	if m.mode != ModeInsert {
		t.Fatal("expected ModeInsert after enter")
	}
	if !m.cmdInput.Focused() {
		t.Fatal("expected cmd input focused")
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if m.mode != ModeNormal {
		t.Fatal("expected ModeNormal after esc")
	}
	if m.cmdInput.Focused() {
		t.Fatal("expected cmd input blurred")
	}
}

func nodeIDs(nodes []*common.NodeInfo) []string {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return ids
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestAdvanced_Defaults(t *testing.T) {
	f := newAdvancedForm()
	if len(f.fields) != 20 {
		t.Fatalf("expected 20 fields, got %d", len(f.fields))
	}
	if got := f.value("timeout"); got != "60s" {
		t.Fatalf("expected timeout 60s, got %q", got)
	}
	if !f.isOn("parallel") || f.isOn("serial") {
		t.Fatal("expected parallel on, serial off")
	}
}

func TestAdvanced_ToggleBoolWithSpace(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	// 下移到 parallel 行 (索引 3)
	for i := 0; i < 3; i++ {
		nm, _ := m.Update(key(tea.KeyDown))
		m = nm.(ExecModel)
	}
	nm, _ := m.Update(runeKey(' '))
	m = nm.(ExecModel)
	if m.advanced.isOn("parallel") {
		t.Fatal("space should toggle parallel off")
	}
	if m.advanced.isOn("serial") {
		t.Fatal("serial should stay off")
	}
}

func TestAdvanced_SpaceIgnoredOnTextField(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	// 光标在 timeout (索引 0, KindText), 空格应被忽略
	nm, _ := m.Update(runeKey(' '))
	m = nm.(ExecModel)
	if !m.advanced.isOn("parallel") {
		t.Fatal("space on text field should be ignored")
	}
}

func TestAdvanced_EditTextField(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(ExecModel)
	if m.mode != ModeInsert {
		t.Fatal("expected ModeInsert on enter")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("120s")})
	m = nm.(ExecModel)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if got := m.advanced.value("timeout"); got != "120s" {
		t.Fatalf("expected timeout 120s, got %q", got)
	}
}

func TestAdvanced_SaveReturnsToRun(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	nm, _ := m.Update(runeKey('s'))
	m = nm.(ExecModel)
	if m.current() != LocRun {
		t.Fatalf("expected LocRun after save, got %v", m.current())
	}
	if m.advanced == nil {
		t.Fatal("expected advanced kept after save")
	}
	got := m.View()
	for _, want := range []string{"高级", "并行", "timeout=60s"} {
		if !contains(got, want) {
			t.Fatalf("run view missing %q after save:\n%s", want, got)
		}
	}
}

func TestAdvanced_EscDiscards(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	nm, _ := m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if m.current() != LocRun {
		t.Fatalf("expected LocRun after esc, got %v", m.current())
	}
	if m.advanced != nil {
		t.Fatal("expected advanced cleared after esc")
	}
}

func TestAdvanced_Summary(t *testing.T) {
	f := newAdvancedForm()
	got := advancedSummary(f)
	for _, want := range []string{"并行", "timeout=60s"} {
		if !contains(got, want) {
			t.Fatalf("default summary missing %q: %q", want, got)
		}
	}
	f.toggle(4) // serial
	got = advancedSummary(f)
	if !contains(got, "串行") {
		t.Fatalf("serial summary missing 串行: %q", got)
	}
	if contains(got, "并行") {
		t.Fatalf("serial summary should not contain 并行: %q", got)
	}
	f.toggle(9) // async
	got = advancedSummary(f)
	if !contains(got, "async") {
		t.Fatalf("summary missing async: %q", got)
	}
}

func TestRunView_ShowsAdvancedHintWhenUnset(t *testing.T) {
	m := newTestModel(t)
	got := m.View()
	if !contains(got, "默认(未设置)") {
		t.Fatalf("run view missing advanced hint:\n%s", got)
	}
}

func TestAdvanced_BuildOpts_Defaults(t *testing.T) {
	f := newAdvancedForm()
	opts, err := f.buildOpts()
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Parallel {
		t.Fatal("expected parallel")
	}
	if opts.TimeoutConfig == nil || opts.TimeoutConfig.ConnectTimeout != 10*time.Second || opts.TimeoutConfig.CommandTimeout != 30*time.Second {
		t.Fatalf("unexpected timeout config: %+v", opts.TimeoutConfig)
	}
	if opts.RetryConfig == nil || opts.RetryConfig.MaxRetries != 3 {
		t.Fatalf("unexpected retry config: %+v", opts.RetryConfig)
	}
}

func TestAdvanced_BuildOpts_SerialOverrides(t *testing.T) {
	f := newAdvancedForm()
	f.toggle(4) // serial 行
	opts, err := f.buildOpts()
	if err != nil {
		t.Fatal(err)
	}
	if opts.Parallel {
		t.Fatal("expected serial mode")
	}
}

func TestAdvanced_BuildOpts_NoRetryDisables(t *testing.T) {
	f := newAdvancedForm()
	f.toggle(8) // no-retry 行
	opts, err := f.buildOpts()
	if err != nil {
		t.Fatal(err)
	}
	if opts.RetryConfig != nil {
		t.Fatalf("expected no retry config, got %+v", opts.RetryConfig)
	}
}

func TestAdvanced_BuildOpts_InvalidDuration(t *testing.T) {
	f := newAdvancedForm()
	f.fields[0].input.SetValue("abc")
	if _, err := f.buildOpts(); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestAdvanced_ViewShowsCheckboxes(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	got := m.View()
	for _, want := range []string{"高级选项", "parallel", "[x]", "[ ]"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func fakeStream(results []command.CommandResult) {
	runStream = func(ctx context.Context, ids []string, cmd string, opts *command.ExecuteOptions) (<-chan command.CommandResult, func()) {
		ch := make(chan command.CommandResult, len(results))
		for _, r := range results {
			ch <- r
		}
		close(ch)
		return ch, func() {}
	}
}

func TestStartRun_EmptyCommand(t *testing.T) {
	m := newTestModel(t)
	if _, err := m.startRun(); err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestStartRun_NoTargets(t *testing.T) {
	m := newTestModel(t)
	m.CaptureTargets(nil)
	m.cmdInput.SetValue("echo hi")
	if _, err := m.startRun(); err == nil {
		t.Fatal("expected error for no targets")
	}
}

func TestStartRun_InvalidAdvancedOption(t *testing.T) {
	m := newTestModel(t)
	m.cmdInput.SetValue("echo hi")
	m.advanced = newAdvancedForm()
	m.advanced.fields[0].input.SetValue("abc")
	if _, err := m.startRun(); err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestRun_StreamsResultsAndRenders(t *testing.T) {
	fakeStream([]command.CommandResult{
		{NodeID: "n1", Success: true, ExitCode: 0, Duration: time.Second, Output: "ok"},
		{NodeID: "n2", Success: false, ExitCode: 127, Error: errors.New("boom")},
	})
	m := newTestModel(t)
	m.cmdInput.SetValue("echo hi")
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(ExecModel)
	if m.current() != LocResult {
		t.Fatalf("expected LocResult after r, got %v", m.current())
	}
	if cmd == nil {
		t.Fatal("expected start cmd")
	}
	msg := cmd()
	sm, ok := msg.(ExecStreamMsg)
	if !ok {
		t.Fatalf("expected ExecStreamMsg, got %T", msg)
	}
	// 泵出第一条
	nm, cmd = m.Update(sm)
	m = nm.(ExecModel)
	rmsg := cmd().(ExecResultMsg)
	nm, cmd = m.Update(rmsg)
	m = nm.(ExecModel)
	if len(m.results) != 1 || m.results[0].NodeID != "n1" {
		t.Fatalf("expected 1 result n1, got %v", m.results)
	}
	// 第二条
	nm, cmd = m.Update((cmd().(ExecResultMsg)))
	m = nm.(ExecModel)
	if len(m.results) != 2 || m.results[1].NodeID != "n2" {
		t.Fatalf("expected 2 results, got %v", m.results)
	}
	// 泵结束: ch 已关闭, 下一泵消息是 ExecDoneMsg
	nm, cmd = m.Update(cmd())
	m = nm.(ExecModel)
	if cmd != nil {
		t.Fatal("expected nil cmd after ExecDoneMsg")
	}
	got := m.View()
	for _, want := range []string{"Exec 结果", "n1", "n2", "成功 1/2"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestResult_EscReturnsToRun(t *testing.T) {
	m := newTestModel(t)
	m.push(LocResult)
	nm, _ := m.Update(key(tea.KeyEsc))
	m = nm.(ExecModel)
	if m.current() != LocRun {
		t.Fatalf("expected LocRun after esc, got %v", m.current())
	}
}

func TestResult_Rerun(t *testing.T) {
	fakeStream(nil)
	m := newTestModel(t)
	m.cmdInput.SetValue("echo hi")
	m.push(LocResult)
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(ExecModel)
	if cmd == nil {
		t.Fatal("expected rerun cmd")
	}
	if _, ok := cmd().(ExecStreamMsg); !ok {
		t.Fatalf("expected ExecStreamMsg, got %T", cmd())
	}
}
