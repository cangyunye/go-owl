package session

import (
	"strconv"
	"strings"
	"sync"
)

// CommandHistory 命令历史
type CommandHistory struct {
	commands []string
	maxSize  int
	mu       sync.RWMutex
}

// NewCommandHistory 创建命令历史
func NewCommandHistory(maxSize int) *CommandHistory {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &CommandHistory{
		commands: make([]string, 0, maxSize),
		maxSize:  maxSize,
	}
}

// Add 添加命令到历史
func (h *CommandHistory) Add(command string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 去除空白字符
	command = strings.TrimSpace(command)
	if command == "" {
		return
	}

	// 避免重复相邻的命令
	if len(h.commands) > 0 && h.commands[len(h.commands)-1] == command {
		return
	}

	// 添加到末尾
	h.commands = append(h.commands, command)

	// 如果超过最大容量，删除最早的
	if len(h.commands) > h.maxSize {
		h.commands = h.commands[len(h.commands)-h.maxSize:]
	}
}

// GetAll 获取所有历史命令
func (h *CommandHistory) GetAll() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]string, len(h.commands))
	copy(result, h.commands)
	return result
}

// GetMatching 获取匹配的命令
func (h *CommandHistory) GetMatching(prefix string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}

	var matches []string
	for _, cmd := range h.commands {
		if strings.HasPrefix(cmd, prefix) {
			matches = append(matches, cmd)
		}
	}

	return matches
}

// GetByIndex 根据索引获取命令
func (h *CommandHistory) GetByIndex(indexStr string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 支持负数索引
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		// 尝试获取最后一条以 prefix 开头的命令
		prefix := indexStr
		if len(h.commands) > 0 {
			for i := len(h.commands) - 1; i >= 0; i-- {
				if strings.HasPrefix(h.commands[i], prefix) {
					return h.commands[i]
				}
			}
		}
		return ""
	}

	// 转换负数索引
	if index < 0 {
		index = len(h.commands) + index
	}

	// 检查范围
	if index < 0 || index >= len(h.commands) {
		return ""
	}

	return h.commands[index]
}

// GetLast 获取最后 n 条命令
func (h *CommandHistory) GetLast(n int) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if n <= 0 || len(h.commands) == 0 {
		return nil
	}

	if n > len(h.commands) {
		n = len(h.commands)
	}

	start := len(h.commands) - n
	result := make([]string, n)
	copy(result, h.commands[start:])

	return result
}

// Clear 清空历史
func (h *CommandHistory) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commands = make([]string, 0, h.maxSize)
}

// Size 获取历史命令数量
func (h *CommandHistory) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.commands)
}
