// Package metrics 提供 node_exporter 监控功能
package metrics

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/metrics"
)

var (
	metricsCfgFile    string
	metricsEndpoint   string
	metricsAddEndpoint string
)

func NewMetricsCmd() *cobra.Command {
	metricsCmd := &cobra.Command{
		Use:   "metrics",
		Short: "node_exporter 监控",
		Long:  `从 node_exporter 端点采集系统监控数据，提供类似 top 的动态刷新效果。`,
	}

	watchCmd := &cobra.Command{
		Use:   "watch",
		Short: "实时监控 node_exporter 端点",
		Long: `实时采集 node_exporter 端点数据并以仪表盘展示。

示例:
  owl metrics watch
  owl metrics watch --endpoint 192.168.1.10:9100`,
		Run: runWatch,
	}

	watchCmd.Flags().StringVar(&metricsCfgFile, "config", "",
		"配置文件路径 (默认 ~/.owl/metrics.yaml)")
	watchCmd.Flags().StringVar(&metricsEndpoint, "endpoint", "",
		"端点地址，逗号分隔 (覆盖配置文件)")
	watchCmd.Flags().StringVar(&metricsAddEndpoint, "add-endpoint", "",
		"补充端点地址 (与配置文件合并)")

	metricsCmd.AddCommand(watchCmd)
	return metricsCmd
}

func runWatch(cmd *cobra.Command, args []string) {
	cfg := loadMetricsConfig()
	if len(cfg.Endpoints) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未配置监控端点")
		os.Exit(1)
	}

	if !metrics.IsMetricsEnabled() {
		fmt.Fprintln(os.Stderr, "错误: metrics 功能未启用，请使用 -tags metrics 编译")
		os.Exit(1)
	}

	timeout := time.Duration(cfg.ScrapeTimeout) * time.Second
	scraper := metrics.NewOwlScraper(int64(timeout))
	analyzer := metrics.NewOwlAnalyzer()
	renderer := metrics.NewOwlRenderer()
	ctx := context.Background()

	scrapeInterval := time.Duration(cfg.ScrapeInterval) * time.Second

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(scrapeInterval)
	defer ticker.Stop()

	doScrape(ctx, scraper, analyzer, renderer, cfg.Endpoints, cfg.ScrapeInterval, len(cfg.Endpoints), cfg.MaxDiskColumns)

	for {
		select {
		case <-sigChan:
			fmt.Print(renderer.ShowCursor())
			fmt.Println()
			fmt.Println("监控已退出")
			return
		case <-ticker.C:
			doScrape(ctx, scraper, analyzer, renderer, cfg.Endpoints, cfg.ScrapeInterval, len(cfg.Endpoints), cfg.MaxDiskColumns)
		}
	}
}

func doScrape(
	ctx context.Context,
	scraper metrics.Scraper,
	analyzer metrics.Analyzer,
	renderer metrics.Renderer,
	epList []metrics.EndpointConfig,
	scrapeInterval int,
	nodeCount int,
	maxDiskColumns int,
) {
	now := time.Now()
	results := scraper.ScrapeAll(ctx, epList, len(epList))

	type nodeResult struct {
		endpoint string
		success  bool
		latency  float64
		alerts   []metrics.Alert
	}

	var nrList []nodeResult
	var allFamilies []struct {
		endpoint string
		families []metrics.MetricFamily
	}

	parser := metrics.NewOwlParser()

	for _, r := range results {
		nr := nodeResult{endpoint: r.Endpoint, success: r.Success, latency: r.Latency}
		if r.Success && len(r.Raw) > 0 {
			families, err := parser.Parse(r.Raw)
			if err == nil {
				nr.alerts = analyzer.Analyze(r.Endpoint, families)
				allFamilies = append(allFamilies, struct {
					endpoint string
					families []metrics.MetricFamily
				}{r.Endpoint, families})
			}
		}
		nrList = append(nrList, nr)
	}

	fmt.Print(renderer.ClearScreen())
	fmt.Print(renderer.HideCursor())
	fmt.Print(renderer.RenderHeader(now.UnixNano(), scrapeInterval, nodeCount))

	columns, rows := buildTable(allFamilies, maxDiskColumns)
	if len(columns) > 0 && len(rows) > 0 {
		fmt.Print(renderer.RenderTableHorizontal(columns, rows))
	} else {
		fmt.Println("  (等待数据采集...)")
	}

	var allAlerts []metrics.Alert
	for _, nr := range nrList {
		allAlerts = append(allAlerts, nr.alerts...)
	}
	if len(allAlerts) > 0 {
		fmt.Println("\n⚠️  告警:")
		for _, a := range allAlerts {
			icon := "⚠"
			if a.Severity == metrics.AlertCritical {
				icon = "🔴"
			}
			fmt.Printf("  %s [%s] %s\n", icon, a.Endpoint, a.Message)
		}
	}

	var dispResults []metrics.Result
	for _, nr := range nrList {
		dispResults = append(dispResults, metrics.Result{
			Name: nr.endpoint, Success: nr.success,
			Latency: int64(nr.latency * 1e9),
		})
	}
	fmt.Print(renderer.RenderFooter(dispResults))
}

