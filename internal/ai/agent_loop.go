package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const defaultMaxTurns = 10

// forceToolInstruction 原生模式下首轮模型只回文本不调工具时追加一次，强制其发起工具调用。
const forceToolInstruction = "你必须通过工具调用完成用户请求：发起对工具的调用。不要只输出普通文本回答。"

// nativeProtocolOverride 在原生模式下插入，抵消系统提示词中的文本协议输出契约，
// 避免模型在两种工具调用约定之间摇摆。
const nativeProtocolOverride = "工具调用协议说明：本次会话已启用原生 function calling，请直接通过 API tools 机制发起工具调用；忽略系统提示词中「输出契约」关于 ```json tool_calls 输出格式的约定，其余规则不变。"

type toolLoopParams struct {
	messages   []Message
	onProgress ProgressCallback
	// userInput 原始用户输入（本地降级链参数提取需要）
	userInput string
	// localFallback 首轮无工具调用时是否尝试本地意图分类降级链（仅 Process）
	localFallback bool
	// useToolHints 多轮执行后是否注入工具提示（仅 Process）
	useToolHints bool
	// allowDirectAnswer 允许首轮无工具调用时把 LLM 文本直接作为回答
	// （多轮共享上下文的对话轮：追问/闲聊不需要工具）。Process 首轮路由
	// 保持"不确定"收口（防提示词失败的自由文本泄漏）。
	allowDirectAnswer bool
}

type toolLoopResult struct {
	messages []Message
	reply    string
}

func (a *Agent) effectiveMaxTurns() int {
	if a.config != nil && a.config.AI.MaxTurns > 0 {
		return a.config.AI.MaxTurns
	}
	return defaultMaxTurns
}

func (a *Agent) summarizeAfterTool() bool {
	if a.config != nil && a.config.AI.SummarizeAfterTool != nil {
		return *a.config.AI.SummarizeAfterTool
	}
	return true
}

// nativeToolsEnabled 决定是否走原生 function calling：
// off 永不；on/auto 时要求模型客户端实现 ToolCallingChatModel。
func (a *Agent) nativeToolsEnabled(chatModel ChatModel) bool {
	mode := "auto"
	if a.config != nil && a.config.AI.NativeTools != "" {
		mode = strings.ToLower(a.config.AI.NativeTools)
	}
	if mode == "off" || mode == "false" {
		return false
	}
	_, ok := chatModel.(ToolCallingChatModel)
	return ok
}

