package playbook

import (
	"fmt"
	"sort"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

func NewPlaybookTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: i18n.T("playbook.template.list.short"),
		Run:   runTemplateList,
	}
}

func runTemplateList(cmd *cobra.Command, args []string) {
	entries, err := pb.LoadTemplates("")
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s", i18n.T("playbook.template.list.err_load", err))
		return
	}

	fmt.Fprintln(cmd.OutOrStdout(), i18n.T("playbook.template.list.title"))
	fmt.Fprintln(cmd.OutOrStdout())

	builtinByCategory := groupByCategory(filterBySource(entries, "builtin"))
	userByCategory := groupByCategory(filterBySource(entries, "user"))

	fmt.Fprintln(cmd.OutOrStdout(), i18n.T("playbook.template.list.builtin"))
	if len(builtinByCategory) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), i18n.T("playbook.template.list.empty"))
	} else {
		printCategories(cmd, builtinByCategory)
	}

	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), i18n.T("playbook.template.list.user"))
	if len(userByCategory) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), i18n.T("playbook.template.list.empty"))
	} else {
		printCategories(cmd, userByCategory)
	}
}

func filterBySource(entries []*pb.TemplateEntry, source string) []*pb.TemplateEntry {
	var result []*pb.TemplateEntry
	for _, e := range entries {
		if e.Source == source {
			result = append(result, e)
		}
	}
	return result
}

func groupByCategory(entries []*pb.TemplateEntry) map[string][]*pb.TemplateEntry {
	groups := make(map[string][]*pb.TemplateEntry)
	for _, e := range entries {
		groups[e.Category] = append(groups[e.Category], e)
	}
	return groups
}

func printCategories(cmd *cobra.Command, groups map[string][]*pb.TemplateEntry) {
	categories := make([]string, 0, len(groups))
	for cat := range groups {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, cat := range categories {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s/\n", cat)
		entries := groups[cat]
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name < entries[j].Name
		})
		for _, e := range entries {
			desc := ""
			if e.Meta != nil {
				desc = e.Meta.Description
			}
			fmt.Fprintf(cmd.OutOrStdout(), "    • %s - %s\n", e.Name, desc)
		}
	}
}