// buildTable 构建横向表头表格
func buildTable(nodeFamilies []struct {
	endpoint string
	families []metrics.MetricFamily
}, maxDiskColumns int,
) ([]metrics.TableColumn, []metrics.NodeTableRow) {
	metricMap := make(map[string]*buildCell)

	for _, nf := range nodeFamilies {
		for _, ex := range extractors {
			values := ex(nf.families)
			for _, v := range values {
				key := v.name
				if v.sub != "" {
					key = v.name + ":" + v.sub
				}
				rowName := v.name
				if v.sub != "" {
					rowName = v.name + " " + v.sub
				}
				if _, ok := metricMap[key]; !ok {
					metricMap[key] = &buildCell{displayName: rowName, nodeValues: make(map[string]string)}
				}
				metricMap[key].nodeValues[nf.endpoint] = v.val
			}
		}
	}
	if len(metricMap) == 0 {
		return nil, nil
	}

	var sorted []buildEntry
	for k, c := range metricMap {
		sorted = append(sorted, buildEntry{key: k, name: c.displayName})
	}

	if maxDiskColumns > 0 {
		sorted = filterDiskCols(sorted, metricMap, maxDiskColumns)
	}

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if pri(sorted[i].name) > pri(sorted[j].name) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var cols []metrics.TableColumn
	for _, e := range sorted {
		cols = append(cols, metrics.TableColumn{Name: e.name})
	}

	var nodeOrder []string
	seen := map[string]bool{}
	for _, nf := range nodeFamilies {
		if !seen[nf.endpoint] {
			seen[nf.endpoint] = true
			nodeOrder = append(nodeOrder, nf.endpoint)
		}
	}

	var rows []metrics.NodeTableRow
	for _, n := range nodeOrder {
		row := metrics.NodeTableRow{NodeName: n, Values: make([]string, len(cols))}
		for i := range row.Values {
			row.Values[i] = "-"
		}
		for k, c := range metricMap {
			for idx, e := range sorted {
				if e.key == k {
					if v, ok := c.nodeValues[n]; ok {
						row.Values[idx] = v
					}
					break
				}
			}
		}
		rows = append(rows, row)
	}
	return cols, rows
}

// filterDiskCols 限制磁盘使用率列数为最高 N 个
func filterDiskCols(cols []buildEntry, metricMap map[string]*buildCell, maxN int) []buildEntry {
	type diskInfo struct {
		key    string
		name   string
		maxVal float64
	}
	var disks []diskInfo
	var others []buildEntry

	for _, e := range cols {
		if !strings.HasPrefix(e.name, "磁盘使用率") {
			others = append(others, e)
			continue
		}
		var maxV float64
		if c, ok := metricMap[e.key]; ok {
			for _, v := range c.nodeValues {
				pct := parsePct(v)
				if pct > maxV {
					maxV = pct
				}
			}
		}
		disks = append(disks, diskInfo{key: e.key, name: e.name, maxVal: maxV})
	}

	sort.SliceStable(disks, func(i, j int) bool { return disks[i].maxVal > disks[j].maxVal })
	if len(disks) > maxN {
		disks = disks[:maxN]
	}

	result := others
	for _, d := range disks {
		result = append(result, buildEntry{key: d.key, name: d.name})
	}
	return result
}

