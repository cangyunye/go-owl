package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

	aiPrompts "github.com/cangyunye/go-owl/internal/ai/prompts"
)

const defaultMaxTurns = 10

// forceToolInstruction 原生模式下首轮模型只回文本不调工具时追加一次，强制其发起工具调用。
const forceToolInstruction = "你必须通过工具调用完成用户请求：发起对工具的调用。不要只输出普通文本回答。"

// truncateMiddle 按字节预算截断 s，保留头尾（错误信息多在尾部），rune 安全。
func truncateMiddle(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	const markerReserve = 48 // 省略标记的最大字节预留
	marker := "\n…[中间省略 %d 字节]…\n"
	usable := max - markerReserve
	headLen := usable * 3 / 4
	tailLen := usable - headLen
	for headLen > 0 && !isRuneStart(s[headLen]) {
		headLen--
	}
	for tailLen > 0 && tailLen < len(s) && !isRuneStart(s[len(s)-tailLen]) {
		tailLen--
	}
	omitted := len(s) - headLen - tailLen
	m := strings.ReplaceAll(marker, "%d", itoa(omitted))
	return s[:headLen] + m + s[len(s)-tailLen:]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

func itoa(n int) string { return fmt.Sprintf("%d", n) }

// pagingFallbackNotice 兜底截断时附加到回复的显式告知（模型不可改写）。
const pagingFallbackNotice = "\n\n> ℹ️ 工具原始输出超出上下文预算,以上总结仅基于首尾片段;需要完整数据请缩小过滤条件。"

// defaultMaxResultPages 单个结果分页注入的默认页数上限。
const defaultMaxResultPages = 12

// directReturnTools 查询类只读工具:executor 已产出成品表格/文本,
// 首轮执行完直接把结果返回用户,不再发起总结用 LLM 调用。
// 注意与 confirmRequiredTools（写操作集合）不相交;alert_* 是链式工作流
// 工具（列表→方案→确认→执行）,直出会掐断流程,必须留在总结循环里。
var directReturnTools = map[string]struct{}{
	"query_nodes": {}, "query_database": {}, "node_status": {},
	"node_ping": {}, "node_check": {},
	"list_playbooks":         {},
	"playbook_template_list": {}, "playbook_template_info": {}, "playbook_template_export": {},
	"playbook_state_list": {}, "playbook_state_show": {},
	"validate_playbook": {},
	"history_list":      {}, "async_list": {}, "async_status": {},
	"settings_show": {},
}

// compactForContext 把表格类文本的连续空白压缩为单空格:列对齐填充只服务
// 人眼,进 LLM 上下文纯属体积浪费（实测 170B/行 → 69B/行）。
func compactForContext(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.Join(strings.Fields(l), " ")
	}
	return strings.Join(lines, "\n")
}