// runToolLoop 是工具生成阶段的统一循环：文本协议与原生 function calling 双模式，
// 工具结果回注后继续循环直到 LLM 输出总结文本或耗尽轮数。
// 首轮早退（直接返回工具原始输出）仅在 summarize_after_tool=false 时保留。
func (a *Agent) runToolLoop(ctx context.Context, chatModel ChatModel, p toolLoopParams) (toolLoopResult, error) {
	native := a.nativeToolsEnabled(chatModel)
	msgs := p.messages
	if native && len(msgs) > 0 && msgs[0].Role == "system" {
		msgs = append([]Message{msgs[0], {Role: "system", Content: nativeProtocolOverride}}, msgs[1:]...)
	}

	maxTurns := a.effectiveMaxTurns()
	summarize := a.summarizeAfterTool()
	var lastToolResult string
	forcedRetry := false

	for turn := 0; turn < maxTurns; turn++ {
		var content string
		var toolCalls []ToolCall

		if native {
			resp, err := a.generateToolsForLoop(ctx, chatModel, msgs, p.onProgress)
			if err != nil {
				if errors.Is(err, ErrToolsUnsupported) {
					debugPrint(a.debug, "provider 不支持原生 tools，本次会话降级文本协议")
					native = false
					turn--
					continue
				}
				return toolLoopResult{messages: msgs}, fmt.Errorf("AI 调用失败: %w", err)
			}
			content = resp.Content
			toolCalls = resp.ToolCalls
		} else {
			response, err := a.generateForLoop(ctx, chatModel, msgs, p.onProgress)
			if err != nil {
				return toolLoopResult{messages: msgs}, fmt.Errorf("AI 调用失败: %w", err)
			}
			content = response
			toolCalls = a.parseToolCalls(response)
		}

		debugPrint(a.debug, "=== 第 %d 轮 === 工具调用数: %d", turn+1, len(toolCalls))

		if len(toolCalls) == 0 {
			if turn == 0 && p.allowDirectAnswer && strings.TrimSpace(content) != "" {
				debugPrint(a.debug, "多轮对话轮：LLM 直接回答（未调用工具）")
				if p.onProgress != nil {
					p.onProgress("result", "完成")
				}
				return toolLoopResult{messages: msgs, reply: content}, nil
			}
			if turn > 0 {
				reply := strings.TrimSpace(content)
				if reply == "" && lastToolResult != "" {
					reply = lastToolResult
				}
				if p.onProgress != nil {
					p.onProgress("result", "完成")
				}
				return toolLoopResult{messages: msgs, reply: reply}, nil
			}

			// 首轮无工具调用
			if native && !forcedRetry {
				forcedRetry = true
				msgs = append(msgs, Message{Role: "system", Content: forceToolInstruction})
				turn--
				continue
			}
			if p.localFallback &&
				((len(content) > 100 && !strings.Contains(content, "tool_calls")) ||
					strings.Contains(content, "我不确定您要做什么")) {
				debugPrint(a.debug, "LLM 无法生成有效工具调用，尝试本地参数提取链")
				if reply, ok := a.localFallbackChain(ctx, p.userInput, p.onProgress, &msgs); ok {
					if p.onProgress != nil {
						p.onProgress("result", "完成")
					}
					return toolLoopResult{messages: msgs, reply: reply}, nil
				}
			}
			debugPrint(a.debug, "无有效工具调用，返回不确定（LLM 自由文本不透出）")
			return toolLoopResult{messages: msgs, reply: "我不确定您要做什么"}, nil
		}

		if p.onProgress != nil {
			p.onProgress("generate", toolCalls[0].Name)
		}

		// assistant 消息回注：原生模式携带结构化 tool_calls，文本模式存原始文本
		if native {
			assistantMsg := Message{Role: "assistant", Content: content}
			for _, call := range toolCalls {
				assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, MessageToolCall{
					ID: call.ID, Name: call.Name, ArgsJSON: call.ArgsJSON(),
				})
			}
			msgs = append(msgs, assistantMsg)
		} else {
			msgs = append(msgs, Message{Role: "assistant", Content: content})
		}

		var lastToolName string
		var toolResultStr string
		for _, call := range toolCalls {
			if p.onProgress != nil {
				p.onProgress("execute", call.Name)
			}
			if ok, question := a.confirmToolCall(call); !ok {
				if p.onProgress != nil {
					p.onProgress("result", "等待确认")
				}
				return toolLoopResult{messages: msgs, reply: question}, nil
			}
			result, err := a.executeToolCall(ctx, call)
			if err != nil {
				result = fmt.Sprintf("Tool execution failed: %v", err)
			}
			toolResultStr = result
			lastToolResult = result
			lastToolName = call.Name
			if native {
				msgs = append(msgs, Message{Role: "tool", ToolCallID: call.ID, Content: result})
			} else {
				msgs = append(msgs, Message{Role: "user", Content: fmt.Sprintf("\n\n[TOOL_CALL_RESULT]\n%s\n[/TOOL_CALL_RESULT]", result)})
			}
		}

		if turn > 0 && p.useToolHints && lastToolName != "" {
			if hint, ok := toolHints[lastToolName]; ok {
				msgs = append(msgs, Message{Role: "system", Content: "\n\n" + hint})
			}
		}

		// 兼容旧行为：关闭总结时首轮工具结果直接返回，不额外调用 LLM
		if !summarize && turn == 0 {
			debugPrint(a.debug, "首轮执行工具后直接返回结果（summarize_after_tool=false）")
			if p.onProgress != nil {
				p.onProgress("result", "完成")
			}
			return toolLoopResult{messages: msgs, reply: toolResultStr}, nil
		}
	}

	// 轮数耗尽：返回最后一个工具结果，避免空回复
	if p.onProgress != nil {
		p.onProgress("result", "完成")
	}
	return toolLoopResult{messages: msgs, reply: lastToolResult}, nil
}

