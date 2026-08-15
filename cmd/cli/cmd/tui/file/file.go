package file

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/transfer"
)

type Loc int

const (
	LocFile Loc = iota
	LocAdvanced
	LocResult
	LocBrowser
)

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
)

type Op int

const (
	OpUpload Op = iota
	OpDownload
	OpTransfer
)

type LeavePanelMsg struct{}

var opNames = []string{"upload", "download", "transfer"}
var opLabels = []string{"文件上传", "文件下载", "扩散传输"}

type FileModel struct {
	store common.NodeStore

	stack  []Loc
	mode   Mode
	cursor int
	op     Op

	fileInput   textinput.Model
	destInput   textinput.Model
	nodesInput  textinput.Model
	groupsInput textinput.Model
	labelsInput textinput.Model

	targets  []*common.NodeInfo
	advanced *AdvancedForm
	browser  *FileBrowser
	error    string

	lastUpload   *uploadRunState
	lastDownload *downloadRunState
	loading      bool
	results      []transfer.TransferResult
}

func NewModel(store common.NodeStore) FileModel {
	dest := textinput.New()
	dest.Placeholder = "目标目录 (默认 /tmp)"
	dest.Width = 40
	dest.CharLimit = 256
	dest.Blur()
	return FileModel{
		store:       store,
		stack:       []Loc{LocFile},
		fileInput:   newInput("本地文件路径 (必填)", 50),
		destInput:   dest,
		nodesInput:  newInput("节点 ID,逗号分隔 (留空=当前过滤可见)", 40),
		groupsInput: newInput("分组,逗号分隔", 40),
		labelsInput: newInput("标签 k=v,逗号分隔", 40),
	}
}

func newInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Width = width
	ti.CharLimit = 256
	ti.Blur()
	return ti
}

func (m FileModel) Init() tea.Cmd { return nil }

func (m FileModel) Targets() []*common.NodeInfo { return m.targets }

// Loading 返回文件传输是否仍在进行中
func (m FileModel) Loading() bool { return m.loading }

// Results 返回最近一次文件传输的结果列表
func (m FileModel) Results() []transfer.TransferResult { return m.results }

func (m *FileModel) CaptureTargets(nodes []*common.NodeInfo) {
	m.targets = append([]*common.NodeInfo(nil), nodes...)
}

// FillNodes 填显式节点列表并清空组/标签条件
func (m *FileModel) FillNodes(ids []string) {
	m.nodesInput.SetValue(strings.Join(ids, ","))
	m.groupsInput.SetValue("")
	m.labelsInput.SetValue("")
}

// FillConditions 填分组/标签条件并清空显式节点列表(组/标签为空即清空)
func (m *FileModel) FillConditions(groups []string, labels map[string]string) {
	m.nodesInput.SetValue("")
	m.groupsInput.SetValue(strings.Join(groups, ","))
	var pairs []string
	for k, v := range labels {
		if v != "" {
			pairs = append(pairs, k+"="+v)
		} else {
			pairs = append(pairs, k)
		}
	}
	sort.Strings(pairs)
	m.labelsInput.SetValue(strings.Join(pairs, ","))
}

func (m FileModel) NodesValue() string { return m.nodesInput.Value() }

func (m FileModel) GroupsValue() string { return m.groupsInput.Value() }

// ResolveForTest 导出目标解析(测试断言快照回退)
func (m FileModel) ResolveForTest() ([]*common.NodeInfo, error) { return m.resolveTargets() }

func (m FileModel) current() Loc { return m.stack[len(m.stack)-1] }

func (m *FileModel) push(l Loc) { m.stack = append(m.stack, l) }

func (m *FileModel) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m FileModel) Mode() Mode { return m.mode }

func (m FileModel) InsertMode() bool { return m.mode != ModeNormal }

func (m FileModel) IsDirty() bool { return false }

func (m FileModel) Path() []string {
	sub := opNames[m.op]
	switch m.current() {
	case LocAdvanced:
		return []string{"file", sub, "advanced"}
	case LocResult:
		return []string{"file", sub, "result"}
	case LocBrowser:
		return []string{"file", sub, "browser"}
	default:
		return []string{"file", sub}
	}
}

// applyOpPlaceholders 按操作切换文件/目标目录字段的占位提示(下载为本地目录,默认 .)
func (m *FileModel) applyOpPlaceholders() {
	if m.op == OpDownload {
		m.destInput.Placeholder = "本地目录 (默认 .)"
		m.fileInput.Placeholder = "远程文件路径 (必填)"
	} else {
		m.destInput.Placeholder = "目标目录 (默认 /tmp)"
		m.fileInput.Placeholder = "本地文件路径 (必填)"
	}
}

func (m *FileModel) fieldAt(i int) *textinput.Model {
	switch i {
	case 0:
		return &m.fileInput
	case 1:
		return &m.nodesInput
	case 2:
		return &m.groupsInput
	case 3:
		return &m.labelsInput
	default:
		return &m.destInput
	}
}

func (m FileModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 传输完成消息在任何位置都必须被消费,否则结果丢失且 loading 卡死
	switch msg.(type) {
	case UploadDoneMsg, DownloadDoneMsg:
		return m.updateResult(msg)
	}
	switch m.current() {
	case LocAdvanced:
		return m.updateAdvanced(msg)
	case LocResult:
		return m.updateResult(msg)
	case LocBrowser:
		return m.updateBrowser(msg)
	default:
		return m.updateFile(msg)
	}
}

