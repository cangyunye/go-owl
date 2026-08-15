package exec

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// LeavePanelMsg 请求 App 切回 Nodes 面板
type LeavePanelMsg struct{}

type ExecModel struct {
	targets []*common.NodeInfo
}

func NewModel(store common.NodeStore) ExecModel { return ExecModel{} }

func (m ExecModel) Targets() []*common.NodeInfo { return m.targets }

func (m *ExecModel) CaptureTargets(nodes []*common.NodeInfo) {
	m.targets = append([]*common.NodeInfo(nil), nodes...)
}

func (m ExecModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m ExecModel) Init() tea.Cmd                            { return nil }
func (m ExecModel) View() string                             { return "exec" }
func (m ExecModel) InsertMode() bool                        { return false }
func (m ExecModel) Path() []string                          { return []string{"exec"} }
func (m ExecModel) IsDirty() bool                           { return false }
