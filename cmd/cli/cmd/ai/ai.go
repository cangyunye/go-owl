package ai

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/cangyunye/go-owl/internal/control/playbook"
	internalhistory "github.com/cangyunye/go-owl/internal/history"
	"github.com/cangyunye/go-owl/internal/i18n"
)

var (
	aiModel    string
	aiProvider string
	aiAPIKey   string
	aiBaseURL  string
	aiTimeout  int
	aiSession  string
	aiVerbose  bool
)

func NewAICmd() *cobra.Command {
	aiCmd := &cobra.Command{
		Use:   "ai",
		Short: i18n.T("ai.cmd.short"),
		Long:  i18n.T("ai.cmd.long"),
		Run:   runAI,
	}

	aiCmd.Flags().StringVar(&aiModel, "model", "gpt-4o",
		i18n.T("ai.flag.model"))
	aiCmd.Flags().StringVar(&aiProvider, "provider", "openai",
		i18n.T("ai.flag.provider"))
	aiCmd.Flags().StringVar(&aiAPIKey, "api-key", "",
		i18n.T("ai.flag.api_key"))
	aiCmd.Flags().StringVar(&aiBaseURL, "base-url", "",
		i18n.T("ai.flag.base_url"))
	aiCmd.Flags().IntVar(&aiTimeout, "timeout", 120,
		i18n.T("ai.flag.timeout"))
	aiCmd.Flags().StringVar(&aiSession, "session", "",
		i18n.T("ai.flag.session"))
	aiCmd.Flags().BoolVarP(&aiVerbose, "verbose", "v", false,
		i18n.T("ai.flag.verbose"))
	// 保留 --debug 作为别名以保持向后兼容性
	aiCmd.Flags().BoolVar(&aiVerbose, "debug", false,
		i18n.T("ai.flag.debug_alias"))

	aiCmd.AddCommand(NewModelsCmd())
	aiCmd.AddCommand(NewConfigCmd())
	aiCmd.AddCommand(NewHistoryCmd())

	return aiCmd
}

func NewModelsCmd() *cobra.Command {
	modelsCmd := &cobra.Command{
		Use:   "models",
		Short: i18n.T("ai.models.short"),
		Long:  i18n.T("ai.models.long"),
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
				fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.models.err_no_api_key"))
				os.Exit(1)
			}

			if aiProvider != "openai" && aiProvider != "qwen" && aiProvider != "dashscope" && aiProvider != "deepseek" && aiProvider != "" {
				fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.models.err_provider_unsupported", aiProvider))
				os.Exit(1)
			}

			fmt.Println(i18n.T("ai.models.fetching"))
			fmt.Println()

			client := ai.NewOpenAIClient(config)
			models, err := client.ListModels(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.models.err_list_failed", err))
				os.Exit(1)
			}

			if len(models) == 0 {
				fmt.Println(i18n.T("ai.models.none_found"))
				return
			}

			fmt.Println(i18n.T("ai.models.available"))
			fmt.Println()
			for _, m := range models {
				fmt.Printf("  • %s\n", m)
			}
			fmt.Println()
			fmt.Printf("%s", i18n.T("ai.models.total", i18n.F(len(models))))
		},
	}

	modelsCmd.Flags().StringVar(&aiProvider, "provider", "openai",
		i18n.T("ai.flag.provider"))
	modelsCmd.Flags().StringVar(&aiAPIKey, "api-key", "",
		i18n.T("ai.flag.api_key"))
	modelsCmd.Flags().StringVar(&aiBaseURL, "base-url", "",
		i18n.T("ai.flag.base_url"))
	modelsCmd.Flags().IntVar(&aiTimeout, "timeout", 30,
		i18n.T("ai.flag.timeout"))

	return modelsCmd
}

