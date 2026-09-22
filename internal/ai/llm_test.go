package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testAPIKey = "test-key"

func makeTestConfig(provider, model string) *Config {
	return &Config{
		AI: AIConfig{
			Provider: provider,
			APIKey:   testAPIKey,
			Model:    model,
		},
	}
}

func makeOpenAITestConfig(model, baseURL string) *Config {
	cfg := &Config{
		AI: AIConfig{
			APIKey:  testAPIKey,
			Model:   model,
			BaseURL: baseURL,
			Timeout: 60,
		},
	}
	if baseURL == "" {
		cfg.AI.BaseURL = "https://api.openai.com/v1"
	}
	return cfg
}

func TestOpenAIClient_NewOpenAIClient(t *testing.T) {
	config := makeOpenAITestConfig("gpt-4o", "https://api.openai.com/v1")

	client := NewOpenAIClient(config)

	if client.apiKey != testAPIKey {
		t.Errorf("expected API key '%s', got '%s'", testAPIKey, client.apiKey)
	}
	if client.model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got '%s'", client.model)
	}
	if client.baseURL != "https://api.openai.com/v1" {
		t.Errorf("expected base URL 'https://api.openai.com/v1', got '%s'", client.baseURL)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be set")
	}
}

func TestOpenAIClient_NewOpenAIClient_DefaultBaseURL(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			APIKey:  testAPIKey,
			Model:   "gpt-4o",
			BaseURL: "",
			Timeout: 60,
		},
	}

	client := NewOpenAIClient(config)

	if client.baseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default base URL 'https://api.openai.com/v1', got '%s'", client.baseURL)
	}
}

func TestAnthropicClient_NewAnthropicClient(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			APIKey:  testAPIKey,
			Model:   "claude-3-opus",
			Timeout: 60,
		},
	}

	client := NewAnthropicClient(config)

	if client.apiKey != testAPIKey {
		t.Errorf("expected API key '%s', got '%s'", testAPIKey, client.apiKey)
	}
	if client.model != "claude-3-opus" {
		t.Errorf("expected model 'claude-3-opus', got '%s'", client.model)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be set")
	}
}

func TestCreateLLMClient_OpenAI(t *testing.T) {
	config := makeTestConfig("openai", "gpt-4o")

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*HTTPModel); !ok {
		t.Error("expected HTTPModel type")
	}
}

func TestCreateLLMClient_Anthropic(t *testing.T) {
	config := makeTestConfig("anthropic", "claude-3-opus")

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*HTTPModel); !ok {
		t.Error("expected HTTPModel type")
	}
}

func TestCreateLLMClient_Qwen(t *testing.T) {
	config := makeTestConfig("qwen", "qwen-turbo")

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*HTTPModel); !ok {
		t.Error("expected HTTPModel type for qwen")
	}
}

func TestCreateLLMClient_DeepSeek(t *testing.T) {
	config := makeTestConfig("deepseek", "deepseek-chat")

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*HTTPModel); !ok {
		t.Error("expected HTTPModel type for deepseek")
	}
}

func TestCreateLLMClient_DefaultProvider(t *testing.T) {
	config := makeTestConfig("", "gpt-4o")

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*HTTPModel); !ok {
		t.Error("expected HTTPModel type for default provider")
	}
}

func TestCreateLLMClient_UnsupportedProvider(t *testing.T) {
	config := makeTestConfig("unsupported", "some-model")

	_, err := CreateLLMClient(config)
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestCreateLLMClient_MissingAPIKey(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "openai",
			APIKey:   "",
			Model:    "gpt-4o",
		},
	}

	_, err := CreateLLMClient(config)
	if err == nil {
		t.Error("expected error for missing API key")
	}
}

func TestCreateLLMClient_Qwen_DefaultValues(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "qwen",
			APIKey:   testAPIKey,
			Model:    "",
			BaseURL:  "",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}

	httpModel, ok := client.(*HTTPModel)
	if !ok {
		t.Fatal("expected HTTPModel type")
	}
	if httpModel.model != "qwen-max" {
		t.Errorf("expected default model 'qwen-max', got '%s'", httpModel.model)
	}
	if httpModel.baseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Errorf("expected default base URL for qwen, got '%s'", httpModel.baseURL)
	}
}

