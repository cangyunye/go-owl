package serve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cangyunye/go-owl/internal/i18n"
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
		Short: i18n.T("serve.cmd.short"),
		Long:  i18n.T("serve.cmd.long"),
		Run:   runServe,
	}

	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, i18n.T("serve.flag_port"))
	serveCmd.Flags().StringVar(&host, "host", "127.0.0.1", i18n.T("serve.flag_host"))
	serveCmd.Flags().BoolVar(&dev, "dev", false, i18n.T("serve.flag_dev"))
	serveCmd.Flags().BoolVar(&resetAdmin, "reset-admin", false, i18n.T("serve.flag_reset_admin"))
	serveCmd.Flags().BoolVar(&aiDebug, "ai-debug", false, i18n.T("serve.flag_ai_debug"))

	return serveCmd
}

func runServe(cmd *cobra.Command, args []string) {
	binPath, err := findServeBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "%s", i18n.T("serve.build_hint"))
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
