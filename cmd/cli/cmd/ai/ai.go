package ai

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/playbook"
)

var (
	aiModel    string
	aiProvider string
	aiAPIKey   string
	aiBaseURL  string
	aiTimeout  int
	aiSession  string
)

func NewAICmd() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: "AI 智能助手模式",
		Long: `启动 AI 智能助手交互模式，通过自然语言执行分布式运维操作。

支持的功能：
- 查询节点信息：查询节点状态、分组、标签
- 执行批量命令：在指定节点上执行命令
- 生成剧本：根据需求生成 Ansible-like YAML 剧本
- 文件传输：传输文件到指定节点

示例：
  owl ai
  owl ai --model gpt-4o
  owl ai --provider dashscope --api-key sk-xxx
  echo "查询所有在线节点" | owl ai`,
		Run: runAI,
	}

	aiCmd.Flags().StringVar(&aiModel, "model", "gpt-4o",
		"AI 模型名称")
	aiCmd.Flags().StringVar(&aiProvider, "provider", "openai",
		"AI 提供商: openai, anthropic, dashscope")
	aiCmd.Flags().StringVar(&aiAPIKey, "api-key", "",
		"API Key (也可通过环境变量 OWL_API_KEY 设置)")
	aiCmd.Flags().StringVar(&aiBaseURL, "base-url", "",
		"API Base URL (用于代理或自定义端点，也可通过环境变量 OWL_BASE_URL 设置)")
	aiCmd.Flags().IntVar(&aiTimeout, "timeout", 120,
		"请求超时时间 (秒)")
	aiCmd.Flags().StringVar(&aiSession, "session", "",
		"会话 ID (用于恢复会话)")

	aiCmd.AddCommand(NewModelsCmd())
	aiCmd.AddCommand(NewConfigCmd())

	return aiCmd
}

func NewModelsCmd() *cobra.Command {
	modelsCmd := &cobra.Command{
		Use:   "models",
		Short: "列出可用的 AI 模型",
		Long: `从 API 获取并列出可用的 AI 模型列表。

示例：
  owl ai models
  owl ai models --provider openai --api-key sk-xxx`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()

			home, _ := os.UserHomeDir()
			configPath := filepath.Join(home, ".owl", "config.yaml")
			fileConfig, _ := ai.LoadConfig(configPath)
			if fileConfig == nil {
				fileConfig = ai.DefaultConfig()
			}

			provider := aiProvider
			model := "gpt-4o"
			apiKey := getAPIKey()
			baseURL := getBaseURL()
			timeout := aiTimeout

			if !cmd.Flags().Changed("provider") && fileConfig.AI.Provider != "" {
				provider = fileConfig.AI.Provider
			}
			if apiKey == "" {
				apiKey = fileConfig.AI.APIKey
			}
			if baseURL == "" {
				baseURL = fileConfig.AI.BaseURL
			}

			config := &ai.Config{
				AI: ai.AIConfig{
					Provider: provider,
					Model:    model,
					APIKey:   apiKey,
					BaseURL:  baseURL,
					Timeout:  timeout,
				},
			}

			if config.AI.APIKey == "" {
				fmt.Fprintf(os.Stderr, "错误: 请提供 API Key (使用 --api-key 参数或设置 OWL_API_KEY 环境变量)\n")
				os.Exit(1)
			}

			if aiProvider != "openai" && aiProvider != "qwen" && aiProvider != "dashscope" && aiProvider != "deepseek" && aiProvider != "" {
				fmt.Fprintf(os.Stderr, "错误: %s 提供商不支持模型列表 API\n", aiProvider)
				os.Exit(1)
			}

			fmt.Println("正在获取可用模型列表...")
			fmt.Println()

			client := ai.NewOpenAIClient(config)
			models, err := client.ListModels(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误: 获取模型列表失败: %v\n", err)
				os.Exit(1)
			}

			if len(models) == 0 {
				fmt.Println("未找到可用模型")
				return
			}

			fmt.Println("可用模型:")
			fmt.Println()
			for _, m := range models {
				fmt.Printf("  • %s\n", m)
			}
			fmt.Println()
			fmt.Printf("共找到 %d 个模型\n", len(models))
		},
	}

	modelsCmd.Flags().StringVar(&aiProvider, "provider", "openai",
		"AI 提供商: openai, anthropic, dashscope")
	modelsCmd.Flags().StringVar(&aiAPIKey, "api-key", "",
		"API Key (也可通过环境变量 OWL_API_KEY 设置)")
	modelsCmd.Flags().StringVar(&aiBaseURL, "base-url", "",
		"API Base URL (用于代理或自定义端点，也可通过环境变量 OWL_BASE_URL 设置)")
	modelsCmd.Flags().IntVar(&aiTimeout, "timeout", 30,
		"请求超时时间 (秒)")

	return modelsCmd
}

