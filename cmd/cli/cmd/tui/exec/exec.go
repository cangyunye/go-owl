package exec

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/control/command"
)

type Loc int

const (
	LocRun Loc = iota
	LocAdvanced
	LocResult
	LocDanger
)

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
)

type LeavePanelMsg struct{}

var formats = []string{"simple", "detail", "json"}

type ExecModel struct {
	store common.NodeStore

	stack  []Loc
	mode   Mode
	cursor int

	cmdInput    textinput.Model
	nodesInput  textinput.Model
	groupsInput textinput.Model
	labelsInput textinput.Model
	format      string
	formatIdx   int

	advanced *AdvancedForm

	targets []*common.NodeInfo

	runCh    chan command.CommandResult
	lastCmd  string
	lastIDs  []string
	lastOpts *command.ExecuteOptions
	loading  bool
	results  []command.CommandResult

	error string
}

func NewModel(store common.NodeStore) ExecModel {
	return ExecModel{
		store:       store,
		stack:       []Loc{LocRun},
		cmdInput:    newInput("输入要执行的命令 (必填)", 60),
		nodesInput:  newInput("节点 ID,逗号分隔 (留空=当前过滤可见)", 40),
		groupsInput: newInput("分组,逗号分隔", 40),
		labelsInput: newInput("标签 k=v,逗号分隔", 40),
		format:      "simple",
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

func (m ExecModel) Targets() []*common.NodeInfo { return m.targets }

func (m ExecModel) Init() tea.Cmd { return nil }

func (m *ExecModel) CaptureTargets(nodes []*common.NodeInfo) {
	m.targets = append([]*common.NodeInfo(nil), nodes...)
}

func (m ExecModel) current() Loc { return m.stack[len(m.stack)-1] }

func (m *ExecModel) push(l Loc) { m.stack = append(m.stack, l) }

func (m *ExecModel) pop() {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m ExecModel) Mode() Mode { return m.mode }

func (m ExecModel) InsertMode() bool { return m.mode != ModeNormal }

func (m ExecModel) IsDirty() bool { return false }

func (m ExecModel) Path() []string {
	switch m.current() {
	case LocAdvanced:
		return []string{"exec", "run", "advanced"}
	case LocResult:
		return []string{"exec", "run", "result"}
	case LocDanger:
		return []string{"exec", "run", "danger"}
	default:
		return []string{"exec", "run"}
	}
}

func (m *ExecModel) fieldAt(i int) *textinput.Model {
	switch i {
	case 0:
		return &m.cmdInput
	case 1:
		return &m.nodesInput
	case 2:
		return &m.groupsInput
	default:
		return &m.labelsInput
	}
}

func (m ExecModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.current() {
	case LocAdvanced:
		return m.updateAdvanced(msg)
	case LocResult:
		return m.updateResult(msg)
	case LocDanger:
		return m.updateDanger(msg)
	default:
		return m.updateRun(msg)
	}
}

func (m ExecModel) updateRun(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.cursor = (m.cursor - 1 + 4) % 4
	case "down":
		m.cursor = (m.cursor + 1) % 4
	case "enter":
		m.mode = ModeInsert
		m.fieldAt(m.cursor).Focus()
	case "f":
		m.formatIdx = (m.formatIdx + 1) % len(formats)
		m.format = formats[m.formatIdx]
	case "r":
		cmd, err := m.startRun()
		if err != nil {
			m.error = err.Error()
			return m, nil
		}
		return m, cmd
	case "a":
		m.advanced = newAdvancedForm()
		m.push(LocAdvanced)
	case "esc":
		return m, func() tea.Msg { return LeavePanelMsg{} }
	}
	return m, nil
}

func (m ExecModel) updateAdvanced(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.advanced
	if f == nil {
		return m, nil
	}
	if m.mode == ModeInsert {
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			m.mode = ModeNormal
			f.fields[f.cursor].input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		f.fields[f.cursor].input, cmd = f.fields[f.cursor].input.Update(msg)
		return m, cmd
	}
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "up":
		f.move(-1)
	case "down":
		f.move(1)
	case "enter":
		if f.fields[f.cursor].kind == KindText {
			m.mode = ModeInsert
			f.fields[f.cursor].input.SetValue("")
			f.fields[f.cursor].input.Focus()
		}
	case " ":
		if f.fields[f.cursor].kind == KindBool {
			f.toggle(f.cursor)
		}
	case "s":
		m.pop()
	case "esc":
		m.pop()
		m.advanced = nil
	}
	return m, nil
}
func (m ExecModel) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ExecStreamMsg:
		m.loading = false
		m.results = nil
		m.runCh = msg.ch
		return m, pumpResults(msg.ch)
	case ExecResultMsg:
		m.results = append(m.results, msg.Result)
		return m, pumpResults(m.runCh)
	case ExecDoneMsg:
		m.loading = false
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.pop()
			return m, nil
		case "r":
			cmd, err := m.startRun()
			if err != nil {
				m.error = err.Error()
				return m, nil
			}
			return m, cmd
		}
	}
	return m, nil
}
func (m ExecModel) updateDanger(msg tea.Msg) (tea.Model, tea.Cmd)   { return m, nil }

func (m *ExecModel) resolveTargets() ([]*common.NodeInfo, error) {
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
