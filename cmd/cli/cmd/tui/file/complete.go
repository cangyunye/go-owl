package file

import (
	complete "github.com/cangyunye/go-owl/cmd/cli/cmd/tui/complete"
)

// completionCands 当前激活字段的补全候选来源;返回 nil 表示该字段不补全
// (本地/远程文件与目标目录字段)。候选来自节点库全量,与 resolveTargets 一致。
func (m *FileModel) completionCands() []complete.Candidate {
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
func (m *FileModel) syncCompletion() {
	m.comp.Sync(m.fieldAt(m.cursor), m.completionCands)
}

// moveField 编辑态内直接切换字段(补全菜单关闭时 ↑↓),保持 Insert 并重置补全抑制。
func (m *FileModel) moveField(d int) {
	m.fieldAt(m.cursor).Blur()
	m.cursor = (m.cursor + d + 5) % 5
	m.comp.Reset()
	m.fieldAt(m.cursor).Focus()
	m.syncCompletion()
}
