package exec

import (
	complete "github.com/cangyunye/go-owl/cmd/cli/cmd/tui/complete"
)

// completionCands 当前激活字段的补全候选来源;返回 nil 表示该字段不补全(命令字段)。
// 候选来自节点库全量,与 resolveTargets 的匹配范围一致。
func (m *ExecModel) completionCands() []complete.Candidate {
	all, err := m.store.List()
	if err != nil {
		return nil
	}
	switch m.cursor {
	case 1:
		return complete.NodeCandidates(all)
	case 2:
		return complete.GroupCandidates(all)
	case 3:
		return complete.LabelCandidates(all)
	}
	return nil
}

// syncCompletion 按激活字段刷新补全菜单。
func (m *ExecModel) syncCompletion() {
	m.comp.Sync(m.fieldAt(m.cursor), m.completionCands)
}
