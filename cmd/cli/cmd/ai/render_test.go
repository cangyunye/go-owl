package ai

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// markdown 样本:表格 + 粗体 + 代码块,覆盖 glamour 渲染路径
const mdSample = "## 节点磁盘使用率\n\n" +
	"| 节点 | 使用率 | 状态 |\n" +
	"|------|--------|------|\n" +
	"| web-01 | 87% | WARN |\n" +
	"| db-01 | 45% | OK |\n\n" +
	"建议对 **web-01** 执行清理:\n\n" +
	"```bash\ndf -h /data\n```\n"

// 工具原始 ASCII 表格:绝不能被段落重排破坏
const plainTable = "ID         Name       Address          Status   Groups\n" +
	"--------------------------------------------------------\n" +
	"web-01     web-nginx  10.0.1.11:22     offline  web,prod\n" +
	"db-01      db-mysql   10.0.2.11:22     online   db\n" +
	"总计: 2 个节点\n"

func TestRenderReply_NonTTY_Passthrough(t *testing.T) {
	if got := renderReply(mdSample, false); got != mdSample {
		t.Fatalf("非 TTY 必须原样返回(管道/tee 场景), got %q", got)
	}
	if got := renderReply(plainTable, false); got != plainTable {
		t.Fatalf("非 TTY 纯文本被改写, got %q", got)
	}
}

func TestRenderReply_MarkdownRendered(t *testing.T) {
	got := renderReply(mdSample, true)
	if got == mdSample {
		t.Fatal("markdown 在 TTY 下应被渲染, 不能原样返回")
	}
	if strings.Contains(got, "|------|") {
		t.Fatalf("markdown 表格分隔行不应保留原文形态, got:\n%s", got)
	}
	for _, want := range []string{"web-01", "db-01", "87%"} {
		if !strings.Contains(got, want) {
			t.Fatalf("渲染后丢失内容 %q, got:\n%s", want, got)
		}
	}
	if !utf8.ValidString(got) {
		t.Fatal("渲染输出不是合法 UTF-8")
	}
}

func TestRenderReply_PlainTablePassthrough(t *testing.T) {
	if got := renderReply(plainTable, true); got != plainTable {
		t.Fatalf("无 markdown 特征的 ASCII 表格必须直通, got %q", got)
	}
}

// LLM 常把工具原始 ASCII 表格直接贴进 markdown 回复（加个 ## 标题）。
// 整体进 glamour 会把等宽表格按段落重排拆行，必须整体直通。
func TestRenderReply_MixedHeadingPlusPlainTable(t *testing.T) {
	mixed := "## 节点磁盘使用率查询结果\n\n" + plainTable + "\n\n以上为原始查询结果。\n"
	if got := renderReply(mixed, true); got != mixed {
		t.Fatalf("markdown+ASCII 表格混排必须整体直通（保护表格对齐）, got %q", got)
	}
}

func TestRenderReply_PlainSentencePassthrough(t *testing.T) {
	s := "已列出 50 个节点,其中 12 个在线。"
	if got := renderReply(s, true); got != s {
		t.Fatalf("普通句子应直通, got %q", got)
	}
}

func TestRenderReply_UTF8NotCut(t *testing.T) {
	got := renderReply(mdSample, true)
	// CJK 内容在折行后仍完整可读
	if !strings.Contains(got, "节点磁盘使用率") && !strings.Contains(got, "节点") {
		t.Fatalf("中文标题丢失, got:\n%s", got)
	}
}

func TestLooksLikeMarkdown(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"代码块", "```bash\ndf -h\n```", true},
		{"表格分隔", "| a | b |\n|---|---|", true},
		{"标题", "## 总结", true},
		{"粗体", "这是**重点**内容", true},
		{"列表", "第一步:\n- 检查磁盘\n- 清理日志", true},
		{"ASCII表格", plainTable, false},
		{"普通句子", "节点全部在线。", false},
		{"空串", "", false},
	}
	for _, c := range cases {
		if got := looksLikeMarkdown(c.in); got != c.want {
			t.Errorf("%s: looksLikeMarkdown=%v, want %v", c.name, got, c.want)
		}
	}
}
