package nodes

import (
	"strings"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type FilterQuery struct {
	Groups []string
	Labels map[string]string
	Search string
}

func ParseFilterQuery(q string) FilterQuery {
	fq := FilterQuery{Labels: map[string]string{}}
	var search []string
	for _, tok := range strings.Fields(q) {
		switch {
		case strings.HasPrefix(tok, "g:"):
			for _, g := range strings.Split(tok[2:], ",") {
				g = strings.TrimSpace(g)
				if g != "" {
					fq.Groups = append(fq.Groups, g)
				}
			}
		case strings.HasPrefix(tok, "l:"):
			for _, pair := range strings.Split(tok[2:], ",") {
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) == 2 {
					k := strings.TrimSpace(parts[0])
					v := strings.TrimSpace(parts[1])
					if k != "" {
						fq.Labels[k] = v
					}
				}
			}
		default:
			if s := strings.TrimSpace(tok); s != "" {
				search = append(search, s)
			}
		}
	}
	fq.Search = strings.Join(search, " ")
	return fq
}

func (fq FilterQuery) Empty() bool {
	return len(fq.Groups) == 0 && len(fq.Labels) == 0 && fq.Search == ""
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
