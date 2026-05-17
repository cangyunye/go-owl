package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

func NewTuiCmd() *cobra.Command {
	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "启动交互式终端用户界面",
		Long: `启动 owl 工具的交互式终端用户界面（TUI）。

通过 TUI，您可以：
- 管理节点
- 执行批量命令
- 传输文件
- 运行剧本
- 使用 AI 助手
- 管理会话

示例：
  owl tui`,
		Run: runTui,
	}

	return tuiCmd
}

func runTui(cmd *cobra.Command, args []string) {
	// 首先尝试寻找已安装的 owl-tui 可执行文件
	tuiPath, err := findTuiExecutable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please build and install go-owl-tui first:\n")
		fmt.Fprintf(os.Stderr, "  cd /path/to/go-owl-tui && go install\n")
		os.Exit(1)
	}

	// 执行 TUI
	tuiCmd := exec.Command(tuiPath, args...)
	tuiCmd.Stdin = os.Stdin
	tuiCmd.Stdout = os.Stdout
	tuiCmd.Stderr = os.Stderr

	if err := tuiCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func findTuiExecutable() (string, error) {
	// 1. 首先尝试在 PATH 中查找 owl-tui
	if path, err := exec.LookPath("owl-tui"); err == nil {
		return path, nil
	}

	// 2. 尝试查找与当前 owl 可执行文件同目录的 owl-tui
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		tuiPath := filepath.Join(exeDir, "owl-tui")
		if _, err := os.Stat(tuiPath); err == nil {
			return tuiPath, nil
		}

		// Windows 版本
		if filepath.Ext(tuiPath) == "" {
			tuiPathWin := tuiPath + ".exe"
			if _, err := os.Stat(tuiPathWin); err == nil {
				return tuiPathWin, nil
			}
		}
	}

	// 3. 尝试从相对路径查找（如果两个项目在同一目录下）
	wd, err := os.Getwd()
	if err == nil {
		// 先尝试当前工作目录的同级目录
		parentDir := filepath.Dir(wd)
		candidate := filepath.Join(parentDir, "go-owl-tui", "owl-tui")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		// 尝试当前工作目录的 owl-tui
		candidate = filepath.Join(wd, "owl-tui")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find owl-tui executable in PATH or adjacent directories")
}
