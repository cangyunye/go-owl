//go:build !metrics

// Package metrics 提供监控数据采集的抽象接口
package metrics

import (
	"context"
	"fmt"
)

// 不带 metrics tag 时的空实现

// IsMetricsEnabled 检查 metrics 功能是否启用
func IsMetricsEnabled() bool {
	return false
}

// NewOwlScraper 返回一个空的采集器
func NewOwlScraper(timeout int64) Scraper {
	return &emptyScraper{}
}

type emptyScraper struct{}

func (e *emptyScraper) ScrapeAll(ctx context.Context, endpoints []EndpointConfig, count int) []ScrapeResult {
	return nil
}

// NewOwlParser 返回一个空的解析器
func NewOwlParser() Parser {
	return &emptyParser{}
}

type emptyParser struct{}

func (e *emptyParser) Parse(raw []byte) ([]MetricFamily, error) {
	return nil, fmt.Errorf("metrics feature not enabled")
}

// NewOwlAnalyzer 返回一个空的分析器
func NewOwlAnalyzer() Analyzer {
	return &emptyAnalyzer{}
}

type emptyAnalyzer struct{}

func (e *emptyAnalyzer) Analyze(endpoint string, families []MetricFamily) []Alert {
	return nil
}

// NewOwlRenderer 返回一个空的渲染器
func NewOwlRenderer() Renderer {
	return &emptyRenderer{}
}

type emptyRenderer struct{}

func (e *emptyRenderer) RenderHeader(now int64, scrapeInterval, nodeCount int) string {
	return ""
}

func (e *emptyRenderer) RenderTableHorizontal(columns []TableColumn, rows []NodeTableRow) string {
	return ""
}

func (e *emptyRenderer) RenderFooter(results []Result) string {
	return ""
}

func (e *emptyRenderer) ClearScreen() string {
	return ""
}

func (e *emptyRenderer) HideCursor() string {
	return ""
}

func (e *emptyRenderer) ShowCursor() string {
	return ""
}

// DefaultConfig 返回默认配置
func DefaultConfig() *MetricsConfig {
	return &MetricsConfig{
		ScrapeTimeout:  10,
		ScrapeInterval: 5,
		MaxDiskColumns: 5,
	}
}

// LoadFile 从文件加载配置
func LoadFile(path string) (*MetricsConfig, error) {
	return DefaultConfig(), nil
}
