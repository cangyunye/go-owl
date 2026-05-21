# AI 配置方案设计文档

## 1. 背景

go-owl 项目需要支持 AI 助手功能，需要从配置文件读取 AI Provider（提供商）和 Model（模型）信息，以便用户灵活配置不同的 AI 服务。

---

## 2. 现有配置结构分析

### 2.1 当前配置文件位置

- **配置文件路径**: `~/.owl/config.yaml`
- **配置文件格式**: YAML

### 2.2 当前 AI 配置结构

```yaml
# ~/.owl/config.yaml
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}
  base_url: https://api.openai.com/v1
  timeout: 120
```

### 2.3 当前支持的 Provider

| Provider | Base URL | 默认模型 | 说明 |
|----------|----------|----------|------|
| openai | https://api.openai.com/v1 | gpt-4o | OpenAI GPT 系列 |
| anthropic | https://api.anthropic.com | claude-3.5-sonnet | Anthropic Claude 系列 |
| dashscope | https://dashscope.aliyuncs.com | qwen-turbo | 阿里云通义千问 |

---

## 3. 配置文件设计方案

### 3.1 推荐配置文件格式

```yaml
# ~/.owl/config.yaml

ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY:-sk-xxx}
  base_url: https://api.openai.com/v1
  timeout: 120

  # Provider-specific settings
  settings:
    temperature: 0.7
    max_tokens: 4096
    top_p: 1.0
    frequency_penalty: 0.0
    presence_penalty: 0.0

  # Model presets (快速切换预设)
  presets:
    default: gpt-4o
    fast: gpt-4o-mini
    creative: gpt-4
    code: gpt-4-turbo
```

### 3.2 Provider 配置详情

#### 3.2.1 OpenAI

```yaml
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}
  base_url: https://api.openai.com/v1
  settings:
    temperature: 0.7
    max_tokens: 4096
```

**支持的模型列表**:
| 模型 | 说明 | 上下文长度 |
|------|------|-----------|
| gpt-4o | 最新全能模型 | 128K |
| gpt-4o-mini | 快速经济型 | 128K |
| gpt-4-turbo | GPT-4 优化版 | 128K |
| gpt-4 | 标准 GPT-4 | 8K/32K |
| gpt-3.5-turbo | 经济型 | 16K |

#### 3.2.2 Anthropic (Claude)

```yaml
ai:
  provider: anthropic
  model: claude-3.5-sonnet
  api_key: ${ANTHROPIC_API_KEY}
  base_url: https://api.anthropic.com
  settings:
    temperature: 0.7
    max_tokens: 4096
```

**支持的模型列表**:
| 模型 | 说明 | 上下文长度 |
|------|------|-----------|
| claude-3.5-sonnet | 最新高性能 | 200K |
| claude-3-opus | 最强能力 | 200K |
| claude-3-sonnet | 平衡型 | 200K |
| claude-3-haiku | 快速经济型 | 200K |

#### 3.2.3 DashScope (通义千问)

```yaml
ai:
  provider: dashscope
  model: qwen-turbo
  api_key: ${DASHSCOPE_API_KEY}
  base_url: https://dashscope.aliyuncs.com/api/v1
  settings:
    temperature: 0.7
    max_tokens: 4096
```

**支持的模型列表**:
| 模型 | 说明 | 上下文长度 |
|------|------|-----------|
| qwen-plus | 增强版 | 131K |
| qwen-turbo | 快速版 | 130K |
| qwen-max | 最强版 | 30K |
| qwen-long | 长文本版 | 10M |

---

## 4. 代码实现设计

### 4.1 配置结构体更新

