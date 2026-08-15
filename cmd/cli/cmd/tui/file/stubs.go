package file

import (
	tea "github.com/charmbracelet/bubbletea"
)

// 占位实现: 仅保证 Task 3 编译通过。
// Task 4 (run.go: updateResult/resultView) 落地后, 删除本文件, 不得保留重复定义。

func (m FileModel) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

func (m FileModel) resultView() string { return "" }
