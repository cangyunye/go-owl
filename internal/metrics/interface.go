// Package metrics 提供监控数据采集的抽象接口
package metrics

import (
	"context"
)

// EndpointConfig 端点配置
type EndpointConfig struct {
	Name    string
	Address string
	NodeRef string
	Labels  map[string]string
}

// MetricFamily 指标族
type MetricFamily struct {
	Name    string
	Metrics []Metric
}

// Metric 单个指标
type Metric struct {
	Value  float64
	Labels map[string]string
}

// ScrapeResult 采集结果
type ScrapeResult struct {
	Endpoint string
	Success  bool
	Latency  float64
	Raw      []byte
}

// Alert 告警信息
type Alert struct {
	Endpoint string
	Severity AlertSeverity
	Message  string
}

// AlertSeverity 告警级别
type AlertSeverity int

const (
	AlertWarning AlertSeverity = iota
	AlertCritical
)

// Scraper 采集器接口
type Scraper interface {
	ScrapeAll(ctx context.Context, endpoints []EndpointConfig, count int) []ScrapeResult
}

// Parser 解析器接口
type Parser interface {
	Parse(raw []byte) ([]MetricFamily, error)
}

// Analyzer 分析器接口
type Analyzer interface {
	Analyze(endpoint string, families []MetricFamily) []Alert
}

// Renderer 渲染器接口
type Renderer interface {
	RenderHeader(now int64, scrapeInterval, nodeCount int) string
	RenderTableHorizontal(columns []TableColumn, rows []NodeTableRow) string
	RenderFooter(results []Result) string
	ClearScreen() string
	HideCursor() string
	ShowCursor() string
}

// TableColumn 表格列
type TableColumn struct {
	Name string
}

// NodeTableRow 节点表格行
type NodeTableRow struct {
	NodeName string
	Values   []string
}

// Result 显示结果
type Result struct {
	Name    string
	Success bool
	Latency int64
}

// MetricsConfig 监控配置
type MetricsConfig struct {
	Endpoints      []EndpointConfig
	ScrapeTimeout  int
	ScrapeInterval int
	MaxDiskColumns int
}