// localFallbackChain 本地意图分类 + 参数提取的降级链（无 LLM 工具调用能力时兜底）。
// 返回 (回复, 是否已得出结论)。
func (a *Agent) localFallbackChain(ctx context.Context, userInput string, onProgress ProgressCallback, msgs *[]Message) (string, bool) {
	nodes := a.nodeMgr.List()
	nodeNames := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodeNames = append(nodeNames, n.Name)
	}

	classifier := NewIntentClassifier()
	intentResult := classifier.Classify(userInput)

	// 置信度阈值 20: 两个及以上关键词命中(如"列出节点")即视为有效意图,
	// 单关键词命中(置信度 10)仍拒绝,兼顾召回与误判。
	if intentResult.Type == IntentUncertain || intentResult.Confidence < 20 {
		debugPrint(a.debug, "本地分类器也无法确定")
		return "", false
	}

	extractor := NewParamExtractor(nodeNames)
	params := extractor.ExtractParams(intentResult.Type, userInput)

	validator := NewValidator()
	if err := validator.ValidateParams(intentResult.Type, params); err != nil {
		debugPrint(a.debug, "参数验证失败: %v", err)
		return "", false
	}

	debugPrint(a.debug, "使用本地参数提取成功: %v", params)

	var toolCallJSON string
	switch intentResult.Type {
	case IntentQueryNodes:
		toolCallJSON = a.buildToolCall("query_nodes", params)
	case IntentExecuteCmd:
		toolCallJSON = a.buildToolCall("execute_command", params)
	case IntentExecuteScript:
		toolCallJSON = a.buildToolCall("execute_script", params)
	case IntentGeneratePlaybook:
		toolCallJSON = a.buildToolCall("generate_playbook", params)
	case IntentTransferFile:
		toolCallJSON = a.buildToolCall("transfer_file", params)
	case IntentFileDownload:
		toolCallJSON = a.buildToolCall("file_download", params)
	case IntentAlertList:
		toolCallJSON = a.buildToolCall("alert_list", params)
	case IntentAlertRemedy:
		toolCallJSON = a.buildToolCall("alert_remedy", params)
	default:
		return "", false
	}

	if toolCallJSON == "" {
		return "", false
	}

	debugPrint(a.debug, "使用本地提取的工具调用")
	toolCalls := a.parseToolCalls(toolCallJSON)
	if len(toolCalls) == 0 {
		return "", false
	}
	if onProgress != nil {
		onProgress("generate", toolCalls[0].Name)
	}
	*msgs = append(*msgs, Message{Role: "assistant", Content: toolCallJSON})

	for _, call := range toolCalls {
		if onProgress != nil {
			onProgress("execute", call.Name)
		}
		if ok, question := a.confirmToolCall(call); !ok {
			if onProgress != nil {
				onProgress("result", "等待确认")
			}
			return question, true
		}
		result, err := a.executeToolCall(ctx, call)
		if err != nil {
			result = fmt.Sprintf("Tool execution failed: %v", err)
		}
		return result, true
	}
	return "", false
}

// generateToolsForLoop 原生 function calling 调用：优先流式实现，delta 经
// OnProgress("delta", ...) 转发给宿主。
func (a *Agent) generateToolsForLoop(ctx context.Context, chatModel ChatModel, msgs []Message, onProgress ProgressCallback) (*ModelResponse, error) {
	tools := a.registry.ToolDefinitions()
	if tsm, ok := chatModel.(ToolCallingStreamModel); ok {
		return tsm.GenerateToolsStream(ctx, msgs, tools, deltaForwarder(onProgress))
	}
	return chatModel.(ToolCallingChatModel).GenerateTools(ctx, msgs, tools)
}

// generateForLoop 文本协议调用：优先流式实现。
func (a *Agent) generateForLoop(ctx context.Context, chatModel ChatModel, msgs []Message, onProgress ProgressCallback) (string, error) {
	if ssm, ok := chatModel.(StreamingChatModel); ok {
		return ssm.GenerateStream(ctx, msgs, deltaForwarder(onProgress))
	}
	return generateWithRetry(ctx, chatModel, msgs, "AI调用")
}

func deltaForwarder(onProgress ProgressCallback) func(string) {
	if onProgress == nil {
		return nil
	}
	return func(delta string) {
		onProgress("delta", delta)
	}
}
