package monitor

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Target 采集目标：节点的连接信息。与 internal/node.ResolvedNode 字段对齐，
// 便于 serve / CLI 直接填充。
type Target struct {
	ID          string
	Name        string
	Address     string
	Port        int
	User        string
	SSHKey      string
	SSHPassword string
	ProxyJump   string
	// Services 受监控服务 unit 列表（serve 端来自节点 label monitor.services），
	// 纯 opt-in：为空时不采集 svc.* 指标。
	Services []string
	// Groups 节点所属分组（告警规则触发范围匹配用；CLI 等来源可为空）。
	Groups []string
}

// Execer 执行单条命令并返回退出码与输出。internal/ssh.NodeExecutor 满足该接口。
type Execer interface {
	Execute(command string, timeout time.Duration) (int, string, error)
}

// ExecerFactory 按采集目标创建执行器，采集器只依赖该接口，便于测试注入。
type ExecerFactory interface {
	NewExecer(t *Target) (Execer, error)
}

// Collector 无代理 SSH 采集器：对单节点串行执行采集命令并解析为指标。
// loadavg 为必选命令，失败即整体失败（节点失联信号）；其余命令失败仅跳过。
type Collector struct {
	factory ExecerFactory
	timeout time.Duration
	now     func() int64
}

// NewCollector 创建采集器，默认单命令超时 10s。
func NewCollector(factory ExecerFactory) *Collector {
	return &Collector{
		factory: factory,
		timeout: 10 * time.Second,
		now:     func() int64 { return time.Now().Unix() },
	}
}

// collectStep 一条采集命令及其解析函数。
// required：失败即整体失败（节点失联信号）；optional：失败/解析失败静默
// 跳过（如节点无 journalctl/systemd），且不计入错误日志。
type collectStep struct {
	name     string
	command  string
	required bool
	optional bool
	parse    func(raw, nodeID string, ts int64) ([]Sample, error)
}

// errStepsCommand 常量集中定义，便于测试与命令表保持一致。
const (
	journalErrorsCmd = "journalctl -p err -q --no-pager -o json --since=-5min | wc -l"
	journalOOMCmd    = `journalctl -k -q --no-pager --since=-5min | grep -ciE "out of memory|oom-kill|killed process" || true`
)

// collectSteps 首批采集命令表。
// locale 敏感命令（df/free/ss）强制 LC_ALL=C：非 C locale 节点的本地化
// 表头（如中文「文件系统」「内存：」）会使解析器失配，且部分失败被静默跳过。
// journalctl 读不到 journal（权限/未装）时命令仍以 0 退出并计 0 —— "读不到"
// 与"无事件"同义，对 >0 阈值规则语义正确。
var collectSteps = []collectStep{
	{name: "loadavg", command: "cat /proc/loadavg", required: true, parse: ParseLoadavg},
	{name: "nproc", command: "nproc", parse: ParseNproc},
	{name: "df-usage", command: "LC_ALL=C df -P", parse: func(raw, n string, ts int64) ([]Sample, error) {
		return ParseDF(raw, n, ts, "disk.usage")
	}},
	{name: "df-inodes", command: "LC_ALL=C df -Pi", parse: func(raw, n string, ts int64) ([]Sample, error) {
		return ParseDF(raw, n, ts, "disk.inodes")
	}},
	{name: "free", command: "LC_ALL=C free -m", parse: ParseFree},
	{name: "netdev", command: "cat /proc/net/dev", parse: ParseNetDev},
	{name: "ss", command: "LC_ALL=C ss -s", parse: ParseSS},
	{name: "journal-errors", command: journalErrorsCmd, optional: true, parse: func(raw, n string, ts int64) ([]Sample, error) {
		return ParseJournalCount(raw, n, "err.journal_errors", ts)
	}},
	{name: "oom", command: journalOOMCmd, optional: true, parse: func(raw, n string, ts int64) ([]Sample, error) {
		return ParseJournalCount(raw, n, "err.oom", ts)
	}},
}