func TestCreateLLMClient_DeepSeek_DefaultValues(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "deepseek",
			APIKey:   testAPIKey,
			Model:    "",
			BaseURL:  "",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}

	httpModel, ok := client.(*HTTPModel)
	if !ok {
		t.Fatal("expected HTTPModel type")
	}
	if httpModel.model != "deepseek-flash" {
		t.Errorf("expected default model 'deepseek-flash', got '%s'", httpModel.model)
	}
	if httpModel.baseURL != "https://api.deepseek.com" {
		t.Errorf("expected default base URL for deepseek, got '%s'", httpModel.baseURL)
	}
}

func TestCreateLLMClient_Anthropic_DefaultModel(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "anthropic",
			APIKey:   testAPIKey,
			Model:    "",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}

	httpModel, ok := client.(*HTTPModel)
	if !ok {
		t.Fatal("expected HTTPModel type")
	}
	if httpModel.model != "claude-sonnet-5" {
		t.Errorf("expected default model 'claude-sonnet-5', got '%s'", httpModel.model)
	}
}

func TestCreateLLMClient_OpenAI_DefaultValues(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "openai",
			APIKey:   testAPIKey,
			Model:    "",
			BaseURL:  "",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}

	httpModel, ok := client.(*HTTPModel)
	if !ok {
		t.Fatal("expected HTTPModel type")
	}
	if httpModel.baseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default base URL for openai, got '%s'", httpModel.baseURL)
	}
}

func TestCreateLLMClient_DashScope_DefaultValues(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "dashscope",
			APIKey:   testAPIKey,
			Model:    "",
			BaseURL:  "",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}

	httpModel, ok := client.(*HTTPModel)
	if !ok {
		t.Fatal("expected HTTPModel type")
	}
	if httpModel.model != "qwen-max" {
		t.Errorf("expected default model 'qwen-max', got '%s'", httpModel.model)
	}
	if httpModel.baseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Errorf("expected default base URL for dashscope, got '%s'", httpModel.baseURL)
	}
}

func TestAllRegisteredProviders(t *testing.T) {
	cfg := LoadConfigForTest(t)

	providers := []string{"openai", "anthropic", "qwen", "dashscope", "deepseek"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			cfg.AI.Provider = provider
			client, err := CreateLLMClient(cfg)
			if err != nil {
				t.Errorf("unexpected error for provider '%s': %v", provider, err)
				return
			}
			if client == nil {
				t.Errorf("expected client for provider '%s'", provider)
			}
		})
	}
}

func TestMessage_Struct(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "Hello, world!",
	}

	if msg.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", msg.Role)
	}
	if msg.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got '%s'", msg.Content)
	}
}

func TestLLMClient_Interface(t *testing.T) {
	var client LLMClient

	config := makeTestConfig("openai", "gpt-4o")

	client = NewOpenAIClient(config)
	if client == nil {
		t.Error("expected OpenAIClient to implement LLMClient interface")
	}

	config = makeTestConfig("anthropic", "claude-3-opus")
	client = NewAnthropicClient(config)
	if client == nil {
		t.Error("expected AnthropicClient to implement LLMClient interface")
	}
}

func TestHTTPModel_Generate_UnauthorizedError(t *testing.T) {
	// 401 响应必须以 error 形式暴露（离线验证鉴权失败路径）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{
		APIType: "openai",
		BaseURL: server.URL,
		Model:   "gpt-4o",
		APIKey:  "bad-key",
	})

	_, err := client.Generate(context.Background(), []Message{{Role: "user", Content: "Hello"}})
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestHTTPModel_Generate_EmptyBaseURLError(t *testing.T) {
	// baseURL 为空时必须报错而不是发起相对路径请求
	client := NewHTTPModel(ModelOptions{
		APIType: "openai",
		BaseURL: "",
		Model:   "gpt-4o",
		APIKey:  testAPIKey,
	})

	_, err := client.Generate(context.Background(), []Message{{Role: "user", Content: "Hello"}})
	if err == nil {
		t.Error("expected error when base URL is empty")
	}
}

func TestHTTPModel_Anthropic_UnauthorizedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	client := NewHTTPModel(ModelOptions{
		APIType: "anthropic",
		BaseURL: server.URL,
		Model:   "claude-3-opus",
		APIKey:  "bad-key",
	})

	_, err := client.Generate(context.Background(), []Message{{Role: "user", Content: "Hello"}})
	if err == nil {
		t.Error("expected error for 401 response")
	}
}
