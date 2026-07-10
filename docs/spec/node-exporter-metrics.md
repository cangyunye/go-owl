# node_exporter 监控数据采集功能规格

## 一、Why

go-owl 作为单机运维工具，需要从配置的 node_exporter 端点采集系统监控数据，提供类似 Linux `top` 命令的动态刷新展示效果。

**设计原则**：
- 作为插件工具集成到 owl（类似 `owl tui`）
- 端点配置通过 YAML 配置文件管理（类似 Prometheus）
- `--endpoint` 参数仅作为快速测试的备选方案

## 二、What Changes

### 2.1 命令定位

```
# 作为插件工具，类似 owl tui
owl metrics watch      # 从配置文件读取端点，进入监控
owl metrics --help     # 查看帮助
```

### 2.2 新增文件

```
go-owl/
├── docs/
│   └── spec/
│       └── node-exporter-metrics.md    # 本文档
│
├── cmd/cli/cmd/
│   └── metrics/                        # 插件命令目录
│       ├── metrics.go
│       ├── watch.go
│       ├── query.go
│       └── list.go
│
├── internal/
│   └── metrics/                        # 核心模块
│       ├── types.go
│       ├── scraper.go
│       ├── parser.go
│       └── aggregator.go
│
├── config/
│   └── metrics.yaml                    # 配置文件
```

## 三、端点配置机制

### 3.1 配置文件格式（metrics.yaml）

```yaml
# 监控端点配置
endpoints:
  - name: "web-server-01"           # 显示名称
    address: "192.168.1.10:9100"     # node_exporter 地址
    labels:                          # 标签（可选）
      env: production
      role: web

  - name: "db-server-01"
    address: "192.168.1.20:9100"
    labels:
      env: production
      role: database

  - name: "cache-server-01"
    address: "192.168.1.30:9100"
    labels:
      env: production
      role: cache

# 默认采集间隔（秒）
scrape_interval: 3

# 超时设置（秒）
scrape_timeout: 5

# 并发采集数量
concurrency: 10
```

### 3.2 配置加载逻辑

```go
type MetricsConfig struct {
    Endpoints      []Endpoint `yaml:"endpoints"`
    ScrapeInterval int         `yaml:"scrape_interval"`
    ScrapeTimeout  int         `yaml:"scrape_timeout"`
    Concurrency    int         `yaml:"concurrency"`
}

type Endpoint struct {
    Name    string            `yaml:"name"`
    Address string            `yaml:"address"`
    Labels  map[string]string `yaml:"labels"`
}

func LoadConfig(path string) (*MetricsConfig, error) {
    // 1. 优先从用户配置目录加载
    //    ~/.owl/metrics.yaml

    // 2. 回退到项目默认配置
    //    ./config/metrics.yaml

    // 3. 使用内置最小配置
}
```

### 3.3 配置优先级

```
命令行参数 --endpoint > 用户配置 ~/.owl/metrics.yaml > 默认配置
```

```bash
# 方式1: 使用配置文件（推荐）
owl metrics watch

# 方式2: 命令行指定端点（快速测试）
owl metrics watch --endpoint 192.168.1.10:9100
owl metrics watch --endpoint node1:9100,node2:9100

# 方式3: 配置文件 + 命令行补充
owl metrics watch --add-endpoint 192.168.1.50:9100
```

## 四、多节点数据采集机制

### 4.1 采集流程

```
┌─────────────────────────────────────────────────────────────────┐
│                     多节点并发采集流程                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   1. 加载配置: endpoints = [node1, node2, node3]              │
│           │                                                     │
│           ▼                                                     │
│   ┌─────────────────────────────────────────────────────────┐ │
│   │           Worker Pool (concurrency=10)                   │ │
│   │                                                          │ │
│   │   ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐       │ │
│   │   │Worker 1│  │Worker 2│  │Worker 3│  │ ...    │       │ │
│   │   └────┬───┘  └────┬───┘  └────┬───┘  └────────┘       │ │
│   │        │           │           │                        │ │
│   │        ▼           ▼           ▼                        │ │
│   │   http://    http://    http://                          │ │
│   │   node1:9100 node2:9100 node3:9100                       │ │
│   └─────────────────────────────────────────────────────────┘ │
│            │              │              │                        │
│            ▼              ▼              ▼                        │
│   ┌─────────────────────────────────────────────────────────┐ │
│   │              结果聚合 (按指标名称 + 节点分组)               │ │
│   │                                                          │ │
│   │   node_cpu ──▶ {node1: 0.85, node2: 0.92, node3: 0.78} │ │
│   │   node_mem ──▶ {node1: 12GB, node2: 28GB, node3: 8GB}  │ │
│   └─────────────────────────────────────────────────────────┘ │
│            │                                                     │
│            ▼                                                     │
│   ┌─────────────────────────────────────────────────────────┐ │
│   │                   表格渲染输出                            │ │
│   └─────────────────────────────────────────────────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 并发控制实现

```go
type Collector struct {
    client     *http.Client
    poolSize   int           // 并发数
    timeout    time.Duration // 单节点超时
}