// unitNameRe 受监控服务 unit 名白名单：unit 名直接拼入 shell 命令，
// 必须杜绝注入（空格/分号/管道等一律拒绝）。
var unitNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,127}$`)

// svcShowStep 构造受监控服务的 systemctl show 采集步骤。服务列表完全由
// 节点 label monitor.services 显式指定（纯 opt-in）：不默认监控 ssh——
// socket 激活型发行版（如 ssh.socket 拉起 ssh.service）平时
// ssh.service 为 inactive，默认监控会产生永久误报。无配置时返回 false。
func svcShowStep(extra []string) (collectStep, bool) {
	seen := make(map[string]bool, len(extra))
	units := make([]string, 0, len(extra))
	for _, u := range extra {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] || !unitNameRe.MatchString(u) {
			continue
		}
		seen[u] = true
		units = append(units, u)
	}
	if len(units) == 0 {
		return collectStep{}, false
	}
	return collectStep{
		name:     "svc",
		command:  "systemctl show -p Id -p LoadState -p ActiveState -p NRestarts " + strings.Join(units, " "),
		optional: true,
		parse:    ParseSystemctlShow,
	}, true
}

// Collect 对目标节点执行一轮采集，返回全部成功解析的指标。
// 单条非必选命令失败被记录在返回错误中但不中断其余采集（optional 步骤除外）。
func (c *Collector) Collect(ctx context.Context, t *Target) ([]Sample, error) {
	exec, err := c.factory.NewExecer(t)
	if err != nil {
		return nil, fmt.Errorf("monitor: 创建执行器失败: %w", err)
	}
	ts := c.now()

	// 显式拷贝后再追加 svc 步骤：collectSteps 是包级共享切片，
	// 直接 append 可能写入其底层数组，并发采集下产生数据竞争
	steps := make([]collectStep, 0, len(collectSteps)+1)
	steps = append(steps, collectSteps...)
	if svcStep, ok := svcShowStep(t.Services); ok {
		steps = append(steps, svcStep)
	}

	var samples []Sample
	var errs []error
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return samples, err
		}
		code, out, execErr := exec.Execute(step.command, c.timeout)
		if execErr != nil || code != 0 {
			if step.required {
				return nil, fmt.Errorf("monitor: 必选命令 %q 执行失败: %w", step.command, execErr)
			}
			if !step.optional {
				errs = append(errs, fmt.Errorf("monitor: 命令 %q 执行失败: %w", step.command, execErr))
			}
			continue
		}
		parsed, parseErr := step.parse(out, t.ID, ts)
		if parseErr != nil {
			if !step.optional {
				errs = append(errs, fmt.Errorf("monitor: 命令 %q 输出解析失败: %w", step.command, parseErr))
			}
			continue
		}
		samples = append(samples, parsed...)
	}
	return samples, errors.Join(errs...)
}

// RatePerSec 由同一累计计数指标的两个相邻采样计算每秒速率。
// 时间差 <= 0 或计数器重置（重启/网卡重建，值回退）时返回 0。
func RatePerSec(prev, cur Sample) float64 {
	dt := float64(cur.TS - prev.TS)
	if dt <= 0 {
		return 0
	}
	delta := cur.Value - prev.Value
	if delta < 0 {
		return 0
	}
	return delta / dt
}

// ExecCommand 在目标节点执行任意命令并返回标准输出与退出码
//（自定义告警检查用；不走固定采集步骤表）。
func (c *Collector) ExecCommand(ctx context.Context, t *Target, command string, timeout time.Duration) (string, int, error) {
	if err := ctx.Err(); err != nil {
		return "", -1, err
	}
	exec, err := c.factory.NewExecer(t)
	if err != nil {
		return "", -1, fmt.Errorf("monitor: 创建执行器失败: %w", err)
	}
	code, out, err := exec.Execute(command, timeout)
	return out, code, err
}
