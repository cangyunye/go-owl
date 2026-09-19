package exec

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

// candidate 一条补全候选: value=回填文本, desc=菜单辅助说明。
type candidate struct {
	value string
	desc  string
}

// tokenAt 定位光标所在逗号分段的 rune 区间 [start, end)。
// 光标落在分隔符上时视为右侧新段(空段)。
func tokenAt(value string, pos int) (start, end int) {
	r := []rune(value)
	if pos < 0 {
		pos = 0
	}
	if pos > len(r) {
		pos = len(r)
	}
	start = 0
	for i := pos - 1; i >= 0; i-- {
		if r[i] == ',' {
			start = i + 1
			break
		}
	}
	end = len(r)
	for i := pos; i < len(r); i++ {
		if r[i] == ',' {
			end = i
			break
		}
	}
	return start, end
}

// completionMenu 补全候选菜单的纯逻辑状态机(无 IO,可独立单测)。
// sync 传入全量候选与当前 token 前缀;query 未变时保留选中项,
// 变化时重置到第一条——与 CLI Editor 的 lastQuery 防抖同语义。
type completionMenu struct {
	all    []candidate
	items  []candidate
	query  string
	active int
}

func newCompletionMenu(all []candidate) *completionMenu {
	m := &completionMenu{}
	m.sync(all, "")
	return m
}

// sync 更新全量候选与过滤词;query 未变时不重置 active。
func (m *completionMenu) sync(all []candidate, query string) {
	m.all = all
	if query != m.query {
		m.query = query
		m.active = 0
	}
	m.rebuild()
}

func (m *completionMenu) rebuild() {
	q := strings.ToLower(m.query)
	m.items = m.items[:0]
	for _, c := range m.all {
		if q == "" || strings.HasPrefix(strings.ToLower(c.value), q) {
			m.items = append(m.items, c)
		}
	}
	if m.active >= len(m.items) {
		m.active = 0
	}
}

func (m *completionMenu) Len() int { return len(m.items) }

func (m *completionMenu) Active() int { return m.active }

func (m *completionMenu) Visible() []candidate { return m.items }

func (m *completionMenu) MoveDown() {
	if n := m.Len(); n > 0 {
		m.active = (m.active + 1) % n
	}
}

func (m *completionMenu) MoveUp() {
	if n := m.Len(); n > 0 {
		m.active = (m.active - 1 + n) % n
	}
}

func (m *completionMenu) Selected() (candidate, bool) {
	if m.active < 0 || m.active >= len(m.items) {
		return candidate{}, false
	}
	return m.items[m.active], true
}

// nodeCandidates 节点候选: 插入 ID(resolveTargets 只认 ID),desc 带名称与状态。
func nodeCandidates(nodes []*common.NodeInfo) []candidate {
	out := make([]candidate, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		status := "○离线"
		if n.Status == "online" {
			status = "●在线"
		}
		desc := status
		if n.Name != "" {
			desc = n.Name + " · " + status
		}
		out = append(out, candidate{value: n.ID, desc: desc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].value < out[j].value })
	return out
}

// groupCandidates 分组候选: 组名并集,desc 为该组节点数。
func groupCandidates(nodes []*common.NodeInfo) []candidate {
	count := map[string]int{}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		for _, g := range n.Groups {
			count[g]++
		}
	}
	out := make([]candidate, 0, len(count))
	for g, c := range count {
		out = append(out, candidate{value: g, desc: fmt.Sprintf("%d 台", c)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].value < out[j].value })
	return out
}

// labelCandidates 标签候选: 裸 key(存在性语义)与全部 k=v 对并集,desc 为匹配节点数。
func labelCandidates(nodes []*common.NodeInfo) []candidate {
	count := map[string]int{}
	add := func(cond string, n int) {
		if _, ok := count[cond]; !ok {
			count[cond] = 0
		}
		count[cond] += n
	}
	for _, n := range nodes {
		if n == nil {
			continue
		}
		for k, v := range n.Labels {
			add(k, 1)
			add(k+"="+v, 1)
		}
	}
	out := make([]candidate, 0, len(count))
	for cond, c := range count {
		out = append(out, candidate{value: cond, desc: fmt.Sprintf("%d 台", c)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].value < out[j].value })
	return out
}
