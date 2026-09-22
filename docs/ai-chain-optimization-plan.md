# AI 链路优化计划（opt/review-20260921 后续专项）

范围：`internal/ai/`（Agent/工具循环/LLM 客户端/会话存储）与 `cmd/plugins/serve/handler/ai*.go`（Web 装配）。
前置条件：**需要真实 API key 做基线验证**——所有 token/延迟/质量类改动必须与旧实现 A/B 对比后再合入；单测继续用 mock 保持全绿。

## P0 正确性前置（先于一切性能项）

1. **Web 多用户并发回调错挂**：`confirmGate`/`nodeContextHook` 是 Agent 上的单槽全局回调，
   serve 侧所有用户共享同一 Agent 装配，并发会话互相覆盖（agent.go:1096-1102 注释自认）。
   改法：回调注册表按 sessionID 分桶（`map[sessionID]callback` + 锁），Web 端注册时携带会话 ID；
   CLI 单会话路径不受影响。工作量 ~0.5 天，纯正确性，可先行。

## P1 连接与对象复用（低成本，无质量风险）

| 项 | 现状 | 改法 | 预期 |
|---|---|---|---|
| http.Client 复用 | 每条消息 `buildChatAgent` 新建 Agent+HTTPModel，每请求一次 TLS 握手（handler/ai.go:60-75） | 进程级共享 `*http.Client`（可按 baseURL 分桶），注入 HTTPModel | 每消息省 1 次握手；P95 延迟下降明显 |
| Agent/工具注册表缓存 | 37 个工具对象+registry 每请求重建 | 按 (userID,model,baseURL) LRU 缓存 Agent 骨架，会话态（消息）仍挂 Session | CPU 与 GC 压力下降 |
| 提示词预编译 | `formatPrompt` 每请求 `template.Parse`（agent.go:711-713） | 常量模板包级 `template.Must` 预编译 | 微优化 |

## P2 Token 与上下文治理（需 key 验证）

1. **消除记忆双重注入**：多轮路径既重发完整 `s.messages`（含未截断的 `[TOOL_CALL_RESULT]` 全文），
   又把 `buildMemory()` 摘要拼进 system（agent.go:1148-1196）——同一信息发两遍。
   方案：滚动窗口 + 摘要兜底。最近 N 轮（建议 8）保留全文，更早轮次替换为摘要行；
   工具结果全文只在"产生它的那一轮"保留，其后压为一行结论。
2. **消息历史上限**：`s.messages` 无上限累积（持久化才截 40 条）。设内存硬上限（如 24 条/8K tokens 估算），
   触顶触发 P2.1 的摘要压缩；`s.history` 同步截断。
3. **工具结果回注截断**：`runToolLoop` 把数千行输出原样塞回并逐轮重发（agent_loop.go:188-191）。
   按字符预算（如 4K）截断，保留头部+尾部（错误信息多在尾部），截断说明注入。
4. **NodeInfo 按需注入**：`getNodeInfo` 每请求把全部节点名按组拼进提示词（agent.go:732-764）。
   改为路由命中节点/执行类场景才注入全量，其余只注入分组与计数摘要。
5. **UTF-8 安全截断**：已在本轮修复（`common.Truncate` rune 安全），此条销项。

## P3 调用次数与成本（高价值，需 key 验证）

1. **路由+工具选择合并**：`Process` 先花一次完整 LLM 调用做路由（输出仅几个字符标签，
   却携带 RouterPrompt ~1.4KB + 记忆 + 输入），再进工具循环——固定 2+ 次往返。
   方案：原生 function calling 可用时单次调用完成（系统提示=各场景压缩描述；
   模型直接选工具；按所选工具推断场景注入对应场景补充提示），失败再降级现有两段式。
   目标：每请求 LLM 往返 2+ → 1，直接砍一半首字延迟与固定 token 开销。
2. **重试区分错误类别**：`generateWithRetry` 对 4xx（key 无效/参数错）也盲目重试 3 次
   （agent.go:692-709）；工具路径已正确区分（tools_protocol.go:136-138）。对齐即可。
3. **prompts 瘦身**：场景提示词内 Markdown 表格重复描述工具参数（与 `Parameters()` schema、
   `GetToolDescriptions` 三重重复，prompts.go 1746 行）。原生 FC 下同一参数描述发 3 遍；
   且文本协议"输出契约"与原生 FC 冲突靠运行时 `nativeProtocolOverride` 补丁抵消。
   方案：场景提示词只写场景差异，参数描述统一由 schema/工具目录生成，删除文本协议契约段。

## P4 观测与验证

- audit 表增加 prompt/completion token 字段与场景标签，为 P2/P3 提供 A/B 数据。
- 验证脚本：固定 10 条代表性问句（节点查询/执行/剧本/告警），对比改动前后
  （a）LLM 往返次数（b）总 token（c）首字延迟（d）结果正确性人工评分。

## 里程碑

| 阶段 | 内容 | 依赖 | 预估 |
|---|---|---|---|
| M1 | P0 回调分桶 + P1 全部 | 无 | 1-2 天 |
| M2 | P2.1/2.2/2.3（上下文治理） | 真实 key 基线 | 2-3 天 |
| M3 | P3.1 路由合并 + P3.3 prompts 瘦身 | 真实 key + M2 稳定 | 3-5 天 |
| M4 | P4 观测补全（与 M2 并行） | 无 | 0.5 天 |

注：原审查项「正则函数内编译」「formatPrompt 重复 Parse」中，前者随本地 NLP 降级链移除已消失，后者列入 P1。
