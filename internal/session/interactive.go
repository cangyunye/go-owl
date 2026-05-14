package session

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// InteractiveLoop 交互循环
type InteractiveLoop struct {
	session        *Session
	parser         *CommandParser
	commandTimeout time.Duration
}

// NewInteractiveLoop 创建交互循环
func NewInteractiveLoop(session *Session) *InteractiveLoop {
	return &InteractiveLoop{
		session:        session,
		parser:         NewCommandParser(session.history),
		commandTimeout: 60 * time.Second,
	}
}

// Run 运行交互循环
func (l *InteractiveLoop) Run() error {
	reader := bufio.NewReader(os.Stdin)

	for {
		// 显示提示符
		l.printPrompt()

		// 读取输入
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		// 处理输入
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// 处理命令
		if l.handleCommand(input) {
			break
		}
	}

	return nil
}

// printPrompt 显示提示符
func (l *InteractiveLoop) printPrompt() {
	stats := l.session.GetConnectionStats()

	switch l.session.Mode {
	case SessionModeSingle:
		if len(stats.NodeIDs) > 0 {
			fmt.Printf("(%s) > ", stats.NodeIDs[0])
		} else {
			fmt.Printf("(disconnected) > ")
		}
	case SessionModeMultiple:
		fmt.Printf("(%d nodes) > ", stats.ActiveConnections)
	}
}

// handleCommand 处理命令
func (l *InteractiveLoop) handleCommand(input string) bool {
	// 解析命令
	command, _ := l.parser.ParseCommand(input)

	// 处理内置命令
	switch command {
	case "exit", "quit":
		return true
	case "help":
		l.printHelp()
		return false
	case "history":
		l.printHistory()
		return false
	case "nodes":
		l.printNodes()
		return false
	case "clear":
		l.clearScreen()
		return false
	}

	// 执行远程命令
	if command != "" {
		l.executeRemoteCommand(input)
	}

	return false
}

// executeRemoteCommand 执行远程命令
func (l *InteractiveLoop) executeRemoteCommand(command string) {
	results := l.session.ExecuteCommand(command, l.commandTimeout)

	switch l.session.Mode {
	case SessionModeSingle:
		// 单节点：显示完整输出
		for _, result := range results {
			if result.Error != nil {
				fmt.Printf("✗ 错误: %v\n", result.Error)
			} else {
				fmt.Println(result.Output)
			}
		}

	case SessionModeMultiple:
		// 多节点：表格形式显示
		l.printResultsTable(results)
	}
}

// printResultsTable 表格形式打印结果
func (l *InteractiveLoop) printResultsTable(results []CommandResult) {
	if len(results) == 0 {
		return
	}

	// 计算列宽
	maxNodeIDLen := 8
	for _, r := range results {
		if len(r.NodeID) > maxNodeIDLen {
			maxNodeIDLen = len(r.NodeID)
		}
	}
	if maxNodeIDLen < 8 {
		maxNodeIDLen = 8
	}

	// 打印表格
	fmt.Printf("┌%s┬─────────┬────────┬─────────┐\n", strings.Repeat("─", maxNodeIDLen+2))
	fmt.Printf("│ %-*s │ 返回码  │  状态  │ 耗时(ms) │\n", maxNodeIDLen, "节点")
	fmt.Printf("├%s┼─────────┼────────┼─────────┤\n", strings.Repeat("─", maxNodeIDLen+2))

	for _, r := range results {
		status := "✓"
		if r.Error != nil || r.ExitCode != 0 {
			status = "✗"
		}
		durationMs := r.Duration.Nanoseconds() / 1000000
		fmt.Printf("│ %-*s │   %02d    │   %s    │ %9d │\n",
			maxNodeIDLen,
			truncateString(r.NodeID, maxNodeIDLen),
			r.ExitCode,
			status,
			durationMs,
		)
	}

	fmt.Printf("└%s┴─────────┴────────┴─────────┘\n", strings.Repeat("─", maxNodeIDLen+2))

	// 打印汇总信息
	l.printResultSummary(results)

	// 如果有错误，显示提示
	for _, r := range results {
		if r.Error != nil || r.ExitCode != 0 {
			fmt.Printf("错误详情请查看: owl session history --session-id %s\n", l.session.ID)
			break
		}
	}
}

// printResultSummary 打印结果汇总
func (l *InteractiveLoop) printResultSummary(results []CommandResult) {
	if len(results) == 0 {
		return
	}

	successCount := 0
	failCount := 0
	totalDuration := time.Duration(0)

	for _, r := range results {
		totalDuration += r.Duration
		if r.Error == nil && r.ExitCode == 0 {
			successCount++
		} else {
			failCount++
		}
	}

	fmt.Printf("\n执行汇总:\n")
	fmt.Printf("  目标节点: %d 个\n", len(results))
	fmt.Printf("  成功:     %d 个\n", successCount)
	fmt.Printf("  失败:     %d 个\n", failCount)
	fmt.Printf("  平均耗时: %.2f ms\n", float64(totalDuration.Nanoseconds()/1000000)/float64(len(results)))
	fmt.Println()
}

// printHelp 显示帮助
func (l *InteractiveLoop) printHelp() {
	fmt.Println("可用命令:")
	fmt.Println("  help     - 显示帮助信息")
	fmt.Println("  history  - 显示命令历史")
	fmt.Println("  nodes    - 显示当前连接的节点")
	fmt.Println("  clear    - 清屏")
	fmt.Println("  exit     - 优雅退出会话")
	fmt.Println()
	fmt.Println("远程命令:")
	fmt.Println("  输入任何 shell 命令将在远程节点执行")
	fmt.Println()
}

// printHistory 显示历史
func (l *InteractiveLoop) printHistory() {
	history := l.session.GetHistory()
	if len(history) == 0 {
		fmt.Println("暂无历史命令")
		return
	}

	for idx, cmd := range history {
		fmt.Printf("  %d  %s\n", idx+1, cmd)
	}
}

// printNodes 显示节点
func (l *InteractiveLoop) printNodes() {
	stats := l.session.GetConnectionStats()
	fmt.Printf("当前连接的 %d 个节点:\n", stats.ActiveConnections)
	for _, nodeID := range stats.NodeIDs {
		fmt.Printf("  - %s\n", nodeID)
	}
}

// clearScreen 清屏
func (l *InteractiveLoop) clearScreen() {
	fmt.Print("\033[2J\033[H")
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
