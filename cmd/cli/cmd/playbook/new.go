package playbook

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "github.com/cangyunye/go-owl/pkg/playbook"
	"github.com/spf13/cobra"

	"github.com/cangyunye/go-owl/internal/i18n"
)

var (
	pbNewFrom   string
	pbNewVars   []string
	pbNewOutput string
)

func NewPlaybookNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: i18n.T("playbook.new.short"),
		Long:  i18n.T("playbook.new.long"),
		Run:   runPlaybookNew,
	}

	cmd.Flags().StringVar(&pbNewFrom, "from", "", i18n.T("playbook.new.flag_from"))
	cmd.Flags().StringArrayVar(&pbNewVars, "var", nil, i18n.T("playbook.new.flag_var"))
	cmd.Flags().StringVarP(&pbNewOutput, "output", "o", "", i18n.T("playbook.new.flag_output"))

	_ = cmd.MarkFlagRequired("from")

	return cmd
}

func runPlaybookNew(cmd *cobra.Command, args []string) {
	entry, err := pb.GetTemplate(pbNewFrom, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.new.err", err))
		os.Exit(1)
	}

	meta, err := pb.ParseTemplateMeta(entry.Content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.new.err", err))
		os.Exit(1)
	}

	provided := parseVarFlags(pbNewVars)

	promptForMissingParams(meta.Parameters, provided)

	validated, err := pb.ValidateParams(meta.Parameters, provided)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.new.err", err))
		os.Exit(1)
	}

	rendered, err := pb.Instantiate(entry.Content, validated)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.new.err", err))
		os.Exit(1)
	}

	outputPath := determineNewOutputPath(pbNewFrom, pbNewOutput)

	if err := savePlaybookFile(outputPath, rendered); err != nil {
		fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.new.err", err))
		os.Exit(1)
	}

	fmt.Printf("%s", i18n.T("playbook.new.ok_created", outputPath))
	fmt.Println(i18n.T("playbook.new.hint_command"))
	fmt.Printf("%s", i18n.T("playbook.new.run_hint", outputPath))
}

func parseVarFlags(vars []string) map[string]interface{} {
	provided := make(map[string]interface{})
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.new.err_var_invalid", v))
			os.Exit(1)
		}
		provided[parts[0]] = parts[1]
	}
	return provided
}

func promptForMissingParams(params []pb.TemplateParameter, provided map[string]interface{}) {
	reader := bufio.NewReader(os.Stdin)
	for _, p := range params {
		if _, ok := provided[p.Name]; ok {
			continue
		}

		prompt := i18n.T("playbook.new.prompt_param", p.Name)
		if p.Description != "" {
			prompt += i18n.T("playbook.new.prompt_param_desc", p.Description)
		}
		if p.Default != nil {
			prompt += i18n.T("playbook.new.prompt_param_default", p.Default)
		}
		fmt.Printf("%s: ", prompt)

		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", i18n.T("playbook.new.err_read", err))
			os.Exit(1)
		}
		if val := strings.TrimSpace(input); val != "" {
			provided[p.Name] = val
		}
	}
}

func determineNewOutputPath(from, specifiedPath string) string {
	if specifiedPath != "" {
		return specifiedPath
	}
	name := from
	if i := strings.LastIndex(from, "/"); i >= 0 {
		name = from[i+1:]
	}
	return filepath.Join("./playbooks", name+".yaml")
}