func (c *Collector) ScrapeAll(endpoints []Endpoint) []*ScrapeResult {
    results := make([]*ScrapeResult, len(endpoints))
    semaphore := make(chan struct{}, c.poolSize)
    var wg sync.WaitGroup

    for i, ep := range endpoints {
        wg.Add(1)
        go func(idx int, endpoint Endpoint) {
            defer wg.Done()
            semaphore <- struct{}{}        // 获取令牌
            defer func() { <-semaphore }() // 归还令牌

            results[idx] = c.scrapeOne(endpoint)
        }(i, ep)
    }

    wg.Wait()
    return results
}
```

### 4.3 容错处理

```go
type ScrapeResult struct {
    Endpoint   string
    Success    bool
    Error      error
    Latency    time.Duration
    Metrics    []*Metric
}

func (c *Collector) scrapeOne(endpoint Endpoint) *ScrapeResult {
    start := time.Now()
    defer func() {
        if r := recover(); r != nil {
            log.Printf("panic scraping %s: %v", endpoint.Address, r)
        }
    }()

    url := fmt.Sprintf("http://%s/metrics", endpoint.Address)
    resp, err := c.client.Get(url)

    if err != nil {
        return &ScrapeResult{
            Endpoint: endpoint.Name,
            Success:  false,
            Error:    err,
            Latency:  time.Since(start),
        }
    }
    defer resp.Body.Close()

    metrics, _ := parseMetrics(resp.Body)
    return &ScrapeResult{
        Endpoint: endpoint.Name,
        Success:  true,
        Latency:  time.Since(start),
        Metrics:  metrics,
    }
}
```

## 五、动态刷新机制（类 top 命令）

### 5.1 主循环

```go
func runWatch() {
    config := loadConfig()
    collector := NewCollector(config)
    terminal := newTerminal()

    ticker := time.NewTicker(time.Duration(config.ScrapeInterval) * time.Second)
    defer ticker.Stop()

    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    for {
        select {
        case <-sigChan:
            terminal.Restore()  // 恢复终端
            return

        case <-ticker.C:
            // 1. 清屏
            terminal.Clear()

            // 2. 绘制顶部信息栏
            terminal.DrawHeader(time.Now(), config.ScrapeInterval)

            // 3. 并发采集
            results := collector.ScrapeAll(config.Endpoints)

            // 4. 聚合数据
            aggregated := AggregateResults(results)

            // 5. 渲染表格
            terminal.DrawTable(aggregated)

            // 6. 渲染底部状态栏
            terminal.DrawFooter(results)
        }
    }
}
```

### 5.2 终端控制

```go
type Terminal struct {
    stdout   io.Writer
    mu       sync.Mutex
}

var (
    // ANSI 转义序列
    ClearScreen     = "\033[2J"
    CursorHome      = "\033[H"
    HideCursor      = "\033[?25l"
    ShowCursor      = "\033[?25h"
    SaveCursor      = "\033[s"
    RestoreCursor   = "\033[u"
)

func (t *Terminal) Clear() {
    fmt.Print(ClearScreen)
    fmt.Print(CursorHome)
}

func (t *Terminal) DrawHeader(t time.Time, interval int) {
    t.mu.Lock()
    defer t.mu.Unlock()

    fmt.Printf(" owl metrics watch  -  实时监控  [Ctrl+C 退出]\n")
    fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
    fmt.Printf(" 刷新时间: %s  |  间隔: %ds  |  节点: %d 个\n",
        t.Format("15:04:05"), interval, totalEndpoints)
    fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
}

