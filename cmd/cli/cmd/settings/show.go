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
	fmt.Println("Server:")
	fmt.Printf("  Address: %s\n", settings.Server.Address)
	fmt.Printf("  Timeout: %s\n", settings.Server.Timeout)
	fmt.Println()
	fmt.Println("Output:")
	fmt.Printf("  Format: %s\n", settings.Output.Format)
	fmt.Printf("  Color:  %v\n", settings.Output.Color)
	fmt.Println()
	fmt.Println("Diffusion:")
	fmt.Printf("  Fan-out:      %d\n", settings.Diffusion.FanOut)
	fmt.Printf("  Max depth:    %d\n", settings.Diffusion.MaxDepth)
	fmt.Printf("  Source count: %d\n", settings.Diffusion.SourceCount)
	fmt.Println()
	fmt.Println("Defaults:")
	fmt.Printf("  Timeout: %s\n", settings.Defaults.Timeout)
	if len(settings.Defaults.Groups) > 0 {
		fmt.Printf("  Groups:  %v\n", settings.Defaults.Groups)
	} else {
		fmt.Println("  Groups:  (none)")
	}
	if len(settings.Defaults.Labels) > 0 {
		fmt.Printf("  Labels:  %v\n", settings.Defaults.Labels)
	} else {
		fmt.Println("  Labels:  (none)")
	}
}
