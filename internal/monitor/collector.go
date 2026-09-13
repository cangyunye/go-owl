package monitor

import (
	"context"
	"errors"
	"fmt"
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
type collectStep struct {
	name     string
	command  string
	required bool
	parse    func(raw, nodeID string, ts int64) ([]Sample, error)
}

// collectSteps 首批采集命令表（可后续扩展 svc.*/err.* 等）。
// locale 敏感命令（df/free/ss）强制 LC_ALL=C：非 C locale 节点的本地化
// 表头（如中文「文件系统」「内存：」）会使解析器失配，且部分失败被静默跳过。
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
}

// Collect 对目标节点执行一轮采集，返回全部成功解析的指标。
// 单条非必选命令失败被记录在返回错误中但不中断其余采集。
func (c *Collector) Collect(ctx context.Context, t *Target) ([]Sample, error) {
	exec, err := c.factory.NewExecer(t)
	if err != nil {
		return nil, fmt.Errorf("monitor: 创建执行器失败: %w", err)
	}
	ts := c.now()

	var samples []Sample
	var errs []error
	for _, step := range collectSteps {
		if err := ctx.Err(); err != nil {
			return samples, err
		}
		code, out, execErr := exec.Execute(step.command, c.timeout)
		if execErr != nil || code != 0 {
			if step.required {
				return nil, fmt.Errorf("monitor: 必选命令 %q 执行失败: %w", step.command, execErr)
			}
			errs = append(errs, fmt.Errorf("monitor: 命令 %q 执行失败: %w", step.command, execErr))
			continue
		}
		parsed, parseErr := step.parse(out, t.ID, ts)
		if parseErr != nil {
			errs = append(errs, fmt.Errorf("monitor: 命令 %q 输出解析失败: %w", step.command, parseErr))
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