// splitResultPages 按行切页（行边界不截断），单行超页宽时硬切；
// 各页重组必须无损还原原文。
func splitResultPages(s string, pageSize int) []string {
	if pageSize <= 0 {
		return []string{s}
	}
	var pages []string
	var cur strings.Builder
	curLen := 0
	flush := func() {
		if cur.Len() > 0 {
			pages = append(pages, cur.String())
			cur.Reset()
			curLen = 0
		}
	}
	for _, line := range strings.Split(s, "\n") {
		lineLen := len(line) + 1
		if lineLen > pageSize {
			// 单行超页宽:硬切
			flush()
			for len(line) > 0 {
				n := pageSize
				if n > len(line) {
					n = len(line)
				}
				pages = append(pages, line[:n])
				line = line[n:]
			}
			continue
		}
		if curLen+lineLen > pageSize {
			flush()
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
		curLen += lineLen
	}
	flush()
	if len(pages) == 0 {
		pages = []string{s}
	}
	return pages
}

// trimTrailingSpaces 裁掉每行行尾空白:等宽表格的列填充只到最后一列有值处。
func trimTrailingSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

func (a *Agent) queryDirectReturnEnabled() bool {
	if a.config != nil && a.config.AI.QueryDirectReturn != nil {
		return *a.config.AI.QueryDirectReturn
	}
	return true
}

// contextSize 工具结果单页大小（字节），0 = 不限制（默认全量注入）。
func (a *Agent) contextSize() int {
	if a.config != nil && a.config.AI.ContextSize > 0 {
		return a.config.AI.ContextSize
	}
	return 0
}

func (a *Agent) maxResultPages() int {
	if a.config != nil && a.config.AI.MaxResultPages > 0 {
		return a.config.AI.MaxResultPages
	}
	return defaultMaxResultPages
}

// buildResultInjection 构造工具结果回注 LLM 的消息：
//   - 默认（未设 context_size）：全量注入，零截断；
//   - context_size>0 且结果超限：紧凑编码后按行切页，首页随本轮注入，
//     其余页排入待发队列逐轮发送（分页分析，数据不丢）；
//   - 页数超 max_result_pages：兜底退回首尾截断（truncated 置位，
//     终稿追加显式告知）。
func (a *Agent) buildResultInjection(call ToolCall, result string, native bool, pending *[]Message, truncated *bool) []Message {
	wrap := func(content string) Message {
		if native {
			return Message{Role: "tool", ToolCallID: call.ID, Content: content}
		}
		return Message{Role: "user", Content: fmt.Sprintf("\n\n[TOOL_CALL_RESULT]\n%s\n[/TOOL_CALL_RESULT]", content)}
	}

	pageSize := a.contextSize()
	if pageSize <= 0 || len(result) <= pageSize {
		return []Message{wrap(result)}
	}

	pages := splitResultPages(compactForContext(result), pageSize)
	if len(pages) > a.maxResultPages() {
		*truncated = true
		return []Message{wrap(truncateMiddle(result, pageSize))}
	}

	n := len(pages)
	first := pages[0] + fmt.Sprintf("\n[结果过大,已切分为 %d 页:当前第 1/%d 页;后续页随后发送,请逐页记录要点,收到全部页后再综合分析]", n, n)
	var out []Message
	out = append(out, wrap(first))
	for i := 1; i < n; i++ {
		out = append(out, Message{Role: "user", Content: fmt.Sprintf("[工具结果 第 %d/%d 页]\n%s", i+1, n, pages[i])})
	}
	// 除首页外排入待发队列，每轮模型确认后注入一页
	*pending = append(*pending, out[1:]...)
	return out[:1]
}

func allCallsDirect(calls []ToolCall) bool {
	if len(calls) == 0 {
		return false
	}
	for _, c := range calls {
		if _, ok := directReturnTools[c.Name]; !ok {
			return false
		}
	}
	return true
}

// toolCallGuidance 模型未能产出工具调用时的统一指引：
// 绝不做本地字符串猜测后执行真实命令（曾发生静默落到第一个节点）。
const toolCallGuidance = "我无法将您的请求可靠地映射到可执行的运维工具，为避免误执行已中止。" +
	"请换一种更明确的说法（例如「在 node-1 上查看磁盘使用率」），" +
	"或直接使用 owl exec / owl playbook 等命令完成操作。"

// nativeProtocolOverride 在原生模式下插入，抵消系统提示词中的文本协议输出契约，
// 避免模型在两种工具调用约定之间摇摆。
const nativeProtocolOverride = "工具调用协议说明：本次会话已启用原生 function calling，请直接通过 API tools 机制发起工具调用；忽略系统提示词中「输出契约」关于 ```json tool_calls 输出格式的约定，其余规则不变。"

type toolLoopParams struct {
	messages   []Message
	onProgress ProgressCallback
	// userInput 原始用户输入（本地降级链参数提取需要）
	userInput string
	// useToolHints 多轮执行后是否注入工具提示（仅 Process）
	useToolHints bool
	// allowDirectAnswer 允许首轮无工具调用时把 LLM 文本直接作为回答
	// （多轮共享上下文的对话轮：追问/闲聊不需要工具）。Process 首轮路由
	// 保持"不确定"收口（防提示词失败的自由文本泄漏）。
	allowDirectAnswer bool
	// noDowngrade 合并路由模式使用：provider 不支持原生 FC 时不在循环内
	// 降级文本协议，而是把 ErrToolsUnsupported 抛出，由调用方整体回退
	// 两段式路由（保留既有降级契约）。
	noDowngrade bool
}

type toolLoopResult struct {
	messages []Message
	reply    string
	// usedTools 本次循环是否实际发起过工具调用（确认拦截也算发起）
	usedTools bool
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

// mergedRoutingDirective 合并模式在通用工具目录之上补充的场景选择指引。
const mergedRoutingDirective = "\n\n## 场景与工具选择\n" +
	"直接根据用户请求选择并调用最合适的工具完成任务（无需先输出场景标签）。" +
	"纯闲聊、寒暄或概念性提问可直接用文本回答，不要调用工具。"

// processMergedRouting P3.1：原生 function calling 模型把「场景路由」与
// 「工具选择」合并为一次 LLM 调用——路由输出的只是几个字符的标签，
// 原先却要独立消耗一次携带完整路由提示词的往返。
// 返回 handled=false 表示合并模式未能得出结论（无工具且无直接回答），
// 调用方回退两段式路由。
func (a *Agent) processMergedRouting(ctx context.Context, chatModel ChatModel, userInput, sessionMemory string, onProgress ProgressCallback) (string, bool, error) {
	messages := []Message{
		{Role: "system", Content: a.RenderSystemPrompt(aiPrompts.GenericToolSystemPrompt) + mergedRoutingDirective},
	}
	if sessionMemory != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: "以下是此前会话的对话与操作记录，仅作参考背景，不要把它当作新的用户请求：\n" + sessionMemory,
		})
	}
	messages = append(messages, Message{Role: "user", Content: userInput})

	result, err := a.runToolLoop(ctx, chatModel, toolLoopParams{
		messages:          messages,
		onProgress:        onProgress,
		userInput:         userInput,
		useToolHints:      true,
		allowDirectAnswer: true,
		noDowngrade:       true,
	})
	if err != nil {
		if errors.Is(err, ErrToolsUnsupported) {
			return "", false, nil // provider 不支持原生 FC：整体回退两段式
		}
		return "", false, err
	}
	if !result.usedTools && strings.TrimSpace(result.reply) == toolCallGuidance {
		return "", false, nil
	}
	return result.reply, true, nil
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
	// 分页注入状态：pendingPages 为待逐轮发送的后续页；pageTurns 独立于
	// maxTurns 计数（分页轮不挤占工具轮预算）；truncatedOversize 标记
	// 兜底截断发生，终稿需追加显式告知。
	var pendingPages []Message
	pageTurns := 0
	truncatedOversize := false

	for turn := 0; turn < maxTurns+pageTurns; turn++ {
		var content string
		var toolCalls []ToolCall

		if native {
			resp, err := a.generateToolsForLoop(ctx, chatModel, msgs, p.onProgress)
			if err != nil {
				if errors.Is(err, ErrToolsUnsupported) {
					if p.noDowngrade {
						return toolLoopResult{}, err
					}
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
				return toolLoopResult{messages: msgs, reply: content}, nil // 直接回答：无工具
			}
			if turn > 0 {
				// 分页推进优先于终稿判定：还有未发送的页，模型的页间确认
				// 不作为终稿，注入下一页继续。
				if len(pendingPages) > 0 {
					if strings.TrimSpace(content) != "" {
						msgs = append(msgs, Message{Role: "assistant", Content: content})
					}
					msgs = append(msgs, pendingPages[0])
					pendingPages = pendingPages[1:]
					pageTurns++
					if len(pendingPages) == 0 {
						msgs = append(msgs, Message{Role: "user", Content: "[全部页已发送完毕] 请基于以上全部页的内容给出综合分析结论。"})
					}
					continue
				}
				reply := strings.TrimSpace(content)
				if reply == "" && lastToolResult != "" {
					reply = lastToolResult
				}
				if truncatedOversize {
					reply += pagingFallbackNotice
				}
				if p.onProgress != nil {
					p.onProgress("result", "完成")
				}
				return toolLoopResult{messages: msgs, reply: reply, usedTools: true}, nil
			}

			// 首轮无工具调用
			if native && !forcedRetry {
				forcedRetry = true
				msgs = append(msgs, Message{Role: "system", Content: forceToolInstruction})
				turn--
				continue
			}
			debugPrint(a.debug, "无有效工具调用，返回指引（不做本地猜测执行）")
			return toolLoopResult{messages: msgs, reply: toolCallGuidance}, nil
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
		var results []string
		for _, call := range toolCalls {
			if p.onProgress != nil {
				p.onProgress("execute", call.Name)
			}
			if ok, question := a.confirmToolCall(call, resolveGate(ctx, a)); !ok {
				if p.onProgress != nil {
					p.onProgress("result", "等待确认")
				}
				return toolLoopResult{messages: msgs, reply: question, usedTools: true}, nil
			}
			result, err := a.executeToolCall(ctx, call)
			if err != nil {
				result = fmt.Sprintf("Tool execution failed: %v", err)
			}
			results = append(results, result)
			toolResultStr = result
			lastToolResult = result
			lastToolName = call.Name
			// 回注：默认全量注入（零截断）；context_size 显式设置且超限时分页，
			// 页数超上限才兜底截断。
			msgs = append(msgs, a.buildResultInjection(call, result, native, &pendingPages, &truncatedOversize)...)
		}

		// 查询类只读工具首轮直出：结果直接返回用户，省一次总结用 LLM 调用；
		// 结果超单页进入分页分析时（pendingPages 非空）不直出。
		if turn == 0 && len(pendingPages) == 0 && a.queryDirectReturnEnabled() && allCallsDirect(toolCalls) {
			debugPrint(a.debug, "查询类工具首轮直出（%d 个结果，未发起总结调用）", len(results))
			if p.onProgress != nil {
				p.onProgress("result", "完成")
			}
			return toolLoopResult{messages: msgs, reply: strings.Join(results, "\n\n"), usedTools: true}, nil
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
			return toolLoopResult{messages: msgs, reply: toolResultStr, usedTools: true}, nil
		}
	}

	// 轮数耗尽：返回最后一个工具结果，避免空回复
	if p.onProgress != nil {
		p.onProgress("result", "完成")
	}
	return toolLoopResult{messages: msgs, reply: lastToolResult, usedTools: true}, nil
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
