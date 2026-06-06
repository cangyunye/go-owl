//go:build metrics

// Package metrics 提供 go-owl-metrics 的适配器实现
package metrics

import (
	"context"
	"strings"
	"time"

	gometricsConfig "github.com/sinvigil/go-owl-metrics/pkg/config"
	gometricsDisplay "github.com/sinvigil/go-owl-metrics/pkg/display"
	gometricsParser "github.com/sinvigil/go-owl-metrics/pkg/parser"
	gometricsScraper "github.com/sinvigil/go-owl-metrics/pkg/scraper"
	gometricsAnalyzer "github.com/sinvigil/go-owl-metrics/pkg/analyzer"
	gometricsTypes "github.com/sinvigil/go-owl-metrics/pkg/types"
)

// IsMetricsEnabled 检查 metrics 功能是否启用
func IsMetricsEnabled() bool {
	return true
}

// OwlScraper go-owl-metrics 采集器适配器
type OwlScraper struct {
	inner *gometricsScraper.Scraper
}

func NewOwlScraper(timeout int64) *OwlScraper {
	return &OwlScraper{
		inner: gometricsScraper.New(time.Duration(timeout)),
	}
}

func (o *OwlScraper) ScrapeAll(ctx context.Context, endpoints []EndpointConfig, count int) []ScrapeResult {
	innerEndpoints := make([]gometricsTypes.EndpointConfig, len(endpoints))
	for i, ep := range endpoints {
		innerEndpoints[i] = gometricsTypes.EndpointConfig{
			Name:    ep.Name,
			Address: ep.Address,
			NodeRef: ep.NodeRef,
			Labels:  ep.Labels,
		}
	}

	innerResults := o.inner.ScrapeAll(ctx, innerEndpoints, count)
	results := make([]ScrapeResult, len(innerResults))
	for i, r := range innerResults {
		results[i] = ScrapeResult{
			Endpoint: r.Endpoint,
			Success:  r.Success,
			Latency:  r.Latency.Seconds(),
			Raw:      r.Raw,
		}
	}
	return results
}

// OwlParser go-owl-metrics 解析器适配器
type OwlParser struct{}

func NewOwlParser() *OwlParser {
	return &OwlParser{}
}

func (o *OwlParser) Parse(raw []byte) ([]MetricFamily, error) {
	innerFamilies, err := gometricsParser.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}

	families := make([]MetricFamily, len(innerFamilies))
	for i, f := range innerFamilies {
		metrics := make([]Metric, len(f.Metrics))
		for j, m := range f.Metrics {
			metrics[j] = Metric{
				Value:  m.Value,
				Labels: m.Labels,
			}
		}
		families[i] = MetricFamily{
			Name:    f.Name,
			Metrics: metrics,
		}
	}
	return families, nil
}

// OwlAnalyzer go-owl-metrics 分析器适配器
type OwlAnalyzer struct {
	inner *gometricsAnalyzer.Analyzer
}

func NewOwlAnalyzer() *OwlAnalyzer {
	return &OwlAnalyzer{
		inner: gometricsAnalyzer.New(),
	}
}

func (o *OwlAnalyzer) Analyze(endpoint string, families []MetricFamily) []Alert {
	innerFamilies := make([]gometricsTypes.MetricFamily, len(families))
	for i, f := range families {
		metrics := make([]gometricsTypes.Metric, len(f.Metrics))
		for j, m := range f.Metrics {
			metrics[j] = gometricsTypes.Metric{
				Value:  m.Value,
				Labels: m.Labels,
			}
		}
		innerFamilies[i] = gometricsTypes.MetricFamily{
			Name:    f.Name,
			Metrics: metrics,
		}
	}

	innerAlerts := o.inner.Analyze(endpoint, innerFamilies)
	alerts := make([]Alert, len(innerAlerts))
	for i, a := range innerAlerts {
		severity := AlertWarning
		if a.Severity == gometricsAnalyzer.AlertCritical {
			severity = AlertCritical
		}
		alerts[i] = Alert{
			Endpoint: a.Endpoint,
			Severity: severity,
			Message:  a.Message,
		}
	}
	return alerts
}

// OwlRenderer go-owl-metrics 渲染器适配器
type OwlRenderer struct {
	inner *gometricsDisplay.Renderer
}

func NewOwlRenderer() *OwlRenderer {
	return &OwlRenderer{
		inner: gometricsDisplay.New(),
	}
}

func (o *OwlRenderer) RenderHeader(now int64, scrapeInterval, nodeCount int) string {
	return o.inner.RenderHeader(time.Unix(0, now), scrapeInterval, nodeCount)
}

func (o *OwlRenderer) RenderTableHorizontal(columns []TableColumn, rows []NodeTableRow) string {
	innerColumns := make([]gometricsDisplay.TableColumn, len(columns))
	for i, c := range columns {
		innerColumns[i] = gometricsDisplay.TableColumn{Name: c.Name}
	}

	innerRows := make([]gometricsDisplay.NodeTableRow, len(rows))
	for i, r := range rows {
		innerRows[i] = gometricsDisplay.NodeTableRow{
			NodeName: r.NodeName,
			Values:   r.Values,
		}
	}

	return o.inner.RenderTableHorizontal(innerColumns, innerRows)
}

func (o *OwlRenderer) RenderFooter(results []Result) string {
	innerResults := make([]gometricsDisplay.Result, len(results))
	for i, r := range results {
		innerResults[i] = gometricsDisplay.Result{
			Name:    r.Name,
			Success: r.Success,
			Latency: r.Latency,
		}
	}
	return o.inner.RenderFooter(innerResults)
}

func (o *OwlRenderer) ClearScreen() string {
	return gometricsDisplay.ClearScreen()
}

func (o *OwlRenderer) HideCursor() string {
	return gometricsDisplay.HideCursor()
}

func (o *OwlRenderer) ShowCursor() string {
	return gometricsDisplay.ShowCursor()
}

// DefaultConfig 返回默认配置
func DefaultConfig() *MetricsConfig {
	innerCfg := gometricsConfig.DefaultConfig()
	cfg := &MetricsConfig{
		ScrapeTimeout:  innerCfg.ScrapeTimeout,
		ScrapeInterval: innerCfg.ScrapeInterval,
		MaxDiskColumns: innerCfg.MaxDiskColumns,
	}
	cfg.Endpoints = make([]EndpointConfig, len(innerCfg.Endpoints))
	for i, ep := range innerCfg.Endpoints {
		cfg.Endpoints[i] = EndpointConfig{
			Name:    ep.Name,
			Address: ep.Address,
			NodeRef: ep.NodeRef,
			Labels:  ep.Labels,
		}
	}
	return cfg
}

// LoadFile 从文件加载配置
func LoadFile(path string) (*MetricsConfig, error) {
	innerCfg, err := gometricsConfig.LoadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &MetricsConfig{
		ScrapeTimeout:  innerCfg.ScrapeTimeout,
		ScrapeInterval: innerCfg.ScrapeInterval,
		MaxDiskColumns: innerCfg.MaxDiskColumns,
	}
	cfg.Endpoints = make([]EndpointConfig, len(innerCfg.Endpoints))
	for i, ep := range innerCfg.Endpoints {
		cfg.Endpoints[i] = EndpointConfig{
			Name:    ep.Name,
			Address: ep.Address,
			NodeRef: ep.NodeRef,
			Labels:  ep.Labels,
		}
	}
	return cfg, nil
}