```go
// internal/ai/config.go

package ai

// AIConfig AI 配置
type AIConfig struct {
    // 基础配置
    Provider string            `yaml:"provider"` // openai, anthropic, dashscope
    Model    string           `yaml:"model"`    // 模型名称
    APIKey   string           `yaml:"api_key"`
    BaseURL  string           `yaml:"base_url"`
    Timeout  int              `yaml:"timeout"` // seconds

    // Provider-specific 设置
    Settings SettingsConfig    `yaml:"settings"`

    // 模型预设
    Presets  map[string]string `yaml:"presets"`
}

// SettingsConfig 模型参数设置
type SettingsConfig struct {
    Temperature     float64 `yaml:"temperature"`      // 0.0-2.0
    MaxTokens       int     `yaml:"max_tokens"`        // 最大生成 token 数
    TopP            float64 `yaml:"top_p"`            // 0.0-1.0
    FrequencyPenalty float64 `yaml:"frequency_penalty"` // -2.0-2.0
    PresencePenalty float64 `yaml:"presence_penalty"`  // -2.0-2.0
}

// DefaultSettings 获取默认设置
func DefaultSettings() SettingsConfig {
    return SettingsConfig{
        Temperature:     0.7,
        MaxTokens:       4096,
        TopP:            1.0,
        FrequencyPenalty: 0.0,
        PresencePenalty: 0.0,
    }
}
```

### 4.2 Provider 映射表

```go
// internal/ai/provider.go

package ai

// ProviderConfig Provider 配置
type ProviderConfig struct {
    Name        string
    BaseURL     string
    Models      []string
    DefaultModel string
}

// Built-in Providers
var Providers = map[string]ProviderConfig{
    "openai": {
        Name:        "OpenAI",
        BaseURL:     "https://api.openai.com/v1",
        DefaultModel: "gpt-4o",
        Models: []string{
            "gpt-4o",
            "gpt-4o-mini",
            "gpt-4-turbo",
            "gpt-4",
            "gpt-3.5-turbo",
        },
    },
    "anthropic": {
        Name:        "Anthropic",
        BaseURL:     "https://api.anthropic.com",
        DefaultModel: "claude-3.5-sonnet",
        Models: []string{
            "claude-3.5-sonnet",
            "claude-3-opus",
            "claude-3-sonnet",
            "claude-3-haiku",
        },
    },
    "dashscope": {
        Name:        "DashScope",
        BaseURL:     "https://dashscope.aliyuncs.com/api/v1",
        DefaultModel: "qwen-turbo",
        Models: []string{
            "qwen-plus",
            "qwen-turbo",
            "qwen-max",
            "qwen-long",
        },
    },
}

// GetProviderConfig 获取 Provider 配置
func GetProviderConfig(provider string) (ProviderConfig, bool) {
    cfg, ok := Providers[provider]
    return cfg, ok
}

// ValidateModel 验证模型是否支持
func ValidateModel(provider, model string) bool {
    cfg, ok := Providers[provider]
    if !ok {
        return false
    }
    for _, m := range cfg.Models {
        if m == model {
            return true
        }
    }
    return false
}
```

### 4.3 配置加载逻辑

```go
// internal/ai/config.go

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return DefaultConfig(), nil
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("配置解析失败: %w", err)
    }

    // 环境变量替换
    cfg.AI.APIKey = os.ExpandEnv(cfg.AI.APIKey)

    // 验证 Provider
    if _, ok := Providers[cfg.AI.Provider]; !ok {
        return nil, fmt.Errorf("不支持的 Provider: %s, 支持: %v",
            cfg.AI.Provider, getProviderNames())
    }

    // 验证 Model
    if !ValidateModel(cfg.AI.Provider, cfg.AI.Model) {
        return nil, fmt.Errorf("Provider %s 不支持模型: %s",
            cfg.AI.Provider, cfg.AI.Model)
    }

    // 应用默认设置
    if cfg.AI.Settings.Temperature == 0 {
        cfg.AI.Settings = DefaultSettings()
    }

    return &cfg, nil
}

func getProviderNames() []string {
    names := make([]string, 0, len(Providers))
    for name := range Providers {
        names = append(names, name)
    }
    return names
}
```

---

## 5. 使用方式

### 5.1 配置文件示例

创建 `~/.owl/config.yaml`:

```yaml
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}
  base_url: ""
  timeout: 120
  settings:
    temperature: 0.7
    max_tokens: 4096
  presets:
    default: gpt-4o
    fast: gpt-4o-mini
    code: gpt-4-turbo
```

### 5.2 命令行使用

```bash
# 使用默认配置
owl ai

# 切换 Provider
owl ai --provider anthropic --model claude-3.5-sonnet

# 覆盖模型参数
owl ai --temperature 0.5 --max-tokens 2048
```

