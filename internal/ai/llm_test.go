package ai

import (
	"context"
	"testing"
)

func TestOpenAIClient_NewOpenAIClient(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			APIKey:  "test-key",
			Model:   "gpt-4o",
			BaseURL: "https://api.openai.com/v1",
			Timeout: 60,
		},
	}

	client := NewOpenAIClient(config)

	if client.apiKey != "test-key" {
		t.Errorf("expected API key 'test-key', got '%s'", client.apiKey)
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
			APIKey:  "test-key",
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
			APIKey:  "test-key",
			Model:   "claude-3-opus",
			Timeout: 60,
		},
	}

	client := NewAnthropicClient(config)

	if client.apiKey != "test-key" {
		t.Errorf("expected API key 'test-key', got '%s'", client.apiKey)
	}
	if client.model != "claude-3-opus" {
		t.Errorf("expected model 'claude-3-opus', got '%s'", client.model)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be set")
	}
}

func TestCreateLLMClient_OpenAI(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "openai",
			APIKey:   "test-key",
			Model:    "gpt-4o",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*OpenAIClient); !ok {
		t.Error("expected OpenAIClient type")
	}
}

func TestCreateLLMClient_Anthropic(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "anthropic",
			APIKey:   "test-key",
			Model:    "claude-3-opus",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*AnthropicClient); !ok {
		t.Error("expected AnthropicClient type")
	}
}

func TestCreateLLMClient_Qwen(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "qwen",
			APIKey:   "test-key",
			Model:    "qwen-turbo",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*OpenAIClient); !ok {
		t.Error("expected OpenAIClient type for qwen")
	}
}

func TestCreateLLMClient_DeepSeek(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "deepseek",
			APIKey:   "test-key",
			Model:    "deepseek-chat",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*OpenAIClient); !ok {
		t.Error("expected OpenAIClient type for deepseek")
	}
}

func TestCreateLLMClient_DefaultProvider(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "",
			APIKey:   "test-key",
			Model:    "gpt-4o",
		},
	}

	client, err := CreateLLMClient(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client to be created")
	}
	if _, ok := client.(*OpenAIClient); !ok {
		t.Error("expected OpenAIClient type for default provider")
	}
}

func TestCreateLLMClient_UnsupportedProvider(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "unsupported",
			APIKey:   "test-key",
			Model:    "some-model",
		},
	}

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
			APIKey:   "test-key",
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

	openAIClient, ok := client.(*OpenAIClient)
	if !ok {
		t.Fatal("expected OpenAIClient type")
	}
	if openAIClient.model != "qwen-turbo" {
		t.Errorf("expected default model 'qwen-turbo', got '%s'", openAIClient.model)
	}
	if openAIClient.baseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Errorf("expected default base URL for qwen, got '%s'", openAIClient.baseURL)
	}
}

func TestCreateLLMClient_DeepSeek_DefaultValues(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "deepseek",
			APIKey:   "test-key",
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

	openAIClient, ok := client.(*OpenAIClient)
	if !ok {
		t.Fatal("expected OpenAIClient type")
	}
	if openAIClient.model != "deepseek-chat" {
		t.Errorf("expected default model 'deepseek-chat', got '%s'", openAIClient.model)
	}
	if openAIClient.baseURL != "https://api.deepseek.com" {
		t.Errorf("expected default base URL for deepseek, got '%s'", openAIClient.baseURL)
	}
}

func TestCreateLLMClient_Anthropic_DefaultModel(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "anthropic",
			APIKey:   "test-key",
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

	anthropicClient, ok := client.(*AnthropicClient)
	if !ok {
		t.Fatal("expected AnthropicClient type")
	}
	if anthropicClient.model != "claude-3-opus-20240229" {
		t.Errorf("expected default model 'claude-3-opus-20240229', got '%s'", anthropicClient.model)
	}
}

func TestCreateLLMClient_OpenAI_DefaultValues(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "openai",
			APIKey:   "test-key",
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

	openAIClient, ok := client.(*OpenAIClient)
	if !ok {
		t.Fatal("expected OpenAIClient type")
	}
	if openAIClient.baseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default base URL for openai, got '%s'", openAIClient.baseURL)
	}
}

func TestCreateLLMClient_DashScope_DefaultValues(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			Provider: "dashscope",
			APIKey:   "test-key",
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

	openAIClient, ok := client.(*OpenAIClient)
	if !ok {
		t.Fatal("expected OpenAIClient type")
	}
	if openAIClient.model != "qwen-turbo" {
		t.Errorf("expected default model 'qwen-turbo', got '%s'", openAIClient.model)
	}
	if openAIClient.baseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Errorf("expected default base URL for dashscope, got '%s'", openAIClient.baseURL)
	}
}

func TestAllRegisteredProviders(t *testing.T) {
	providers := []string{"openai", "anthropic", "qwen", "dashscope", "deepseek"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			config := &Config{
				AI: AIConfig{
					Provider: provider,
					APIKey:   "test-key",
				},
			}
			client, err := CreateLLMClient(config)
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

	config := &Config{
		AI: AIConfig{
			Provider: "openai",
			APIKey:   "test-key",
			Model:    "gpt-4o",
		},
	}

	client = NewOpenAIClient(config)
	if client == nil {
		t.Error("expected OpenAIClient to implement LLMClient interface")
	}

	client = NewAnthropicClient(config)
	if client == nil {
		t.Error("expected AnthropicClient to implement LLMClient interface")
	}
}

func TestOpenAIClient_Generate_MissingAPIKey(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			APIKey:  "",
			Model:   "gpt-4o",
			BaseURL: "https://api.openai.com/v1",
			Timeout: 60,
		},
	}

	client := NewOpenAIClient(config)

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	ctx := context.Background()
	_, err := client.Generate(ctx, messages)

	if err == nil {
		t.Error("expected error when API key is empty")
	}
}

func TestOpenAIClient_Generate_MissingBaseURL(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			APIKey:  "test-key",
			Model:   "gpt-4o",
			BaseURL: "",
			Timeout: 60,
		},
	}

	client := NewOpenAIClient(config)

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	ctx := context.Background()
	_, err := client.Generate(ctx, messages)

	if err == nil {
		t.Error("expected error when base URL is empty")
	}
}

func TestAnthropicClient_Generate_MissingAPIKey(t *testing.T) {
	config := &Config{
		AI: AIConfig{
			APIKey:  "",
			Model:   "claude-3-opus",
			Timeout: 60,
		},
	}

	client := NewAnthropicClient(config)

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	ctx := context.Background()
	_, err := client.Generate(ctx, messages)

	if err == nil {
		t.Error("expected error when API key is empty")
	}
}
