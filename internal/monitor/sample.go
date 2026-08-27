// Package monitor 提供智能监控核心引擎：无代理 SSH 采集、指标存储、
// 规则告警与对策库。与 serve 插件解耦，便于独立测试与复用。
package monitor

// Sample 一个指标数据点，由 (NodeID, Metric, TS) 唯一确定。
type Sample struct {
	NodeID string
	Metric string
	TS     int64 // unix 秒
	Value  float64
}