func progressLog(sessionID string, debug bool, step string, detail string) {
	timestamp := time.Now().Format("15:04:05")

	role := "assistant"
	var label string
	switch step {
	case "route":
		label = i18n.T("ai.progress.route", detail)
	case "analyze":
		label = i18n.T("ai.progress.analyze")
	case "generate":
		label = i18n.T("ai.progress.generate", detail)
	case "execute":
		label = i18n.T("ai.progress.execute")
	case "result":
		if strings.HasPrefix(detail, "失败") {
			label = detail
		} else {
			label = i18n.T("ai.progress.done")
		}
	default:
		label = detail
	}

	fmt.Fprintf(os.Stderr, "[%s] owl-ai: %s\n", timestamp, label)

	chat := &internalhistory.AiChat{
		SessionID: sessionID,
		Step:      step,
		Role:      role,
		Output:    detail,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	internalhistory.RecordAiChatGlobal(chat)
}

func debugLog(debug bool, format string, args ...interface{}) {
	if debug {
		timestamp := time.Now().Format("15:04:05")
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[%s] DEBUG: %s\n", timestamp, msg)
	}
}

func truncateForDB(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

func runAI(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// 设置日志详细模式
	ai.SetLogVerbose(aiVerbose)
	ai.SetLLMLogVerbose(aiVerbose)

	store := common.GetNodeStore()
	nodeStoreAdapter := createBridgeAdapter(store)
	bridge := ai.NewNodeStoreBridge()
	bridge.SyncFromStore(nodeStoreAdapter)

	nodeMgr := ai.InitNodeManager(bridge)
	if nodeMgr == nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.error_init_node_manager"))
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

	sessionID := fmt.Sprintf("ai-%d", time.Now().UnixMilli())

	executor := ai.NewCLIExecutor(nodeMgr, nodeStoreAdapter)
	agent, err := ai.NewAgent(executor, config, nodeMgr, nodeStoreAdapter, playbookParser, aiVerbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize Eino LLM: %v, using fallback mode\n", err)
	}

	if len(args) > 0 {
		query := strings.Join(args, " ")
		debugLog(aiVerbose, "user input: %s", query)
		timestamp := time.Now().Format("15:04:05")
		fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.chat.user_line", timestamp, query))

		internalhistory.RecordAiChatGlobal(&internalhistory.AiChat{
			SessionID: sessionID,
			Step:      "route",
			Role:      "user",
			Input:     query,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})

		onProgress := func(step string, detail string) {
			progressLog(sessionID, aiVerbose, step, detail)
		}

		// 单次（非交互）模式：写操作无法交互确认，直接拒绝并提示。
		agent.SetConfirmGate(ai.RejectWriteOpsGate())

		response, err := agent.Process(ctx, query, onProgress)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.chat.failed", time.Now().Format("15:04:05"), err))
			os.Exit(1)
		}

		internalhistory.RecordAiChatGlobal(&internalhistory.AiChat{
			SessionID: sessionID,
			Step:      "result",
			Role:      "assistant",
			Output:    truncateForDB(response, 4096),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})

		fmt.Println(response)
		return
	}

	fmt.Println("\033[36m╔════════════════════════════════════════════════════════════╗\033[0m")
	fmt.Println(i18n.T("ai.banner.title"))
	fmt.Println("\033[36m╚════════════════════════════════════════════════════════════╝\033[0m")
	fmt.Println()
	fmt.Println(i18n.T("ai.welcome.intro"))
	fmt.Println()
	fmt.Println(i18n.T("ai.welcome.item_query"))
	fmt.Println(i18n.T("ai.welcome.item_exec"))
	fmt.Println(i18n.T("ai.welcome.item_playbook"))
	fmt.Println(i18n.T("ai.welcome.item_transfer"))
	fmt.Println()
	fmt.Println(i18n.T("ai.welcome.quit_hint"))
	fmt.Println()

	session := ai.NewSessionManager()
	sessionID = aiSession
	if sessionID == "" {
		sessionID = "default"
	}
	currentSession := session.CreateSession(sessionID, agent)
	currentSession.OnProgress = func(step string, detail string) {
		progressLog(sessionID, aiVerbose, step, detail)
	}
	currentSession.SetDefaultConfirmGate()

	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print(i18n.T("ai.chat.prompt"))
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			fmt.Print(i18n.T("ai.chat.prompt"))
			continue
		}

		if strings.EqualFold(input, "quit") || strings.EqualFold(input, "exit") {
			fmt.Println(i18n.T("ai.chat.bye"))
			break
		}

		if strings.EqualFold(input, "help") {
			printHelp()
			fmt.Print(i18n.T("ai.chat.prompt"))
			continue
		}

		if strings.HasPrefix(input, "!") {
			cmdStr := strings.TrimPrefix(input, "!")
			handleDirectCommand(cmdStr)
			fmt.Print(i18n.T("ai.chat.prompt"))
			continue
		}

		internalhistory.RecordAiChatGlobal(&internalhistory.AiChat{
			SessionID: sessionID,
			Step:      "route",
			Role:      "user",
			Input:     input,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})

		response, err := currentSession.Send(ctx, input)
		if err != nil {
			fmt.Printf("%s", i18n.T("ai.chat.error", err))
		} else {
			fmt.Printf("\033[36mAI>\033[0m %s\n", response)
		}

		msgCount := currentSession.MessageCount()
		if msgCount > 0 {
			fmt.Printf("%s", i18n.T("ai.chat.context_count", i18n.F(msgCount)))
		}
		fmt.Println()
		fmt.Print(i18n.T("ai.chat.prompt"))
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("ai.chat.err_read_input", err))
	}
}