func (m FileModel) updateFile(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mode == ModeInsert {
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			m.mode = ModeNormal
			m.fieldAt(m.cursor).Blur()
			return m, nil
		}
		f := m.fieldAt(m.cursor)
		var cmd tea.Cmd
		*f, cmd = f.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		m.cursor = (m.cursor - 1 + 5) % 5
	case "down":
		m.cursor = (m.cursor + 1) % 5
	case "enter":
		if m.cursor == 0 && m.op != OpDownload {
			// 本地文件字段: 弹出文件浏览器(起点为启动工作目录)
			m.browser = NewFileBrowser("")
			m.push(LocBrowser)
			return m, nil
		}
		m.mode = ModeInsert
		m.fieldAt(m.cursor).Focus()
	case "left":
		m.op = Op((int(m.op) - 1 + 3) % 3)
		m.applyOpPlaceholders()
	case "right":
		m.op = Op((int(m.op) + 1) % 3)
		m.applyOpPlaceholders()
	case "a":
		m.advanced = newAdvancedForm()
		m.push(LocAdvanced)
	case "r":
		var cmd tea.Cmd
		var err error
		switch m.op {
		case OpDownload:
			cmd, err = m.startDownload()
		default:
			cmd, err = m.startUpload()
		}
		if err != nil {
			m.error = err.Error()
			return m, nil
		}
		m.error = ""
		m.push(LocResult)
		return m, cmd
	case "esc":
		return m, func() tea.Msg { return LeavePanelMsg{} }
	}
	return m, nil
}

// updateBrowser 文件浏览器按键处理; 路径输入用 / 打开(同 nodes filter 范式)
func (m FileModel) updateBrowser(msg tea.Msg) (tea.Model, tea.Cmd) {
	b := m.browser
	if b == nil {
		return m, nil
	}
	if b.inputOpen {
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "esc":
				b.inputOpen = false
				b.input.Blur()
				b.err = ""
				m.mode = ModeNormal
				return m, nil
			case "enter":
				if err := b.Jump(b.input.Value()); err != nil {
					b.err = err.Error()
				} else {
					b.err = ""
					b.inputOpen = false
					b.input.Blur()
					m.mode = ModeNormal
				}
				return m, nil
			}
		}
		var cmd tea.Cmd
		b.input, cmd = b.input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		b.Up()
	case "down":
		b.Down()
	case "enter":
		path, isFile, _ := b.Enter()
		if !isFile {
			return m, nil // 已进入子目录
		}
		m.fileInput.SetValue(path)
		m.pop()
		m.browser = nil
		m.mode = ModeInsert
		m.fileInput.Focus()
		return m, textinput.Blink
	case "backspace", "left":
		b.Parent()
	case "/":
		b.inputOpen = true
		b.input.Focus()
		m.mode = ModeInsert
		return m, textinput.Blink
	case "h":
		b.ToggleHidden()
	case "esc":
		m.pop()
		m.browser = nil
		m.mode = ModeInsert
		m.fileInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m FileModel) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case UploadDoneMsg:
		m.loading = false
		m.results = msg.Results
		return m, nil
	case DownloadDoneMsg:
		m.loading = false
		m.results = msg.Results
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.pop()
		case "r":
			var cmd tea.Cmd
			var err error
			switch m.op {
			case OpDownload:
				cmd, err = m.startDownload()
			default:
				cmd, err = m.startUpload()
			}
			if err != nil {
				m.error = err.Error()
				m.pop()
				return m, nil
			}
			m.error = ""
			return m, cmd
		}
	}
	return m, nil
}

func (m *FileModel) resolveTargets() ([]*common.NodeInfo, error) {
	all, err := m.store.List()
	if err != nil {
		return nil, err
	}
	var nodes []*common.NodeInfo
	switch {
	case m.nodesInput.Value() != "":
		want := map[string]bool{}
		for _, id := range splitTrim(m.nodesInput.Value(), ",") {
			want[id] = true
		}
		for _, n := range all {
			if want[n.ID] {
				nodes = append(nodes, n)
			}
		}
	case m.groupsInput.Value() != "":
		groups := splitTrim(m.groupsInput.Value(), ",")
		for _, n := range all {
			if groupsIntersect(n.Groups, groups) {
				nodes = append(nodes, n)
			}
		}
	case m.labelsInput.Value() != "":
		labels := parseLabels(m.labelsInput.Value())
		for _, n := range all {
			if labelsMatch(n.Labels, labels) {
				nodes = append(nodes, n)
			}
		}
	default:
		nodes = m.targets
	}
	return dedupeSorted(nodes), nil
}

func splitTrim(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func groupsIntersect(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func parseLabels(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range splitTrim(s, ",") {
		if k, v, ok := strings.Cut(pair, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		} else {
			out[strings.TrimSpace(pair)] = ""
		}
	}
	return out
}

func labelsMatch(labels map[string]string, want map[string]string) bool {
	if labels == nil {
		return false
	}
	for k, v := range want {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func dedupeSorted(nodes []*common.NodeInfo) []*common.NodeInfo {
	seen := map[string]bool{}
	out := make([]*common.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n == nil || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
