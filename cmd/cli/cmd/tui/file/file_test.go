package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/transfer"
	"github.com/cangyunye/go-owl/internal/history"
)

func key(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
func runeKey(r rune) tea.Msg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func newTestModel(t *testing.T) FileModel {
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
	m := NewModel(newTestStore(t))
	if m.op != OpUpload {
		t.Fatalf("expected OpUpload, got %v", m.op)
	}
	if m.current() != LocFile {
		t.Fatalf("expected stack top LocFile, got %v", m.current())
	}
	if got := m.Path(); len(got) != 2 || got[0] != "file" || got[1] != "upload" {
		t.Fatalf("unexpected path: %v", got)
	}
	if m.Mode() != ModeNormal || m.InsertMode() {
		t.Fatal("expected ModeNormal")
	}
	if m.IsDirty() {
		t.Fatal("file panel never dirty")
	}
}

func newTestStore(t *testing.T) common.NodeStore {
	t.Helper()
	return common.NewInMemoryNodeStoreAt(filepath.Join(t.TempDir(), "nodes.json"))
}

func TestOpToggle_LeftRightCycles(t *testing.T) {
	m := newTestModel(t)
	// 标准按键: right 一次 → download, 再 right → transfer, 再 right → upload
	nm, _ := m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpDownload {
		t.Fatalf("expected OpDownload after right, got %v", m.op)
	}
	if got := m.Path(); got[1] != "download" {
		t.Fatalf("expected path file/download, got %v", got)
	}
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpTransfer {
		t.Fatalf("expected OpTransfer after second right, got %v", m.op)
	}
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpUpload {
		t.Fatalf("expected wrap to OpUpload, got %v", m.op)
	}
	nm, _ = m.Update(key(tea.KeyLeft))
	m = nm.(FileModel)
	if m.op != OpTransfer {
		t.Fatalf("expected OpTransfer on left, got %v", m.op)
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

func TestRunView_ShowsFiveFieldsAndTargets(t *testing.T) {
	m := newTestModel(t)
	got := m.View()
	for _, want := range []string{"文件上传", "本地文件", "节点", "分组", "标签", "目标目录", "目标 3 台", "/tmp"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestEscAtRootEmitsLeavePanel(t *testing.T) {
	m := newTestModel(t)
	nm, cmd := m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
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
	m.op = OpDownload // download 的本地文件字段不弹浏览器, 走普通编辑
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.mode != ModeInsert {
		t.Fatal("expected ModeInsert after enter")
	}
	if !m.fileInput.Focused() {
		t.Fatal("expected file input focused")
	}
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
	if m.mode != ModeNormal {
		t.Fatal("expected ModeNormal after esc")
	}
	if m.fileInput.Focused() {
		t.Fatal("expected file input blurred")
	}
}

func TestInsertMode_LeftRightTogglesOpDisabled(t *testing.T) {
	m := newTestModel(t)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if m.op != OpUpload {
		t.Fatal("op must not toggle in Insert mode")
	}
}

func TestAdvanced_Defaults(t *testing.T) {
	f := newAdvancedForm()
	if !f.isOn("parallel") {
		t.Fatal("expected parallel on")
	}
	if !f.isOn("resume") {
		t.Fatal("expected resume on")
	}
	if f.isOn("no-overwrite") {
		t.Fatal("expected no-overwrite off")
	}
	opts := f.uploadOpts()
	if opts == nil || !opts.Parallel || !opts.Resume || opts.NoOverwrite {
		t.Fatalf("unexpected default opts: %+v", opts)
	}
}

func TestAdvanced_ToggleBoolWithSpace(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	// cursor 0 = parallel 行, 空格关闭
	nm, _ := m.Update(runeKey(' '))
	m = nm.(FileModel)
	if m.advanced.isOn("parallel") {
		t.Fatal("expected parallel off after space")
	}
	// 移到 resume 行再切换
	nm, _ = m.Update(key(tea.KeyDown))
	m = nm.(FileModel)
	nm, _ = m.Update(runeKey(' '))
	m = nm.(FileModel)
	if m.advanced.isOn("resume") {
		t.Fatal("expected resume off after space on row 1")
	}
}

func TestAdvanced_SaveReturnsToFile(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	nm, _ := m.Update(runeKey('s'))
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected LocFile after save, got %v", m.current())
	}
	if m.advanced != nil {
		t.Fatal("expected advanced cleared")
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

func fakeUpload(results []transfer.TransferResult) {
	uploadRun = func(ctx context.Context, ids []string, localFile, remotePath string, opts *transfer.UploadOptions) []transfer.TransferResult {
		return results
	}
	recordOperation = func(o *history.Operation) error { return nil }
	recordFileTransfer = func(f *history.FileTransfer) error { return nil }
}

func TestStartUpload_EmptyFile(t *testing.T) {
	m := newTestModel(t)
	if _, err := m.startUpload(); err == nil {
		t.Fatal("expected error for empty file path")
	}
}

func TestStartUpload_FileNotExist(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.SetValue(filepath.Join(t.TempDir(), "no-such.tar.gz"))
	if _, err := m.startUpload(); err == nil {
		t.Fatal("expected error for missing local file")
	}
}

func TestStartUpload_NoTargets(t *testing.T) {
	m := newTestModel(t)
	m.CaptureTargets(nil)
	m.fileInput.SetValue(filepath.Join(t.TempDir(), "a.tar"))
	if _, err := m.startUpload(); err == nil {
		t.Fatal("expected error for no targets")
	}
}

func TestUpload_RunsAndRenders(t *testing.T) {
	fakeUpload([]transfer.TransferResult{
		{NodeID: "n1", Path: "/tmp/a.tar", Method: "scp"},
		{NodeID: "n2", Path: "/tmp/a.tar", Method: "scp", Error: errors.New("boom")},
	})
	m := newTestModel(t)
	local := filepath.Join(t.TempDir(), "a.tar")
	if err := os.WriteFile(local, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	m.fileInput.SetValue(local)
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(FileModel)
	if m.current() != LocResult {
		t.Fatalf("expected LocResult after r, got %v", m.current())
	}
	if cmd == nil {
		t.Fatal("expected start cmd")
	}
	msg := cmd()
	dm, ok := msg.(UploadDoneMsg)
	if !ok {
		t.Fatalf("expected UploadDoneMsg, got %T", msg)
	}
	nm, _ = m.Update(dm)
	m = nm.(FileModel)
	got := m.View()
	for _, want := range []string{"上传结果", "n1", "n2", "成功 1/2", "/tmp/a.tar"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestResult_EscReturnsToFile(t *testing.T) {
	m := newTestModel(t)
	m.push(LocResult)
	nm, _ := m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected LocFile after esc, got %v", m.current())
	}
}

func TestResult_Rerun(t *testing.T) {
	fakeUpload(nil)
	m := newTestModel(t)
	local := filepath.Join(t.TempDir(), "a.tar")
	if err := os.WriteFile(local, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	m.fileInput.SetValue(local)
	m.push(LocResult)
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(FileModel)
	if cmd == nil {
		t.Fatal("expected rerun cmd")
	}
	if _, ok := cmd().(UploadDoneMsg); !ok {
		t.Fatalf("expected UploadDoneMsg, got %T", cmd())
	}
}

func TestResult_RerunDownload(t *testing.T) {
	downloadRun = func(ctx context.Context, ids []string, remoteFile, localDir string, opts *transfer.DownloadOptions) []transfer.TransferResult {
		return nil
	}
	recordOperation = func(o *history.Operation) error { return nil }
	recordFileTransfer = func(f *history.FileTransfer) error { return nil }
	m := newTestModel(t)
	m.op = OpDownload
	m.destInput.SetValue(filepath.Join(t.TempDir(), "out"))
	m.fileInput.SetValue("/var/log/app.log")
	m.push(LocResult)
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(FileModel)
	if cmd == nil {
		t.Fatal("expected rerun cmd")
	}
	if _, ok := cmd().(DownloadDoneMsg); !ok {
		t.Fatalf("expected DownloadDoneMsg, got %T", cmd())
	}
	if m.current() != LocResult {
		t.Fatalf("expected to stay on LocResult, got %v", m.current())
	}
}

func TestStartDownload_EmptyRemote(t *testing.T) {
	m := newTestModel(t)
	m.op = OpDownload
	if _, err := m.startDownload(); err == nil {
		t.Fatal("expected error for empty remote path")
	}
}

func TestOpSwitch_SwapsDestPlaceholderByOp(t *testing.T) {
	m := newTestModel(t)
	if !strings.Contains(m.destInput.Placeholder, "/tmp") {
		t.Fatalf("upload dest placeholder should mention /tmp, got %q", m.destInput.Placeholder)
	}
	nm, _ := m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if !strings.Contains(m.destInput.Placeholder, "本地目录") {
		t.Fatalf("download dest placeholder should mention 本地目录, got %q", m.destInput.Placeholder)
	}
	if v := m.destInput.Value(); v != "" {
		t.Fatalf("expected empty dest value, got %q", v)
	}
	if !strings.Contains(m.fileInput.Placeholder, "远程文件") {
		t.Fatalf("download file placeholder should mention 远程文件, got %q", m.fileInput.Placeholder)
	}
	nm, _ = m.Update(key(tea.KeyRight))
	m = nm.(FileModel)
	if !strings.Contains(m.destInput.Placeholder, "目标目录") {
		t.Fatalf("transfer dest placeholder should mention 目标目录, got %q", m.destInput.Placeholder)
	}
	if !strings.Contains(m.fileInput.Placeholder, "本地文件") {
		t.Fatalf("transfer file placeholder should mention 本地文件, got %q", m.fileInput.Placeholder)
	}
}

func TestStartDownload_EmptyDestDefaultsToDot(t *testing.T) {
	var gotDir string
	downloadRun = func(ctx context.Context, ids []string, remoteFile, localDir string, opts *transfer.DownloadOptions) []transfer.TransferResult {
		gotDir = localDir
		return []transfer.TransferResult{{NodeID: "n1", Path: "app.log", Method: "scp"}}
	}
	recordOperation = func(o *history.Operation) error { return nil }
	recordFileTransfer = func(f *history.FileTransfer) error { return nil }
	m := newTestModel(t)
	m.op = OpDownload
	m.fileInput.SetValue("/var/log/app.log")
	cmd, err := m.startDownload()
	if err != nil {
		t.Fatal(err)
	}
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	cmd()
	if gotDir != "." {
		t.Fatalf("expected localDir default '.', got %q", gotDir)
	}
}

func TestStartDownload_NoTargets(t *testing.T) {
	m := newTestModel(t)
	m.op = OpDownload
	m.CaptureTargets(nil)
	m.fileInput.SetValue("/var/log/app.log")
	if _, err := m.startDownload(); err == nil {
		t.Fatal("expected error for no targets")
	}
}

func TestDownload_RunsAndRenders(t *testing.T) {
	downloadRun = func(ctx context.Context, ids []string, remoteFile, localDir string, opts *transfer.DownloadOptions) []transfer.TransferResult {
		return []transfer.TransferResult{
			{NodeID: "n1", Path: "out/n1.app.log", Method: "scp"},
		}
	}
	recordOperation = func(o *history.Operation) error { return nil }
	recordFileTransfer = func(f *history.FileTransfer) error { return nil }
	m := newTestModel(t)
	m.op = OpDownload
	m.destInput.SetValue(filepath.Join(t.TempDir(), "out"))
	m.fileInput.SetValue("/var/log/app.log")
	nm, cmd := m.Update(runeKey('r'))
	m = nm.(FileModel)
	if m.current() != LocResult {
		t.Fatalf("expected LocResult, got %v", m.current())
	}
	msg := cmd()
	dm, ok := msg.(DownloadDoneMsg)
	if !ok {
		t.Fatalf("expected DownloadDoneMsg, got %T", msg)
	}
	nm, _ = m.Update(dm)
	m = nm.(FileModel)
	got := m.View()
	for _, want := range []string{"下载结果", "n1", "成功 1/1"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestDownload_ViewLabelsPerOp(t *testing.T) {
	m := newTestModel(t)
	m.op = OpDownload
	got := m.View()
	for _, want := range []string{"远程文件", "本地目录"} {
		if !contains(got, want) {
			t.Fatalf("download view missing %q:\n%s", want, got)
		}
	}
}

func TestAdvanced_TextEditNameFormatPersists(t *testing.T) {
	m := newTestModel(t)
	m.advanced = newAdvancedForm()
	m.push(LocAdvanced)
	for i := 0; i < 4; i++ {
		nm, _ := m.Update(key(tea.KeyDown))
		m = nm.(FileModel)
	}
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.mode != ModeInsert {
		t.Fatalf("expected ModeInsert on name-format row, got %v", m.mode)
	}
	nm, _ = m.Update(runeKey('n'))
	m = nm.(FileModel)
	nm, _ = m.Update(runeKey('m'))
	m = nm.(FileModel)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
	if m.mode != ModeNormal {
		t.Fatalf("expected ModeNormal after esc, got %v", m.mode)
	}
	if v := m.advanced.value("name-format"); v != "nm" {
		t.Fatalf("expected name-format 'nm', got %q", v)
	}
	if m.advanced.fields[m.advanced.cursor].input.Focused() {
		t.Fatal("expected input blurred after esc")
	}
}

func TestEnterOnLocalFileFieldOpensBrowser(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	// 本地文件字段是 cursor 0, 直接 Enter
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocBrowser {
		t.Fatalf("expected LocBrowser, got %v", m.current())
	}
	if m.browser == nil || m.browser.dir == "" {
		t.Fatal("expected browser opened")
	}
	got := m.View()
	for _, want := range []string{"文件浏览器", "本地文件"} {
		if !contains(got, want) {
			t.Fatalf("view missing %q:\n%s", want, got)
		}
	}
}

func TestBrowserSelectFileFillsInputAndEdits(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	// 切到独立临时目录, 避免把测试文件写进源码树
	t.Chdir(t.TempDir())
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	// 写入一个真实文件供选中
	dir := m.browser.dir
	f := filepath.Join(dir, "pick-me.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// 重新加载使文件可见
	if err := m.browser.Jump(dir); err != nil {
		t.Fatal(err)
	}
	// 光标移到该文件(目录在前, 文件按字母序, pick-me.txt 应在最后)
	for i := 0; i < len(m.browser.entries); i++ {
		if m.browser.entries[i].Name == "pick-me.txt" {
			m.browser.cursor = i
			break
		}
	}
	nm, _ = m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected back to LocFile, got %v", m.current())
	}
	if m.fileInput.Value() != f {
		t.Fatalf("expected input %q, got %q", f, m.fileInput.Value())
	}
	if m.mode != ModeInsert || !m.fileInput.Focused() {
		t.Fatal("expected field editing mode after pick")
	}
}

func TestBrowserEscReturnsToFieldEdit(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	nm, _ = m.Update(key(tea.KeyEsc))
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected LocFile, got %v", m.current())
	}
	if m.mode != ModeInsert || !m.fileInput.Focused() {
		t.Fatal("expected field editing mode after esc")
	}
}

func TestBrowserEscClosesPathInputFirst(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	nm, _ = m.Update(runeKey('/')) // 打开路径输入
	m = nm.(FileModel)
	nm, _ = m.Update(key(tea.KeyEsc)) // 第一次 Esc: 关输入
	m = nm.(FileModel)
	if m.current() != LocBrowser {
		t.Fatal("expected still in browser after first esc")
	}
	nm, _ = m.Update(key(tea.KeyEsc)) // 第二次 Esc: 退出浏览器
	m = nm.(FileModel)
	if m.current() != LocFile {
		t.Fatalf("expected LocFile after second esc, got %v", m.current())
	}
}

func TestBrowserNavigateDirAndBack(t *testing.T) {
	m := newTestModel(t)
	m.fileInput.Reset()
	// 浏览器起点是 os.Getwd(); 切到含子目录的临时目录保证首个条目是目录
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(base)
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	root := m.browser.dir
	// 首个条目应是目录
	if len(m.browser.entries) == 0 || !m.browser.entries[0].IsDir {
		t.Fatal("expected a dir at cursor 0")
	}
	nm, _ = m.Update(key(tea.KeyEnter)) // 进入
	m = nm.(FileModel)
	if m.browser.dir == root {
		t.Fatal("expected dir changed")
	}
	nm, _ = m.Update(key(tea.KeyBackspace)) // 返回上级
	m = nm.(FileModel)
	if m.browser.dir != root {
		t.Fatalf("expected back to %s, got %s", root, m.browser.dir)
	}
}

func TestEnterOnDownloadNoBrowser(t *testing.T) {
	m := newTestModel(t)
	m.op = OpDownload
	m.fileInput.Reset()
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocFile || m.mode != ModeInsert {
		t.Fatalf("download must stay on field edit, got loc=%v mode=%v", m.current(), m.mode)
	}
}

func TestEnterOnOtherFieldNoBrowser(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 1 // 节点字段
	nm, _ := m.Update(key(tea.KeyEnter))
	m = nm.(FileModel)
	if m.current() != LocFile || m.mode != ModeInsert {
		t.Fatalf("other fields must stay on field edit, got loc=%v mode=%v", m.current(), m.mode)
	}
}
