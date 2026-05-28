# Checklist

- [x] RouterPrompt 包含 owl 功能范围声明（owl 是分布式节点管理运维工具）
- [x] RouterPrompt 声明了 4 个命令组标签及各自含义
- [x] RouterPrompt 明确无关查询输出 `uncertain`
- [x] RouterPrompt 列举了典型超出范围场景（MAC 地址、macOS 操作指南）
- [x] NodeSystemPrompt 开头包含 owl 范围界定段落
- [x] ExecSystemPrompt 开头包含 owl 范围界定段落
- [x] FileSystemPrompt 开头包含 owl 范围界定段落
- [x] PlaybookSystemPrompt 开头包含 owl 范围界定段落
- [x] 每个领域提示词的范围界定段落声明了"遇到无关问题必须回复'我不确定您要做什么'"
- [x] NodeSystemPrompt 的范围界定明确指出"mac"在 owl 语境下是节点名称关键字
- [x] Agent.Process() 在 parseToolCalls 返回空时检查响应是否为非工具调用长文本
- [x] 安全网判断逻辑：响应长度 > 100 字符 且不包含 tool_calls 关键字 → 判定为无效
- [x] 无效回复被拦截时返回受控的拒绝消息"我不确定您要做什么"
- [x] "我不确定您要做什么"等合法拒绝响应正常透传
- [x] 代码编译通过（`go build ./...`）
- [x] 现有测试通过（如存在测试套件）
