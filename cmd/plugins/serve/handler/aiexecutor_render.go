package handler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/model"
)

// mdCell escapes cell content so user-supplied values can't break the
// markdown table structure.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func formatLabelMap(labels map[string]string) string {
	if len(labels) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ", ")
}

// renderNodeRowsMarkdown renders structured node rows as a GFM table.
func renderNodeRowsMarkdown(rows []nodeRow) string {
	var sb strings.Builder
	sb.WriteString("| ID | Name | Address | User | Status | Groups | Labels |\n")
	sb.WriteString("|----|------|---------|------|--------|--------|--------|\n")
	for _, r := range rows {
		groups := strings.Join(r.Groups, ", ")
		if groups == "" {
			groups = "-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s:%d | %s | %s | %s | %s |\n",
			mdCell(r.ID), mdCell(r.Name), mdCell(r.Address), r.Port, mdCell(r.User), mdCell(r.Status),
			mdCell(groups), mdCell(formatLabelMap(r.Labels))))
	}
	return sb.String()
}

func renderPlaybooksMarkdown(pbs []*model.Playbook) string {
	var sb strings.Builder
	sb.WriteString("| ID | Name | Category | Tasks |\n")
	sb.WriteString("|----|------|----------|-------|\n")
	for _, pb := range pbs {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d |\n",
			mdCell(pb.ID), mdCell(pb.Name), mdCell(pb.Category), pb.TasksCount))
	}
	return sb.String()
}

func renderPlaybookInfoMarkdown(pb *model.Playbook) string {
	var sb strings.Builder
	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|-------|-------|\n")
	field := func(k, v string) {
		sb.WriteString(fmt.Sprintf("| %s | %s |\n", mdCell(k), mdCell(v)))
	}
	field("ID", pb.ID)
	field("Name", pb.Name)
	field("Description", pb.Description)
	field("Category", pb.Category)
	field("FilePath", pb.FilePath)
	field("Tasks", fmt.Sprintf("%d", pb.TasksCount))
	field("TaskNames", strings.Join(pb.TaskNames, ", "))
	return sb.String()
}
