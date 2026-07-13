package serve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	port        int
	host        string
	dev         bool
	resetAdmin  bool
	aiDebug     bool
)

func NewServeCmd() *cobra.Command {
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "启动 OWL Web 管理控制台",
		Long: `启动 OWL 的 Web 管理控制台，提供基于浏览器的节点管理和操作界面。

功能：
- 节点浏览、搜索和管理
- 基于角色的多用户访问控制
- RESTful JSON API

示例：
  owl serve
  owl serve --port 9090
  owl serve --host 0.0.0.0 --port 8080
  owl serve --dev
  owl serve --reset-admin --port 8080
  owl serve --ai-debug`,
		Run: runServe,
	}

	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "HTTP 监听端口")
	serveCmd.Flags().StringVar(&host, "host", "127.0.0.1", "HTTP 监听地址")
	serveCmd.Flags().BoolVar(&dev, "dev", false, "开发模式（前端从文件系统加载）")
	serveCmd.Flags().BoolVar(&resetAdmin, "reset-admin", false, "重置管理员密码")
	serveCmd.Flags().BoolVar(&aiDebug, "ai-debug", false, "AI 调试模式（记录完整提示词/回复）")

	return serveCmd
}

func runServe(cmd *cobra.Command, args []string) {
	binPath, err := findServeBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "请先构建 owl-serve:\n")
		fmt.Fprintf(os.Stderr, "  cd go-owl && go build -o $(go env GOPATH)/bin/owl-serve ./cmd/owl-serve\n")
		os.Exit(1)
	}

	serveArgs := []string{
		fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--host=%s", host),
	}
	if dev {
		serveArgs = append(serveArgs, "--dev")
	}
	if resetAdmin {
		serveArgs = append(serveArgs, "--reset-admin")
	}
	if aiDebug {
		serveArgs = append(serveArgs, "--ai-debug")
	}

	serveCmd := exec.Command(binPath, serveArgs...)
	serveCmd.Stdin = os.Stdin
	serveCmd.Stdout = os.Stdout
	serveCmd.Stderr = os.Stderr

	if err := serveCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error running owl-serve: %v\n", err)
		os.Exit(1)
	}
}

func findServeBinary() (string, error) {
	if path, err := exec.LookPath("owl-serve"); err == nil {
		return path, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}

	// Resolve symlinks to find the real path (e.g. mybin/owl -> go-owl/build/owl-*)
	realPath, err := filepath.EvalSymlinks(exePath)
	if err == nil {
		exePath = realPath
	}

	// Check sibling directory of the exe
	exeDir := filepath.Dir(exePath)
	siblingPath := filepath.Join(exeDir, "owl-serve")
	if _, err := os.Stat(siblingPath); err == nil {
		return siblingPath, nil
	}

	// Walk up from the exe directory looking for go-owl/build/owl-serve
	dir := exeDir
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "build", "owl-serve")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find owl-serve executable in PATH, adjacent directory, or project build/ directory")
}