### 5.3 环境变量

```bash
# 设置 API Key
export OWL_API_KEY="sk-xxx"
export ANTHROPIC_API_KEY="sk-ant-xxx"
export DASHSCOPE_API_KEY="sk-xxx"

# 设置 Base URL (代理)
export OWL_BASE_URL="https://api.openai.com/v1"
```

---

## 6. 配置验证与错误处理

### 6.1 启动时验证

```
$ owl ai
✓ 加载配置文件: ~/.owl/config.yaml
✓ Provider: openai
✓ Model: gpt-4o
✓ API Key: 已设置
✓ 配置验证通过
```

### 6.2 错误示例

```
$ owl ai
✗ 配置错误: 不支持的 Provider: xxx
  支持的 Provider: openai, anthropic, dashscope

$ owl ai
✗ 配置错误: Provider openai 不支持模型: gpt-5
  支持的模型: gpt-4o, gpt-4o-mini, gpt-4-turbo, gpt-4, gpt-3.5-turbo

$ owl ai
✗ 配置错误: API Key 未设置
  请设置环境变量 OWL_API_KEY 或在配置文件中添加
```

---

## 7. 向后兼容性

### 7.1 旧配置迁移

如果用户已有旧版配置文件，自动迁移:

```yaml
# 旧格式 (仍支持)
ai:
  provider: openai
  model: gpt-4o
  api_key: xxx

# 新格式 (推荐)
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}
  settings:
    temperature: 0.7
```

### 7.2 配置合并策略

1. 环境变量优先级最高
2. 命令行参数优先级次之
3. 配置文件优先级最低

---

## 8. 测试计划

| 测试用例 | 说明 | 预期结果 |
|---------|------|---------|
| TC-AI-CONFIG-001 | 加载有效配置文件 | 配置正确加载 |
| TC-AI-CONFIG-002 | 加载无效 Provider | 返回错误提示 |
| TC-AI-CONFIG-003 | 加载无效 Model | 返回错误提示 |
| TC-AI-CONFIG-004 | 环境变量覆盖 | 优先使用环境变量 |
| TC-AI-CONFIG-005 | 默认配置 | 使用内置默认值 |
| TC-AI-CONFIG-006 | 切换 Provider | 正确切换并验证 |
| TC-AI-CONFIG-007 | 使用预设 | 快速切换模型 |

---

## 9. 模型列表获取

### 9.1 设计原则

模型列表不采用硬编码方式，而是通过 Provider API 动态获取，确保：
- 始终获取最新的模型列表
- 支持 Provider 新增的模型
- 无需频繁更新代码

### 9.2 API 获取实现

```go
// internal/ai/models.go

package ai

import (
    "fmt"
    "net/http"
    "encoding/json"
    "time"
)

// ModelInfo 模型信息
type ModelInfo struct {
    ID       string `json:"id"`       // 模型 ID
    Name     string `json:"name"`     // 显示名称
    Context  int    `json:"context"`  // 上下文长度
    Created  int64  `json:"created"`  // 创建时间
    Enabled  bool   `json:"enabled"`  // 是否启用
}

// ModelCache 模型缓存
type ModelCache struct {
    Models     []ModelInfo
    LastUpdate time.Time
    TTL        time.Duration // 缓存有效期
}

var modelCache = &ModelCache{
    TTL: 24 * time.Hour, // 缓存 24 小时
}

// FetchModels 获取模型列表
func (c *Config) FetchModels() ([]ModelInfo, error) {
    // 检查缓存
    if time.Since(modelCache.LastUpdate) < modelCache.TTL && len(modelCache.Models) > 0 {
        return modelCache.Models, nil
    }

    var models []ModelInfo
    var err error

    switch c.AI.Provider {
    case "openai":
        models, err = c.fetchOpenAIModels()
    case "anthropic":
        models, err = c.fetchAnthropicModels()
    case "dashscope":
        models, err = c.fetchDashScopeModels()
    default:
        return nil, fmt.Errorf("不支持的 Provider: %s", c.AI.Provider)
    }

    if err != nil {
        return nil, err
    }

    // 更新缓存
    modelCache.Models = models
    modelCache.LastUpdate = time.Now()

    return models, nil
}

// fetchOpenAIModels 获取 OpenAI 模型列表
func (c *Config) fetchOpenAIModels() ([]ModelInfo, error) {
    url := "https://api.openai.com/v1/models"

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+c.AI.APIKey)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("请求失败: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("API 返回错误: %d", resp.StatusCode)
    }

    var result struct {
        Data []struct {
            ID        string `json:"id"`
            Created   int64  `json:"created"`
            ContextWindow int `json:"context_window,omitempty"`
        } `json:"data"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("解析失败: %w", err)
    }

    // 过滤支持的模型
    models := make([]ModelInfo, 0)
    for _, m := range result.Data {
        if isSupportedModel(m.ID) {
            models = append(models, ModelInfo{
                ID:      m.ID,
                Name:    getModelDisplayName(m.ID),
                Context:  getContextWindow(m.ID, m.ContextWindow),
                Created: m.Created,
                Enabled: true,
            })
        }
    }

    return models, nil
}

