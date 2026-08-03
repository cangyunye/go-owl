package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/common"
	"github.com/cangyunye/go-owl/internal/i18n"
)

type importOptions struct {
	filePath     string
	overwrite    bool
	skipExisting bool
	dryRun       bool
	outputFormat string
	template     bool
	filterNodes  []string
	filterGroups []string
	filterLabels []string
}

func NewImportCmd() *cobra.Command {
	opts := &importOptions{}

	importCmd := &cobra.Command{
		Use:   "import",
		Short: i18n.T("node.import.short"),
		Long:  i18n.T("node.import.long"),
		Run: func(cmd *cobra.Command, args []string) {
			if opts.template {
				generateTemplate(opts.outputFormat)
				return
			}

			if opts.filePath == "" {
				fmt.Fprintln(os.Stderr, i18n.T("node.import.err_no_file"))
				os.Exit(1)
			}

			importNodes(opts)
		},
	}

	importCmd.Flags().StringVarP(&opts.filePath, "file", "f", "",
		i18n.T("node.import.flag_file"))
	importCmd.Flags().BoolVarP(&opts.overwrite, "overwrite", "O", false,
		i18n.T("node.import.flag_overwrite"))
	importCmd.Flags().BoolVar(&opts.skipExisting, "skip-existing", false,
		i18n.T("node.import.flag_skip_existing"))
	importCmd.Flags().BoolVar(&opts.dryRun, "dry-run", false,
		i18n.T("node.import.flag_dry_run"))
	importCmd.Flags().BoolVar(&opts.template, "template", false,
		i18n.T("node.import.flag_template"))
	importCmd.Flags().StringVarP(&opts.outputFormat, "format", "o", "yaml",
		i18n.T("node.import.flag_format"))

	return importCmd
}

func NewExportCmd() *cobra.Command {
	opts := &importOptions{}

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: i18n.T("node.export.short"),
		Long:  i18n.T("node.export.long"),
		Run: func(cmd *cobra.Command, args []string) {
			exportNodes(opts)
		},
	}

	exportCmd.Flags().StringVarP(&opts.filePath, "file", "f", "",
		i18n.T("node.export.flag_file"))
	exportCmd.Flags().StringVarP(&opts.outputFormat, "format", "o", "yaml",
		i18n.T("node.export.flag_format"))
	exportCmd.Flags().StringSliceVar(&opts.filterNodes, "nodes", nil,
		i18n.T("node.export.flag_nodes"))
	exportCmd.Flags().StringSliceVar(&opts.filterGroups, "groups", nil,
		i18n.T("node.export.flag_groups"))
	exportCmd.Flags().StringSliceVar(&opts.filterLabels, "labels", nil,
		i18n.T("node.export.flag_labels"))

	return exportCmd
}

type nodeFile struct {
	Version string             `json:"version" yaml:"version"`
	Nodes   []*common.NodeInfo `json:"nodes" yaml:"nodes"`
}

func importNodes(opts *importOptions) {
	data, err := os.ReadFile(opts.filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.import.err_read", err))
		os.Exit(1)
	}

	var nf nodeFile
	ext := strings.ToLower(filepath.Ext(opts.filePath))

	if ext == ".json" {
		if err := json.Unmarshal(data, &nf); err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("node.import.err_json", err))
			os.Exit(1)
		}
	} else {
		if err := yaml.Unmarshal(data, &nf); err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("node.import.err_yaml", err))
			os.Exit(1)
		}
	}

	store := common.GetNodeStore()

	success := 0
	failed := 0
	skipped := 0

	for _, node := range nf.Nodes {
		if node.ID == "" {
			fmt.Println(i18n.T("node.import.skip_id"))
			failed++
			continue
		}

		if node.Name == "" {
			fmt.Printf("%s\n", i18n.T("node.import.skip_name", node.ID))
			failed++
			continue
		}

		if node.Address == "" {
			fmt.Printf("%s\n", i18n.T("node.import.skip_address", node.ID))
			failed++
			continue
		}

		_, err := store.Get(node.ID)
		nodeExists := err == nil

		if nodeExists && !opts.overwrite && !opts.skipExisting {
			fmt.Printf("%s\n", i18n.T("node.import.skip_exists", node.ID))
			skipped++
			continue
		}

		if nodeExists && opts.skipExisting {
			skipped++
			continue
		}

		if opts.dryRun {
			fmt.Printf("%s\n", i18n.T("node.import.preview", node.ID, node.Name, node.Address, i18n.F(node.Port)))
			success++
			continue
		}

		now := time.Now().Format(time.RFC3339)
		node.CreatedAt = now
		node.UpdatedAt = now

		if nodeExists {
			if err := store.Update(node); err != nil {
				fmt.Printf("%s\n", i18n.T("node.import.err_update", node.ID, err))
				failed++
			} else {
				fmt.Printf("%s\n", i18n.T("node.import.ok_update", node.ID))
				success++
			}
		} else {
			if err := store.Add(node); err != nil {
				fmt.Printf("%s\n", i18n.T("node.import.err_add", node.ID, err))
				failed++
			} else {
				fmt.Printf("%s\n", i18n.T("node.import.ok_add", node.ID))
				success++
			}
		}
	}

	if !opts.dryRun && success > 0 {
		store.Save()
	}

	fmt.Printf("%s\n", i18n.T("node.import.result", i18n.F(success), i18n.F(skipped), i18n.F(failed)))
	if failed > 0 {
		os.Exit(1)
	}
}

