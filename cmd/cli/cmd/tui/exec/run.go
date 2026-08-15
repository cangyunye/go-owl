package exec

import (
	"context"
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/node"
)

type ExecStreamMsg struct {
	ch chan command.CommandResult
}

type ExecResultMsg struct {
	Result command.CommandResult
}

type ExecDoneMsg struct{}

// runStream 可注入的命令执行器(测试替换);默认实现走 command.Executor + RunStreaming
var runStream = func(ctx context.Context, ids []string, cmd string, opts *command.ExecuteOptions) (<-chan command.CommandResult, func()) {
	ex := command.NewExecutor(node.NewNodeResolver())
	return ex.RunStreaming(ctx, ids, cmd, opts), ex.Close
}

func (m *ExecModel) startRun() (tea.Cmd, error) {
	cmd := strings.TrimSpace(m.cmdInput.Value())
	if cmd == "" {
		return nil, errors.New("命令不能为空")
	}
	nodes, err := m.resolveTargets()
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, errors.New("没有目标节点")
	}
	f := m.advanced
	if f == nil {
		f = newAdvancedForm()
	}
	opts, err := f.buildOpts()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	return m.launchRun(ids, cmd, opts), nil
}

func (m *ExecModel) launchRun(ids []string, cmd string, opts *command.ExecuteOptions) tea.Cmd {
	m.lastCmd = cmd
	m.lastIDs = ids
	m.lastOpts = opts
	m.results = nil
	m.loading = true
	m.push(LocResult)
	return func() tea.Msg {
		ch, stop := runStream(context.Background(), ids, cmd, opts)
		out := make(chan command.CommandResult, len(ids))
		go func() {
			defer stop()
			defer close(out)
			for r := range ch {
				out <- r
			}
		}()
		return ExecStreamMsg{ch: out}
	}
}

func pumpResults(ch chan command.CommandResult) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return ExecDoneMsg{}
		}
		return ExecResultMsg{Result: r}
	}
}
