package nodes

import (
	"strings"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type FilterQuery struct {
	Groups []string
	Labels map[string]string
	Search string
	Status string
}

func ParseFilterQuery(q string) FilterQuery {
	fq := FilterQuery{Labels: map[string]string{}}
	q = normalizeFullwidth(q)
	var search []string
	for _, raw := range strings.Fields(q) {
		// `&&` 是显式 AND 运算符:每个空格切出的 token 再按 `&&` 切分,语义与空格等价
		for _, tok := range strings.Split(raw, "&&") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			switch {
			case strings.HasPrefix(tok, "g:"):
				for _, g := range strings.Split(tok[2:], ",") {
					g = strings.TrimSpace(g)
					if g != "" {
						fq.Groups = append(fq.Groups, g)
					}
				}
			case strings.HasPrefix(tok, "l:"):
				for k, v := range parseLabels(tok[2:]) {
					fq.Labels[k] = v
				}
			case strings.HasPrefix(tok, "s:"):
				if v := strings.TrimSpace(tok[2:]); v != "" {
					fq.Status = v
				}
			default:
				if s := strings.TrimSpace(tok); s != "" {
					search = append(search, s)
				}
			}
		}
	}
	fq.Search = strings.Join(search, " ")
	return fq
}

func (fq FilterQuery) Empty() bool {
	return len(fq.Groups) == 0 && len(fq.Labels) == 0 && fq.Search == "" && fq.Status == ""
}

func matchFilter(n *common.NodeInfo, fq FilterQuery) bool {
	if fq.Empty() {
		return true
	}
	if len(fq.Groups) > 0 && !groupsIntersect(n.Groups, fq.Groups) {
		return false
	}
	for k, v := range fq.Labels {
		if n.Labels == nil || n.Labels[k] != v {
			return false
		}
	}
	if fq.Status != "" && !strings.EqualFold(n.Status, fq.Status) {
		return false
	}
	if fq.Search != "" {
		hay := strings.ToLower(n.ID + " " + n.Name + " " + n.Address)
		// 多个裸词 = AND 逐词匹配
		for _, term := range strings.Fields(fq.Search) {
			if !strings.Contains(hay, strings.ToLower(term)) {
				return false
			}
		}
	}
	return true
}

func applyFilter(nodes []*common.NodeInfo, fq FilterQuery) []*common.NodeInfo {
	out := make([]*common.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if matchFilter(n, fq) {
			out = append(out, n)
		}
	}
	return out
}

// normalizeFullwidth 将中文输入法全角模式下的常见符号归一化为半角:
// ＆→& (AND 运算符)、：→: (g:/l:/s: 前缀)、＝→= (label k=v)。
func normalizeFullwidth(q string) string {
	var b strings.Builder
	for _, r := range q {
		switch r {
		case '＆':
			b.WriteRune('&')
		case '：':
			b.WriteRune(':')
		case '＝':
			b.WriteRune('=')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func groupsIntersect(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