// fetchAnthropicModels 获取 Anthropic 模型列表
func (c *Config) fetchAnthropicModels() ([]ModelInfo, error) {
    // Anthropic API 不提供模型列表端点，使用预定义列表
    return []ModelInfo{
        {ID: "claude-3.5-sonnet", Name: "Claude 3.5 Sonnet", Context: 200000},
        {ID: "claude-3-opus", Name: "Claude 3 Opus", Context: 200000},
        {ID: "claude-3-sonnet", Name: "Claude 3 Sonnet", Context: 200000},
        {ID: "claude-3-haiku", Name: "Claude 3 Haiku", Context: 200000},
    }, nil
}

// fetchDashScopeModels 获取 DashScope 模型列表
func (c *Config) fetchDashScopeModels() ([]ModelInfo, error) {
    url := "https://dashscope.aliyuncs.com/api/v1/models"

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+c.AI.APIKey)
    req.Header.Set("Accept", "application/json")

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("请求失败: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("API 返回错误: %d", resp.StatusCode)
    }

    var result struct {
        Data []struct {
            ID       string `json:"id"`
            Name     string `json:"name"`
            Context  int    `json:"context_window"`
        } `json:"data"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("解析失败: %w", err)
    }

    models := make([]ModelInfo, 0, len(result.Data))
    for _, m := range result.Data {
        models = append(models, ModelInfo{
            ID:      m.ID,
            Name:    m.Name,
            Context: m.Context,
            Enabled: true,
        })
    }

    return models, nil
}

// isSupportedModel 检查是否为支持的模型
func isSupportedModel(modelID string) bool {
    supported := []string{
        "gpt-4o", "gpt-4o-mini", "gpt-4o-2024-05-13",
        "gpt-4-turbo", "gpt-4-turbo-2024-04-09",
        "gpt-4", "gpt-4-32k",
        "gpt-3.5-turbo", "gpt-3.5-turbo-16k",
    }
    for _, s := range supported {
        if modelID == s || contains(modelID, s) {
            return true
        }
    }
    return false
}

// getModelDisplayName 获取模型显示名称
func getModelDisplayName(modelID string) string {
    names := map[string]string{
        "gpt-4o":              "GPT-4o",
        "gpt-4o-mini":        "GPT-4o Mini",
        "gpt-4-turbo":        "GPT-4 Turbo",
        "gpt-4":              "GPT-4",
        "gpt-3.5-turbo":      "GPT-3.5 Turbo",
    }
    if name, ok := names[modelID]; ok {
        return name
    }
    return modelID
}

// getContextWindow 获取上下文窗口大小
func getContextWindow(modelID string, defaultValue int) int {
    contexts := map[string]int{
        "gpt-4o":           128000,
        "gpt-4o-mini":      128000,
        "gpt-4-turbo":      128000,
        "gpt-4":            8192,
        "gpt-4-32k":        32768,
        "gpt-3.5-turbo":    16385,
        "gpt-3.5-turbo-16k": 16385,
    }
    if ctx, ok := contexts[modelID]; ok {
        return ctx
    }
    return defaultValue
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
    for i := 0; i <= len(s)-len(substr); i++ {
        if s[i:i+len(substr)] == substr {
            return true
        }
    }
    return false
}
```

### 9.3 模型选择 UI

```go
// cmd/cli/cmd/ai/models.go

package ai

import (
    "fmt"
    "github.com/charmbracelet/bubbles/spinner"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

func (m *aiModel) fetchModels() tea.Cmd {
    return func() tea.Msg {
        models, err := m.config.FetchModels()
        if err != nil {
            return ModelsFetchErrorMsg{Err: err}
        }
        return ModelsFetchedMsg{Models: models}
    }
}

func (m *aiModel) showModelSelector() string {
    if len(m.models) == 0 {
        return "加载中..."
    }

    var sb strings.Builder
    sb.WriteString("选择模型:\n\n")

    for i, model := range m.models {
        prefix := "  "
        if model.ID == m.config.AI.Model {
            prefix = "● "
        }
        ctx := formatContext(model.Context)
        sb.WriteString(fmt.Sprintf("%s%s - %s (%s)\n", prefix, model.ID, model.Name, ctx))
    }

    return sb.String()
}

func formatContext(tokens int) string {
    if tokens >= 1000000 {
        return fmt.Sprintf("%.0fM", float64(tokens)/1000000)
    }
    if tokens >= 1000 {
        return fmt.Sprintf("%.0fK", float64(tokens)/1000)
    }
    return fmt.Sprintf("%d", tokens)
}
```

### 9.4 缓存策略

| 策略 | 说明 |
|------|------|
| 缓存时间 | 24 小时 |
| 缓存位置 | 内存 |
| 强制刷新 | `owl ai models --refresh` |
| 离线模式 | 使用最后缓存的列表 |

### 9.5 命令行接口

```bash
# 查看可用模型
owl ai models

# 强制刷新模型列表
owl ai models --refresh

# 查看特定 Provider 的模型
owl ai models --provider anthropic

# 输出格式
owl ai models --format json
```

### 9.6 错误处理

```
$ owl ai models
✗ 获取模型列表失败: API 请求失败
  请检查网络连接和 API Key

$ owl ai models --provider unknown
✗ 不支持的 Provider: unknown
  支持的 Provider: openai, anthropic, dashscope

$ owl ai models
✓ 从缓存加载 (24 小时前更新)
  如需刷新请使用 --refresh
```

---

## 10. 配置文件生成命令

### 10.1 设计目标

提供一个交互式命令，帮助用户快速生成和配置 `~/.owl/config.yaml` 文件。

### 10.2 命令设计

```bash
# 生成默认配置文件
owl ai config init

# 交互式配置
owl ai config

# 查看当前配置
owl ai config show
```

### 10.3 配置生成流程

**流程 1: 快速初始化 (owl ai config init)**

1. 检查配置文件是否已存在
2. 如果不存在，创建默认配置文件到 `~/.owl/config.yaml`
3. 提示用户设置 API Key

**流程 2: 交互式配置 (owl ai config)**

1. 检查现有配置
2. 依次询问用户配置项：
   - Provider 选择
   - Model 选择
   - API Key 输入
   - Base URL 配置 (可选)
3. 保存配置文件

### 10.4 代码实现设计

```go
// cmd/cli/cmd/ai/config.go

package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
	
	"github.com/cangyunye/go-owl/internal/ai"
)

func NewConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "AI 配置管理",
		Long: `管理 AI 配置文件。

示例：
  owl ai config       # 交互式配置
  owl ai config init  # 快速初始化
  owl ai config show  # 显示当前配置`,
	}
	
	configCmd.AddCommand(NewConfigInitCmd())
	configCmd.AddCommand(NewConfigShowCmd())
	
	return configCmd
}

func NewConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "初始化配置文件",
		Long:  `创建默认配置文件到 ~/.owl/config.yaml`,
		Run:   runConfigInit,
	}
}

func NewConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "显示当前配置",
		Long:  `显示当前的 AI 配置信息 (隐藏 API Key)`,
		Run:   runConfigShow,
	}
}

func runConfigInit(cmd *cobra.Command, args []string) {
	configPath := getConfigPath()
	
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		fmt.Printf("配置文件已存在: %s\n", configPath)
		fmt.Println("如需重新生成，请先删除该文件")
		return
	}
	
	if err := createConfigDir(); err != nil {
		fmt.Printf("创建配置目录失败: %v\n", err)
		os.Exit(1)
	}
	
	config := ai.DefaultConfig()
	
	data, err := yaml.Marshal(config)
	if err != nil {
		fmt.Printf("序列化配置失败: %v\n", err)
		os.Exit(1)
	}
	
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		fmt.Printf("写入配置文件失败: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("✓ 配置文件已创建: %s\n", configPath)
	fmt.Println()
	fmt.Println("下一步：")
	fmt.Println("  1. 编辑配置文件设置 API Key")
	fmt.Println("  2. 或使用 'owl ai config' 进行交互式配置")
}

func runConfigShow(cmd *cobra.Command, args []string) {
	configPath := getConfigPath()
	cfg, err := ai.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Println("当前配置:")
	fmt.Println()
	fmt.Printf("  Provider:    %s\n", cfg.AI.Provider)
	fmt.Printf("  Model:       %s\n", cfg.AI.Model)
	fmt.Printf("  API Key:     %s\n", maskAPIKey(cfg.AI.APIKey))
	fmt.Printf("  Base URL:    %s\n", cfg.AI.BaseURL)
	fmt.Printf("  Timeout:     %ds\n", cfg.AI.Timeout)
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".owl", "config.yaml")
}