func parsePct(s string) float64 {
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// buildTable 内部使用的类型
type buildEntry struct {
	key  string
	name string
}

type buildCell struct {
	displayName string
	nodeValues  map[string]string
}

// --- 指标提取 ---

type ev struct {
	name string
	sub  string
	val  string
}

type extractFn func([]metrics.MetricFamily) []ev

var extractors = []extractFn{
	extractCPU, extractMem, extractL1, extractL5, extractL15,
	extractDisk, extractNetRX, extractNetTX, extractDiskR, extractDiskW,
}

func extractCPU(families []metrics.MetricFamily) []ev {
	f := findFamily(families, "node_cpu_seconds_total")
	if f == nil {
		return nil
	}
	var idle, total float64
	for _, m := range f.Metrics {
		total += m.Value
		if m.Labels["mode"] == "idle" {
			idle += m.Value
		}
	}
	if total == 0 {
		return nil
	}
	return []ev{{name: "CPU使用率", val: fmt.Sprintf("%.1f%%", (1-idle/total)*100)}}
}

func extractMem(families []metrics.MetricFamily) []ev {
	if total, avail := findVal(families, "node_memory_MemTotal_bytes"), findVal(families, "node_memory_MemAvailable_bytes"); total > 0 && avail > 0 {
		return []ev{{name: "内存使用率", val: fmt.Sprintf("%.1f%%", (1-avail/total)*100)}}
	}
	if t := findVal(families, "node_memory_total_bytes"); t > 0 {
		free := findVal(families, "node_memory_free_bytes")
		inact := findVal(families, "node_memory_inactive_bytes")
		if free+inact > 0 {
			return []ev{{name: "内存使用率", val: fmt.Sprintf("%.1f%%", (1-(free+inact)/t)*100)}}
		}
	}
	return nil
}

func extractL1(families []metrics.MetricFamily) []ev {
	if v := findVal(families, "node_load1"); v >= 0 {
		return []ev{{name: "系统负载(1m)", val: fmt.Sprintf("%.2f", v)}}
	}
	return nil
}

func extractL5(families []metrics.MetricFamily) []ev {
	if v := findVal(families, "node_load5"); v >= 0 {
		return []ev{{name: "系统负载(5m)", val: fmt.Sprintf("%.2f", v)}}
	}
	return nil
}

func extractL15(families []metrics.MetricFamily) []ev {
	if v := findVal(families, "node_load15"); v >= 0 {
		return []ev{{name: "系统负载(15m)", val: fmt.Sprintf("%.2f", v)}}
	}
	return nil
}

func extractDisk(families []metrics.MetricFamily) []ev {
	availF := findFamily(families, "node_filesystem_avail_bytes")
	sizeF := findFamily(families, "node_filesystem_size_bytes")
	if availF == nil || sizeF == nil {
		return nil
	}
	am := map[string]float64{}
	for _, m := range availF.Metrics {
		if d, ok := m.Labels["device"]; ok {
			am[d] = m.Value
		}
	}
	var res []ev
	for _, m := range sizeF.Metrics {
		d, fs, mp := m.Labels["device"], m.Labels["fstype"], m.Labels["mountpoint"]
		if isVS(fs) || m.Value <= 0 || isISV(mp) {
			continue
		}
		a, ok := am[d]
		if !ok {
			continue
		}
		l := strings.TrimPrefix(d, "/dev/")
		if mp != "" && mp != "/" {
			l = mp
		}
		res = append(res, ev{name: "磁盘使用率", sub: l, val: fmt.Sprintf("%.1f%%", (1-a/m.Value)*100)})
	}
	return res
}

func extractNetRX(families []metrics.MetricFamily) []ev { return extNet(families, "node_network_receive_bytes_total", "网络接收") }
func extractNetTX(families []metrics.MetricFamily) []ev { return extNet(families, "node_network_transmit_bytes_total", "网络发送") }
func extNet(families []metrics.MetricFamily, name, disp string) []ev {
	f := findFamily(families, name)
	if f == nil {
		return nil
	}
	var res []ev
	for _, m := range f.Metrics {
		d := m.Labels["device"]
		if d == "lo" || d == "lo0" || m.Value == 0 {
			continue
		}
		res = append(res, ev{name: disp, sub: d, val: fmtB(m.Value)})
	}
	return res
}

func extractDiskR(families []metrics.MetricFamily) []ev { return extDiskM(families, "node_disk_read_bytes_total", "磁盘读取") }
func extractDiskW(families []metrics.MetricFamily) []ev { return extDiskM(families, "node_disk_written_bytes_total", "磁盘写入") }
func extDiskM(families []metrics.MetricFamily, name, disp string) []ev {
	f := findFamily(families, name)
	if f == nil {
		return nil
	}
	var res []ev
	for _, m := range f.Metrics {
		if m.Value == 0 {
			continue
		}
		res = append(res, ev{name: disp, sub: m.Labels["device"], val: fmtB(m.Value)})
	}
	return res
}

func findFamily(families []metrics.MetricFamily, name string) *metrics.MetricFamily {
	for i := range families {
		if families[i].Name == name {
			return &families[i]
		}
	}
	return nil
}

func findVal(families []metrics.MetricFamily, name string) float64 {
	f := findFamily(families, name)
	if f != nil && len(f.Metrics) > 0 {
		return f.Metrics[0].Value
	}
	return -1
}

func fmtB(b float64) string {
	u := []string{"B", "KB", "MB", "GB", "TB"}
	if b <= 0 {
		return "0B"
	}
	i, v := 0, b
	for v >= 1024 && i < len(u)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f%s", v, u[i])
	}
	return fmt.Sprintf("%.1f%s", v, u[i])
}