type storeAdapter struct {
	store common.NodeStore
}

func (a *storeAdapter) List() ([]*ai.NodeInfoAdapter, error) {
	nodes, err := a.store.List()
	if err != nil {
		return nil, err
	}
	result := make([]*ai.NodeInfoAdapter, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, &ai.NodeInfoAdapter{
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
	return result, nil
}

func (a *storeAdapter) Get(id string) (*ai.NodeInfoAdapter, error) {
	node, err := a.store.Get(id)
	if err != nil {
		return nil, err
	}
	return &ai.NodeInfoAdapter{
		ID:        node.ID,
		Name:      node.Name,
		Address:   node.Address,
		Port:      node.Port,
		Status:    node.Status,
		Groups:    node.Groups,
		Labels:    node.Labels,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	}, nil
}

func (a *storeAdapter) Add(node *ai.NodeInfoAdapter) error {
	return a.store.Add(&common.NodeInfo{
		ID:        node.ID,
		Name:      node.Name,
		Address:   node.Address,
		Port:      node.Port,
		User:      "",
		Status:    node.Status,
		Groups:    node.Groups,
		Labels:    node.Labels,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	})
}

func (a *storeAdapter) Remove(id string) error {
	return a.store.Remove(id)
}

func (a *storeAdapter) Update(node *ai.NodeInfoAdapter) error {
	return a.store.Update(&common.NodeInfo{
		ID:        node.ID,
		Name:      node.Name,
		Address:   node.Address,
		Port:      node.Port,
		User:      "",
		Status:    node.Status,
		Groups:    node.Groups,
		Labels:    node.Labels,
		CreatedAt: node.CreatedAt,
		UpdatedAt: node.UpdatedAt,
	})
}

func (a *storeAdapter) Save() error {
	return a.store.Save()
}

func (a *storeAdapter) Load() error {
	return a.store.Load()
}

func createBridgeAdapter(store common.NodeStore) ai.NodeStoreAdapter {
	return &storeAdapter{store: store}
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
	fmt.Println(i18n.T("ai.help.title_commands"))
	fmt.Println()
	fmt.Println(i18n.T("ai.help.cmd_help"))
	fmt.Println(i18n.T("ai.help.cmd_quit"))
	fmt.Println(i18n.T("ai.help.cmd_direct"))
	fmt.Println()
	fmt.Println(i18n.T("ai.help.title_examples"))
	fmt.Println()
	fmt.Println(i18n.T("ai.help.ex1_query"))
	fmt.Println(i18n.T("ai.help.ex1_arrow"))
	fmt.Println()
	fmt.Println(i18n.T("ai.help.ex2_query"))
	fmt.Println(i18n.T("ai.help.ex2_arrow"))
	fmt.Println()
	fmt.Println(i18n.T("ai.help.ex3_query"))
	fmt.Println(i18n.T("ai.help.ex3_arrow"))
	fmt.Println()
}

func handleDirectCommand(cmdStr string) {
	fmt.Printf("%s", i18n.T("ai.direct.executing", cmdStr))
	fmt.Println(i18n.T("ai.direct.not_supported"))
}

func NewSessionManager() *ai.SessionManager {
	return ai.NewSessionManager()
}