func createConfigDir() error {
	configPath := getConfigPath()
	dir := filepath.Dir(configPath)
	return os.MkdirAll(dir, 0755)
}

func maskAPIKey(key string) string {
	if key == "" {
		return "(未设置)"
	}
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// 交互式配置模型 (简化示例)
type configWizardModel struct {
	step       int
	providers  list.Model
	models     list.Model
	apiKey     textinput.Model
	baseURL    textinput.Model
	quitting   bool
}
```

### 10.5 使用示例

**示例 1: 快速初始化**

```bash
$ owl ai config init
✓ 配置文件已创建: ~/.owl/config.yaml

下一步：
  1. 编辑配置文件设置 API Key
  2. 或使用 'owl ai config' 进行交互式配置
```

**示例 2: 查看当前配置**

```bash
$ owl ai config show
当前配置:

  Provider:    openai
  Model:       gpt-4o
  API Key:     sk-****-xxxx
  Base URL:    https://api.openai.com/v1
  Timeout:     120s
```

**示例 3: 交互式配置**

```bash
$ owl ai config
╔════════════════════════════════════════════╗
║        owl AI 配置向导                   ║
╚════════════════════════════════════════════╝

步骤 1/4: 选择 Provider
  ○ openai
  ● anthropic
  ○ dashscope
  ○ qwen
  ○ deepseek
[↑/↓] 选择, [Enter] 确认

步骤 2/4: 选择 Model
  ○ claude-3.5-sonnet
  ● claude-3-opus
  ○ claude-3-sonnet
  ○ claude-3-haiku

步骤 3/4: 输入 API Key
  sk-ant-************************************************
  [隐藏输入]

步骤 4/4: Base URL (可选，直接回车跳过)

✓ 配置已保存到: ~/.owl/config.yaml
```

### 10.6 生成的配置文件示例

```yaml
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}
  base_url: ""
  timeout: 120

prompts:
  system: system.md
  playbook: playbook.md
  command: command.md
  transfer: transfer.md

safety:
  confirm_dangerous: true
  allowed_commands: []
  blocked_commands:
  - rm -rf /
  - rm -rf /*
  - ":(){:|:&};:"
  - ">/dev/sda"
  - dd if=/dev/zero of=/dev/sda
```

---

## 11. 测试计划更新

| 测试用例 | 说明 | 预期结果 |
|---------|------|---------|
| TC-AI-CONFIG-008 | owl ai config init | 成功生成配置文件 |
| TC-AI-CONFIG-009 | owl ai config init (文件已存在) | 提示文件已存在 |
| TC-AI-CONFIG-010 | owl ai config show | 显示当前配置 (隐藏 API Key) |
| TC-AI-CONFIG-011 | 交互式配置流程 | 依次询问并保存配置 |
