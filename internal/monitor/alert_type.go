package monitor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Severity 告警级别。
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warn"
	SeverityCritical Severity = "critical"
)

// SeverityRank 级别数值（用于升级判断）。
func SeverityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 2
	case SeverityWarning:
		return 1
	default:
		return 0
	}
}

// RuleParams 通用规则参数（阈值 + 持续采样次数），JSON 序列化存于
// alert_types.default_params。Metric 为指标名或前缀（如 "disk.usage."）。
// PerCore 设置后阈值 = sys.cores × PerCore（用于负载类规则）。
type RuleParams struct {
	Metric   string  `json:"metric"`
	Op       string  `json:"op"` // > >= < <=
	Value    float64 `json:"value"`
	PerCore  float64 `json:"per_core,omitempty"`
	Duration int     `json:"duration"` // 连续满足 N 次采样才触发
}

// AlertType 告警类型：告警 ID 的载体，附带检测规则与处置开关。
type AlertType struct {
	ID              string     `json:"id"`
	Category        string     `json:"category"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	DefaultSeverity Severity   `json:"default_severity"`
	DefaultParams   RuleParams `json:"default_params"`
	AutoApprove     bool       `json:"auto_approve"` // 自动执行放行，默认全关
	Notifiable      bool       `json:"notifiable"`
	Enabled         bool       `json:"enabled"`
	Builtin         bool       `json:"builtin"`
	// 自定义检查（非内置类型可选）：每轮采集在节点上执行 CheckCmd 并按
	// CheckMode 解析为数值指标 custom.<小写类型ID>，再走常规阈值规则评估。
	CheckCmd     string `json:"check_cmd"`               // SSH 执行的检查命令，空 = 无自定义检查
	CheckMode    string `json:"check_mode"`              // value: 输出即数值 | regex: 捕获组取数 | exit_code: 退出码
	CheckPattern string `json:"check_pattern,omitempty"` // regex 模式的正则（捕获组 1 为数值）
}

// CustomMetricID 自定义告警类型的指标名（由类型 ID 派生）。
func CustomMetricID(typeID string) string { return "custom." + strings.ToLower(typeID) }

// ParseCheckOutput 按 CheckMode 把命令输出解析为指标数值。
// value: 去空白后整体为数值；regex: CheckPattern 捕获组 1；exit_code: 0→0 非 0→1。
func ParseCheckOutput(mode, pattern, stdout string, exitCode int) (float64, error) {
	switch mode {
	case "regex":
		if pattern == "" {
			return 0, fmt.Errorf("regex 模式需要 check_pattern")
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return 0, fmt.Errorf("正则无效: %w", err)
		}
		m := re.FindStringSubmatch(stdout)
		if len(m) < 2 {
			return 0, fmt.Errorf("输出未匹配正则捕获组")
		}
		return strconv.ParseFloat(strings.TrimSpace(m[1]), 64)
	case "exit_code":
		if exitCode == 0 {
			return 0, nil
		}
		return 1, nil
	default: // value
		return strconv.ParseFloat(strings.TrimSpace(stdout), 64)
	}
}

// builtinRegistry 内置告警类型种子（第一期 12 个）。
var builtinRegistry = []AlertType{
	{
		ID: "OWL-DSK-001", Category: "disk", Name: "磁盘使用率过高",
		Description:     "某挂载点磁盘使用率超过阈值",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "disk.usage.", Op: ">", Value: 90, Duration: 2},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-DSK-002", Category: "disk", Name: "inode 使用率过高",
		Description:     "某挂载点 inode 使用率超过阈值",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "disk.inodes.", Op: ">", Value: 90, Duration: 2},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-MEM-001", Category: "mem", Name: "内存使用率过高",
		Description:     "内存使用率（基于 available）超过阈值",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "mem.used_pct", Op: ">", Value: 90, Duration: 3},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-MEM-002", Category: "mem", Name: "swap 使用过高",
		Description:     "swap 使用率超过阈值",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "mem.swap_pct", Op: ">", Value: 50, Duration: 3},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-CPU-001", Category: "cpu", Name: "负载持续过高（卡顿）",
		Description:     "load1 持续高于核数 × 系数",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "load.load1", Op: ">", PerCore: 1.5, Duration: 3},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-NET-001", Category: "net", Name: "网卡流量异常",
		Description:     "入/出速率超过阈值（B/s）",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "net.rx_rate.", Op: ">", Value: 500e6, Duration: 2},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-NET-002", Category: "net", Name: "TCP 连接堆积",
		Description:     "TIME_WAIT 连接数超过阈值",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "net.tcp_timewait", Op: ">", Value: 5000, Duration: 2},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-SVC-001", Category: "svc", Name: "关键服务停止",
		Description:     "受监控服务非 active 状态",
		DefaultSeverity: SeverityCritical,
		DefaultParams:   RuleParams{Metric: "svc.active.", Op: "<", Value: 1, Duration: 2},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-SVC-002", Category: "svc", Name: "服务反复重启",
		Description:     "受监控服务短时间内多次重启",
		DefaultSeverity: SeverityCritical,
		DefaultParams:   RuleParams{Metric: "svc.restarts.", Op: ">", Value: 3, Duration: 1},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-ERR-001", Category: "err", Name: "日志高频错误",
		Description:     "近 5 分钟错误日志条数超过阈值",
		DefaultSeverity: SeverityWarning,
		DefaultParams:   RuleParams{Metric: "err.journal_errors", Op: ">", Value: 50, Duration: 1},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-ERR-002", Category: "err", Name: "OOM 事件",
		Description:     "检测到内核 OOM killer 事件",
		DefaultSeverity: SeverityCritical,
		DefaultParams:   RuleParams{Metric: "err.oom", Op: ">", Value: 0, Duration: 1},
		Notifiable:      true, Enabled: true, Builtin: true,
	},
	{
		ID: "OWL-OSS-001", Category: "avail", Name: "节点失联",
		Description:     "SSH 连续失败达到阈值（引擎信号，非指标规则）",
		DefaultSeverity: SeverityCritical,
		Notifiable:      true, Enabled: true, Builtin: true,
	},
}

// BuiltinAlertTypes 返回内置告警类型注册表副本。
func BuiltinAlertTypes() []AlertType {
	out := make([]AlertType, len(builtinRegistry))
	copy(out, builtinRegistry)
	return out
}

// FindAlertType 按告警 ID 查找内置告警类型。
func FindAlertType(id string) (AlertType, bool) {
	for _, at := range builtinRegistry {
		if at.ID == id {
			return at, true
		}
	}
	return AlertType{}, false
}

// AlertTypeID 辅助：格式化校验。
func (at AlertType) String() string {
	return fmt.Sprintf("%s(%s)", at.ID, at.Name)
}