func pri(name string) int {
	switch {
	case strings.HasPrefix(name, "CPU"):
		return 0
	case strings.HasPrefix(name, "内存"):
		return 1
	case strings.HasPrefix(name, "系统负载"):
		return 2
	case strings.HasPrefix(name, "磁盘使用率"):
		return 3
	case strings.HasPrefix(name, "磁盘读取"):
		return 4
	case strings.HasPrefix(name, "磁盘写入"):
		return 5
	case strings.HasPrefix(name, "网络接收"):
		return 6
	case strings.HasPrefix(name, "网络发送"):
		return 7
	default:
		return 99
	}
}

func isVS(fs string) bool {
	return map[string]bool{
		"tmpfs": true, "devtmpfs": true, "squashfs": true, "overlay": true,
		"proc": true, "sysfs": true, "cgroup": true, "cgroup2": true,
		"devpts": true, "shm": true, "hugetlbfs": true, "mqueue": true,
		"pstore": true, "efivarfs": true, "tracefs": true, "securityfs": true,
		"debugfs": true, "bpf": true, "configfs": true, "autofs": true,
	}[fs]
}

func isISV(mp string) bool {
	if mp == "" || mp == "/" || mp == "/System/Volumes/Data" {
		return false
	}
	if strings.HasPrefix(mp, "/Volumes/") {
		return false
	}
	return strings.HasPrefix(mp, "/System/Volumes/")
}

func loadMetricsConfig() *metrics.MetricsConfig {
	if metricsEndpoint != "" {
		cfg := metrics.DefaultConfig()
		for _, addr := range strings.Split(metricsEndpoint, ",") {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				cfg.Endpoints = append(cfg.Endpoints, metrics.EndpointConfig{
					Name: addr, Address: addr,
				})
			}
		}
		return cfg
	}

	configPath := metricsCfgFile
	if configPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(home, ".owl", "metrics.yaml")
		}
	}
	cfg, err := metrics.LoadFile(configPath)
	if err != nil {
		cfg = metrics.DefaultConfig()
	}

	if metricsAddEndpoint != "" {
		for _, addr := range strings.Split(metricsAddEndpoint, ",") {
			addr = strings.TrimSpace(addr)
			if addr != "" {
				cfg.Endpoints = append(cfg.Endpoints, metrics.EndpointConfig{
					Name: addr, Address: addr,
				})
			}
		}
	}
	return cfg
}