func (t *Terminal) DrawTable(data *AggregatedData) {
    t.mu.Lock()
    defer t.mu.Unlock()

    // 绘制表头
    fmt.Printf(" %-20s", "指标")
    for _, name := range data.NodeNames {
        fmt.Printf(" %12s", name)
    }
    fmt.Println()
    fmt.Println(strings.Repeat("─", 20+12*len(data.NodeNames)))

    // 绘制数据行
    for _, metric := range data.Metrics {
        fmt.Printf(" %-20s", metric.Name)
        for _, value := range metric.Values {
            fmt.Printf(" %12s", formatValue(value))
        }
        fmt.Println()
    }
}

func (t *Terminal) DrawFooter(results []*ScrapeResult) {
    fmt.Println()
    fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

    for _, r := range results {
        status := "● 在线"
        if !r.Success {
            status = "○ 离线"
        }
        latency := ""
        if r.Latency > 0 {
            latency = fmt.Sprintf(" (%s)", r.Latency.String())
        }
        fmt.Printf(" %s: %s%s   ", r.Endpoint, status, latency)
    }
    fmt.Println()
}
```

### 5.3 屏幕输出示例

```
 owl metrics watch  -  实时监控  [Ctrl+C 退出]
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 刷新时间: 14:30:25  |  间隔: 3s  |  节点: 3 个
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

 指标                    node1         node2         node3
──────────────────────────────────────────────────────────────
 CPU 使用率              85.2%         92.1%         78.5%
 内存使用率              75.0%         88.5%         62.3%
 系统负载 (1min)         1.25          2.30          0.85
 磁盘使用率              45.2%         67.8%         23.1%
 网络接收              1.2MB/s       5.6MB/s       0.8MB/s
 网络发送              0.5MB/s       3.2MB/s       0.3MB/s
 进程数                    145           289            87

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 ● node1: 在线 (2.3ms)   ● node2: 在线 (1.8ms)   ● node3: 在线 (1.1ms)
```

## 六、命令使用

### 6.1 命令结构

```bash
# 插件入口
owl metrics [command]

# 子命令
owl metrics watch      # 从配置读取端点，实时监控
owl metrics query      # 查询指定指标
owl metrics list       # 列出可用指标
owl metrics status     # 查看端点状态

# 全局选项
--config string        # 指定配置文件 (默认 ~/.owl/metrics.yaml)
--endpoint strings     # 端点地址 (覆盖配置)
--help, -h            # 帮助
```

### 6.2 使用示例

```bash
# 1. 使用配置文件监控（推荐）
owl metrics watch

# 2. 快速测试指定端点
owl metrics watch --endpoint 192.168.1.10:9100
owl metrics watch --endpoint node1:9100,node2:9100

# 3. 查询指标
owl metrics query 'node_cpu_seconds_total{mode="idle"}'
owl metrics query node_load1

# 4. 列出可用指标
owl metrics list --endpoint 192.168.1.10:9100

# 5. 查看端点状态
owl metrics status
```

## 七、Impact

### 7.1 受影响代码
- `cmd/cli/cmd/root.go` - 注册 `metrics` 插件命令
- 新增 `cmd/cli/cmd/metrics/` - 命令实现
- 新增 `internal/metrics/` - 核心逻辑
- 新增 `config/metrics.yaml` - 默认配置

### 7.2 无外部依赖
- 仅使用 Go 标准库
- 不引入第三方包

## 八、ADDED Requirements

### Requirement: 配置驱动的端点管理

系统 SHALL 通过 YAML 配置文件管理监控端点，支持多节点配置。

#### Scenario: 使用配置文件监控
- **WHEN** 用户执行 `owl metrics watch` 且配置文件存在
- **THEN** 系统从 `~/.owl/metrics.yaml` 加载端点并开始监控

#### Scenario: 命令行覆盖配置
- **WHEN** 用户同时指定 `--endpoint` 参数
- **THEN** 命令行参数优先于配置文件

### Requirement: 动态刷新展示

系统 SHALL 提供类似 `top` 命令的实时刷新效果，每 N 秒更新数据。

#### Scenario: 实时监控
- **WHEN** 用户执行 `owl metrics watch`
- **THEN** 屏幕每 3 秒自动刷新，显示最新数据

### Requirement: 多节点并发采集

系统 SHALL 并发采集多个端点，使用 worker pool 控制并发数量。

#### Scenario: 10 节点并发采集
- **WHEN** 配置 10 个端点，并发数设为 5
- **THEN** 系统分批并发采集，每批 5 个节点

### Requirement: 容错与降级

系统 SHALL 容忍部分节点离线，不影响其他节点展示。

#### Scenario: 部分节点离线
- **WHEN** node2 不可达
- **THEN** node1 和 node3 正常显示，node2 标记为 "离线"
