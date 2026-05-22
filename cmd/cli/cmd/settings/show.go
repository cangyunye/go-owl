package settings

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewSettingsShowCmd 创建显示设置命令
func NewSettingsShowCmd() *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "显示当前设置",
		Run:   runSettingsShow,
	}

	return showCmd
}

func runSettingsShow(cmd *cobra.Command, args []string) {
	settings := getCurrentSettings()

	fmt.Println("Current Settings:")
	fmt.Println("=================")
	fmt.Println()
	fmt.Println("Output:")
	fmt.Printf("  Format: %s\n", settings.Output.Format)
	fmt.Printf("  Color:  %v\n", settings.Output.Color)
	fmt.Println()
	fmt.Println("Default:")
	fmt.Printf("  Timeout:  %s\n", settings.Default.Timeout)
	fmt.Printf("  Parallel: %v\n", settings.Default.Parallel)
	if settings.Default.Group != "" {
		fmt.Printf("  Group:    %s\n", settings.Default.Group)
	}
	if len(settings.Default.Labels) > 0 {
		fmt.Printf("  Labels:   %v\n", settings.Default.Labels)
	}
}
