package nodes

import (
	"sort"
	"strconv"
	"strings"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
)

type Column struct {
	Key   string
	Label string
	Pref  int
}

var columnDefs = []Column{
	{"id", "ID", 20},
	{"name", "Name", 20},
	{"address", "Address", 24},
	{"port", "Port", 8},
	{"user", "User", 12},
	{"status", "Status", 10},
	{"groups", "Groups", 18},
	{"labels", "Labels", 24},
	{"last_check", "Last Check", 16},
}

var defaultColumnKeys = []string{"id", "name", "address", "status"}

func colByKey(key string) (Column, bool) {
	for _, c := range columnDefs {
		if c.Key == key {
			return c, true
		}
	}
	return Column{}, false
}

func cellValue(n *common.NodeInfo, key string) string {
	switch key {
	case "id":
		return n.ID
	case "name":
		return n.Name
	case "address":
		return n.Address
	case "port":
		return strconv.Itoa(n.Port)
	case "user":
		return n.User
	case "status":
		return n.Status
	case "groups":
		return strings.Join(n.Groups, ",")
	case "labels":
		return sortedLabels(n.Labels)
	case "last_check":
		return n.LastCheckAt
	}
	return ""
}

func sortedLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
	}
	return b.String()
}

func computeColumnWidths(cols []Column, avail int) []int {
	widths := make([]int, len(cols))
	total := 0
	for i, c := range cols {
		widths[i] = c.Pref
		total += c.Pref
	}
	for total > avail {
		maxI := 0
		for i := 1; i < len(widths); i++ {
			if widths[i] > widths[maxI] {
				maxI = i
			}
		}
		if widths[maxI] <= 6 {
			break
		}
		widths[maxI]--
		total--
	}
	return widths
}

func truncateCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := 0
	var out []rune
	for _, r := range s {
		rw := 1
		if r > 127 {
			rw = 2
		}
		if w+rw >= width {
			if w < width {
				out = append(out, '…')
			}
			break
		}
		w += rw
		out = append(out, r)
	}
	res := string(out)
	for common.DisplayWidth(res) < width {
		res += " "
	}
	return res
}