func exportNodes(opts *importOptions) {
	store := common.GetNodeStore()
	allNodes, err := store.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.import.err_list", err))
		os.Exit(1)
	}

	nodes := exportFilterNodes(allNodes, opts)

	if len(nodes) == 0 {
		fmt.Println(i18n.T("node.import.no_match"))
		return
	}

	nf := nodeFile{
		Version: "1.0",
		Nodes:   nodes,
	}

	var data []byte
	var err2 error

	if opts.outputFormat == "json" {
		data, err2 = json.MarshalIndent(nf, "", "  ")
	} else {
		data, err2 = yaml.Marshal(nf)
	}

	if err2 != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.import.err_serialize", err2))
		os.Exit(1)
	}

	if opts.filePath != "" {
		if err := os.WriteFile(opts.filePath, data, 0644); err != nil {
			fmt.Fprintln(os.Stderr, i18n.T("node.import.err_write", err))
			os.Exit(1)
		}
		fmt.Printf("%s\n", i18n.T("node.import.ok_exported", i18n.F(len(nodes)), opts.filePath))
	} else {
		fmt.Println(string(data))
	}
}

func exportFilterNodes(nodes []*common.NodeInfo, opts *importOptions) []*common.NodeInfo {
	if len(opts.filterNodes) == 0 && len(opts.filterGroups) == 0 && len(opts.filterLabels) == 0 {
		return nodes
	}

	filterNodeSet := make(map[string]bool)
	for _, n := range opts.filterNodes {
		filterNodeSet[n] = true
	}

	filterGroupSet := make(map[string]bool)
	for _, g := range opts.filterGroups {
		filterGroupSet[g] = true
	}

	filterLabelMap := make(map[string]string)
	for _, l := range opts.filterLabels {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			filterLabelMap[parts[0]] = parts[1]
		}
	}

	var filtered []*common.NodeInfo
	for _, node := range nodes {
		if len(filterNodeSet) > 0 {
			if !filterNodeSet[node.ID] {
				continue
			}
		}

		if len(filterGroupSet) > 0 {
			hasGroup := false
			for _, g := range node.Groups {
				if filterGroupSet[g] {
					hasGroup = true
					break
				}
			}
			if !hasGroup {
				continue
			}
		}

		if len(filterLabelMap) > 0 {
			hasLabel := true
			for k, v := range filterLabelMap {
				if nodeVal, ok := node.Labels[k]; !ok || (v != "" && nodeVal != v) {
					hasLabel = false
					break
				}
			}
			if !hasLabel {
				continue
			}
		}

		filtered = append(filtered, node)
	}

	return filtered
}

func generateTemplate(format string) {
	template := nodeFile{
		Version: "1.0",
		Nodes: []*common.NodeInfo{
			{
				ID:        "web-server-01",
				Name:      "Web Server 01",
				Address:   "192.168.1.10",
				Port:      22,
				User:      "root",
				Password:  "",
				SSHKey:    "~/.ssh/id_rsa",
				ProxyJump: "",
				Status:    "offline",
				Groups:    []string{"web", "production"},
				Labels:    map[string]string{"env": "prod", "region": "cn-east-1"},
			},
			{
				ID:        "db-server-01",
				Name:      "Database Server 01",
				Address:   "192.168.1.20",
				Port:      22,
				User:      "postgres",
				Password:  "",
				SSHKey:    "",
				ProxyJump: "bastion.example.com",
				Status:    "offline",
				Groups:    []string{"database"},
				Labels:    map[string]string{"env": "prod", "type": "postgresql"},
			},
			{
				ID:        "cache-server-01",
				Name:      "Cache Server 01",
				Address:   "192.168.1.30",
				Port:      22,
				User:      "redis",
				Password:  "secure-password",
				SSHKey:    "~/.ssh/cache_key.pem",
				ProxyJump: "",
				Status:    "online",
				Groups:    []string{"cache", "staging"},
				Labels:    map[string]string{"env": "staging"},
			},
		},
	}

	var data []byte
	var err error

	if format == "json" {
		data, err = json.MarshalIndent(template, "", "  ")
	} else {
		data, err = yaml.Marshal(template)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.T("node.import.err_template", err))
		os.Exit(1)
	}

	fmt.Println(string(data))
}
