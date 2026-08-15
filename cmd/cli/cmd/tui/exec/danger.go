package exec

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cangyunye/go-owl/internal/control/blacklist"
)

type BlockedInfo struct {
	NodeID  string
	User    string
	Matches []blacklist.MatchItem
}

// checkBlacklist 可注入的黑名单检查(测试替换)
var checkBlacklist = func(nodeUsers map[string]string, cmd string) []BlockedInfo {
	cfg, err := blacklist.LoadConfig()
	if err != nil {
		return nil
	}
	checker := blacklist.NewChecker(cfg)
	var out []BlockedInfo
	for id, user := range nodeUsers {
		r := checker.Check(user, cmd)
		if r.Blocked {
			out = append(out, BlockedInfo{NodeID: id, User: user, Matches: r.Matches})
		}
	}
	return out
}

func (m ExecModel) updateDanger(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.String() {
	case "y":
		m.pop()
		return m, m.launchRun(m.pendingIDs, m.pendingCmd, m.pendingOpts)
	case "n", "esc":
		m.pop()
		return m, nil
	}
	return m, nil
}
