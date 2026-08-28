package monitor

import (
	"context"
	"testing"

	"github.com/cangyunye/go-owl/internal/ai"
	"github.com/stretchr/testify/require"
)

// fakeLLM 可控 LLM 客户端。
type fakeLLM struct {
	reply string
	err   error
	got   []string // 收到的 user 消息
}

func (f *fakeLLM) Generate(ctx context.Context, messages []ai.Message) (string, error) {
	for _, m := range messages {
		if m.Role == "user" {
			f.got = append(f.got, m.Content)
		}
	}
	return f.reply, f.err
}

// TestAIAdvisor_PickExisting 验证引用现有对策时内容以库为准。
func TestAIAdvisor_PickExisting(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.UpsertRemedy(Remedy{
		ID: "RM-1", AlertTypeID: "OWL-DSK-001", Name: "清理临时文件",
		Kind: "script", Content: "find /tmp -type f -mtime +7 -delete",
		Risk: "low", Source: "user", Reviewed: true,
	}))

	llm := &fakeLLM{reply: `{
	  "reasoning": "磁盘使用率过高，清理临时文件",
	  "steps": [{"remedy_id": "RM-1", "name": "尝试改写", "kind": "script",
	    "content": "echo evil", "risk": "high", "generated": false}]
	}`}
	a := NewAIAdvisor(llm, s, 3)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
		Remedies: []Remedy{{
			ID: "RM-1", AlertTypeID: "OWL-DSK-001", Name: "清理临时文件",
			Kind: "script", Content: "find /tmp -type f -mtime +7 -delete",
			Risk: "low", Source: "user", Reviewed: true,
		}},
	})
	require.NoError(t, err)
	require.Len(t, plan.Steps, 1)
	st := plan.Steps[0]
	require.Equal(t, "RM-1", st.RemedyID)
	require.Equal(t, "清理临时文件", st.Name, "现有对策名称以库为准")
	require.Equal(t, "find /tmp -type f -mtime +7 -delete", st.Content, "内容以库为准，不信任 LLM 改写")
	require.Equal(t, "low", st.Risk, "风险以库为准")
	require.Equal(t, "user", st.Source)
	require.False(t, st.Generated)
	require.NotEmpty(t, llm.got, "应包含告警上下文")
	require.Contains(t, llm.got[0], "OWL-DSK-001")
}

// TestAIAdvisor_GenerateScript 验证 AI 现场生成脚本：标记 AI 来源 + 待审核 + 语法闸门。
func TestAIAdvisor_GenerateScript(t *testing.T) {
	s := newTestStore(t)
	llm := &fakeLLM{reply: `{
	  "reasoning": "日志高频错误，清理并检查",
	  "steps": [{"name": "清理过期日志", "kind": "script",
	    "content": "journalctl --vacuum-time=7d", "risk": "low", "generated": true}]
	}`}
	a := NewAIAdvisor(llm, s, 3)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-ERR-001", NodeID: "n1"},
	})
	require.NoError(t, err)
	st := plan.Steps[0]
	require.True(t, st.Generated, "应为生成项")
	require.Equal(t, "ai", st.Source)
	require.False(t, st.Reviewed, "AI 生成默认待审核")
	require.Equal(t, "journalctl --vacuum-time=7d", st.Content)

	// 生成脚本必须过语法闸门
	ok, reason := NewSyntaxGate().Validate("script", st.Content)
	require.True(t, ok, "AI 生成脚本应语法正确: %s", reason)
}

// TestAIAdvisor_InvalidRisk 验证 LLM 返回未知风险时保守取中。
func TestAIAdvisor_InvalidRisk(t *testing.T) {
	s := newTestStore(t)
	llm := &fakeLLM{reply: `{"reasoning": "x", "steps": [
	  {"name": "脚本", "kind": "script", "content": "echo hi", "risk": "unknown", "generated": true}]}`}
	a := NewAIAdvisor(llm, s, 3)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
	})
	require.NoError(t, err)
	require.Equal(t, "medium", plan.Steps[0].Risk, "未知风险保守取中")
}

// TestAIAdvisor_FallbackOnError 验证 LLM 失败/坏 JSON 时降级规则兜底。
func TestAIAdvisor_FallbackOnError(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.UpsertRemedy(Remedy{
		ID: "RM-1", AlertTypeID: "OWL-DSK-001", Name: "清理",
		Kind: "script", Content: "echo hi", Risk: "low",
		Source: "user", Reviewed: true,
	}))

	// LLM 返回错误 → 兜底
	a := NewAIAdvisor(&fakeLLM{err: context.DeadlineExceeded}, s, 3)
	plan, err := a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, plan.Steps, "LLM 失败应降级规则推荐")
	require.Equal(t, "RM-1", plan.Steps[0].RemedyID)

	// LLM 返回非法 JSON → 兜底
	a = NewAIAdvisor(&fakeLLM{reply: "not json at all"}, s, 3)
	plan, err = a.Advise(context.Background(), DisposalRequest{
		Alert: &Alert{ID: "AL-1", AlertTypeID: "OWL-DSK-001", NodeID: "n1"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, plan.Steps, "坏 JSON 应降级规则推荐")
}
