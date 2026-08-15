package file

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// LeavePanelMsg 请求 App 切回 Nodes 面板
type LeavePanelMsg struct{}

type FileModel struct {
	store   common.NodeStore
	targets []*common.NodeInfo
}

func NewModel(store common.NodeStore) FileModel { return FileModel{store: store} }

func (m FileModel) Targets() []*common.NodeInfo { return m.targets }

func (m *FileModel) CaptureTargets(nodes []*common.NodeInfo) {
	m.targets = append([]*common.NodeInfo(nil), nodes...)
}

func (m FileModel) Init() tea.Cmd                           { return nil }
func (m FileModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m FileModel) View() string                            { return "file" }
func (m FileModel) InsertMode() bool                        { return false }
func (m FileModel) Path() []string                          { return []string{"file"} }
func (m FileModel) IsDirty() bool                           { return false }