func runAI(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	store := common.GetNodeStore()
	bridge := createBridgeFromStore(store)

	nodeMgr := ai.InitNodeManager(bridge)
	if nodeMgr == nil {
		fmt.Fprintf(os.Stderr, "Error: 初始化节点管理器失败\n")
		os.Exit(1)
	}

	playbookParser := playbook.NewParser()

	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".owl", "config.yaml")
	fileConfig, err := ai.LoadConfig(configPath)
	if err != nil {
		fileConfig = ai.DefaultConfig()
	}

	provider := aiProvider
	model := aiModel
	apiKey := getAPIKey()
	baseURL := getBaseURL()
	timeout := aiTimeout

	if !cmd.Flags().Changed("provider") && fileConfig.AI.Provider != "" {
		provider = fileConfig.AI.Provider
	}
	if !cmd.Flags().Changed("model") && fileConfig.AI.Model != "" {
		model = fileConfig.AI.Model
	}
	if apiKey == "" {
		apiKey = fileConfig.AI.APIKey
	}
	if baseURL == "" {
		baseURL = fileConfig.AI.BaseURL
	}
	if !cmd.Flags().Changed("timeout") && fileConfig.AI.Timeout > 0 {
		timeout = fileConfig.AI.Timeout
	}

	config := &ai.Config{
		AI: ai.AIConfig{
			Provider: provider,
			Model:    model,
			APIKey:   apiKey,
			BaseURL:  baseURL,
			Timeout:  timeout,
		},
	}

	agent, err := ai.NewAgent(config, nodeMgr, playbookParser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize Eino LLM: %v, using fallback mode\n", err)
	}

	if len(args) > 0 {
		query := strings.Join(args, " ")
		response, err := agent.Process(ctx, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)
		return
	}

	fmt.Println("\033[36m╔════════════════════════════════════════════════════════════╗\033[0m")
	fmt.Println("\033[36m║           owl-AI 智能运维助手                          ║\033[0m")
	fmt.Println("\033[36m╚════════════════════════════════════════════════════════════╝\033[0m")
	fmt.Println()
	fmt.Println("欢迎使用 owl-AI！您可以用自然语言执行以下操作：")
	fmt.Println()
	fmt.Println("  \033[33m•\033[0m 查询节点信息：\"查看所有 web 节点\"")
	fmt.Println("  \033[33m•\033[0m 执行命令：\"在所有节点上执行 uptime\"")
	fmt.Println("  \033[33m•\033[0m 生成剧本：\"生成一个部署 nginx 的剧本\"")
	fmt.Println("  \033[33m•\033[0m 传输文件：\"上传 app.tar.gz 到所有节点\"")
	fmt.Println()
	fmt.Println("\033[90m输入 'quit' 或 'exit' 退出\033[0m")
	fmt.Println()

	session := ai.NewSessionManager()
	sessionID := aiSession
	if sessionID == "" {
		sessionID = "default"
	}
	currentSession := session.CreateSession(sessionID, agent)

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("\033[32m您>\033[0m ")
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			fmt.Print("\033[32m您>\033[0m ")
			continue
		}

		if strings.EqualFold(input, "quit") || strings.EqualFold(input, "exit") {
			fmt.Println("\033[90m再见！\033[0m")
			break
		}

		if strings.EqualFold(input, "help") {
			printHelp()
			fmt.Print("\033[32m您>\033[0m ")
			continue
		}

		if strings.HasPrefix(input, "!") {
			cmdStr := strings.TrimPrefix(input, "!")
			handleDirectCommand(cmdStr)
			fmt.Print("\033[32m您>\033[0m ")
			continue
		}

		response, err := currentSession.Send(ctx, input)
		if err != nil {
			fmt.Printf("\033[31m错误: %v\033[0m\n", err)
		} else {
			fmt.Printf("\033[36mAI>\033[0m %s\n", response)
		}

		fmt.Println()
		fmt.Print("\033[32m您>\033[0m ")
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "读取输入错误: %v\n", err)
	}
}

func createBridgeFromStore(store common.NodeStore) *ai.NodeStoreBridge {
	bridge := ai.NewNodeStoreBridge()
	nodes, err := store.List()
	if err != nil {
		return bridge
	}
	for _, n := range nodes {
		bridge.Add(&ai.NodeInfoAdapter{
			ID:        n.ID,
			Name:      n.Name,
			Address:   n.Address,
			Port:      n.Port,
			Status:    n.Status,
			Groups:    n.Groups,
			Labels:    n.Labels,
			CreatedAt: n.CreatedAt,
			UpdatedAt: n.UpdatedAt,
		})
	}
	return bridge
}

func getAPIKey() string {
	if aiAPIKey != "" {
		return aiAPIKey
	}

	envKey := os.Getenv("OWL_API_KEY")
	if envKey != "" {
		return envKey
	}

	return ""
}

func getBaseURL() string {
	if aiBaseURL != "" {
		return aiBaseURL
	}

	envBaseURL := os.Getenv("OWL_BASE_URL")
	if envBaseURL != "" {
		return envBaseURL
	}

	return ""
}

func printHelp() {
	fmt.Println()
	fmt.Println("\033[33m可用命令：\033[0m")
	fmt.Println()
	fmt.Println("  \033[90mhelp\033[0m         - 显示此帮助信息")
	fmt.Println("  \033[90mquit/exit\033[0m   - 退出程序")
	fmt.Println("  \033[90m!command\033[0m    - 执行直接命令 (如 !node list)")
	fmt.Println()
	fmt.Println("\033[33m示例：\033[0m")
	fmt.Println()
	fmt.Println("  查看所有节点")
	fmt.Println("  → 查询所有节点")
	fmt.Println()
	fmt.Println("  在 web 组执行 uptime")
	fmt.Println("  → 在 web 组的节点上执行 uptime")
	fmt.Println()
	fmt.Println("  生成部署脚本")
	fmt.Println("  → 生成一个部署 nginx 的剧本")
	fmt.Println()
}

func handleDirectCommand(cmdStr string) {
	fmt.Printf("\033[90m[直接执行: %s]\033[0m\n", cmdStr)
	fmt.Println("（直接命令功能需要在完整 CLI 环境中执行）")
}

func NewSessionManager() *ai.SessionManager {
	return ai.NewSessionManager()
}
