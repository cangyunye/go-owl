package transfer

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
)

// RelayTarget 表示一个中继目标节点及其认证信息
type RelayTarget struct {
	Host     string // user@host:/path 格式
	Password string // 可能为空
}

// RelayTargetResult 表示 bash 脚本 CSV 输出中单个目标的结果
type RelayTargetResult struct {
	Target     string
	Status     string // success, failed, timeout, auth_failed
	Error      string
	DurationMs int64
}

// RelaySubTask 表示一个源节点向多个目标节点传输的中继子任务
type RelaySubTask struct {
	SourceNodeID string        // 执行中继的源节点
	Targets      []RelayTarget // 目标列表及其认证信息
	SourceFile   string        // 源节点上要中继的文件路径
	TimeoutSec   int           // 每个目标的 SCP 超时时间
}

// ToShellArgs 将 RelaySubTask 序列化为 bash 脚本的命令行参数
func (r *RelaySubTask) ToShellArgs() []string {
	targetHosts := make([]string, len(r.Targets))
	targetPasswords := make([]string, len(r.Targets))
	hasPassword := false

	for i, t := range r.Targets {
		targetHosts[i] = t.Host
		targetPasswords[i] = t.Password
		if t.Password != "" {
			hasPassword = true
		}
	}

	args := []string{
		"--source", r.SourceFile,
		"--targets", strings.Join(targetHosts, ","),
		"--timeout", strconv.Itoa(r.TimeoutSec),
	}

	if hasPassword {
		args = append(args, "--passwords", strings.Join(targetPasswords, ","))
	}

	return args
}

// ParseRelayResults 解析 bash 脚本输出的 CSV 格式结果
func ParseRelayResults(csvOutput string) ([]RelayTargetResult, error) {
	if csvOutput == "" {
		return nil, nil
	}

	reader := csv.NewReader(strings.NewReader(csvOutput))
	reader.TrimLeadingSpace = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("读取 CSV 头部失败: %w", err)
	}

	if err := validateHeader(header); err != nil {
		return nil, err
	}

	var results []RelayTargetResult

	for {
		record, err := reader.Read()
		if err != nil {
			break
		}

		if len(record) != 4 {
			fmt.Printf("跳过列数不正确的行（预期 4 列，实际 %d 列）: %v\n", len(record), record)
			continue
		}

		durationMs, parseErr := strconv.ParseInt(record[3], 10, 64)
		if parseErr != nil {
			fmt.Printf("跳过无法解析 duration_ms 的行: %v, 错误: %v\n", record, parseErr)
			continue
		}

		results = append(results, RelayTargetResult{
			Target:     record[0],
			Status:     record[1],
			Error:      record[2],
			DurationMs: durationMs,
		})
	}

	return results, nil
}

func validateHeader(header []string) error {
	expected := []string{"target", "status", "error", "duration_ms"}

	if len(header) != len(expected) {
		return fmt.Errorf("CSV 头部格式不正确，预期 %d 列，实际 %d 列", len(expected), len(header))
	}

	for i, h := range header {
		if strings.TrimSpace(h) != expected[i] {
			return fmt.Errorf("CSV 头部不匹配，预期列 %d 为 '%s'，实际为 '%s'", i+1, expected[i], h)
		}
	}

	return nil
}
