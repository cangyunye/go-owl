package main

import (
	"fmt"
	"log"
	"os"

	"github.com/cangyunye/go-owl/cmd/plugins/serve"
	"github.com/cangyunye/go-owl/internal/i18n"
	"github.com/spf13/cobra"
)

// 构建注入：make build-serve / build 的 SERVE_LDFLAGS
//   go build -ldflags "-X main.version=1.2.3 -X main.commitID=abc1234" ./cmd/owl-serve
var (
	version   = "dev"
	commitID  = "unknown"
	buildTime = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		port       int
		host       string
		dev        bool
		resetAdmin bool
		aiDebug    bool
	)

	rootCmd := &cobra.Command{
		Use:     "owl-serve",
		Short:   i18n.T("serve.cmd.short"),
		Long:    i18n.T("serve.cmd.long"),
		Version: fmt.Sprintf("%s (commit %s, built %s)", version, commitID, buildTime),
		Run: func(cmd *cobra.Command, args []string) {
			dbPath := resolveDBPath()

			cfg := &serve.Config{
				DBPath:      dbPath,
				ListenAddr:  fmt.Sprintf("%s:%d", host, port),
				DevMode:     dev,
				AIDebugMode: aiDebug,
			}

			srv := serve.NewServer(cfg)

			if resetAdmin {
				if _, err := os.Stat(dbPath); os.IsNotExist(err) {
					log.Fatalf("database not found at %s, start the server first to initialize", dbPath)
				}
				creds, err := srv.ResetAdmin()
				if err != nil {
					log.Fatalf("reset admin: %v", err)
				}
				fmt.Println("\nAdmin password has been reset.")
				fmt.Printf("Username: %s\n", creds.Username)
				fmt.Printf("Password: %s\n", creds.Password)
				fmt.Println("")
			}

			creds, err := srv.Init()
			if err != nil {
				log.Fatalf("server init: %v", err)
			}

			if creds != nil {
				fmt.Printf("OWL Console %s\n", version)
				fmt.Printf("URL:      http://%s:%d\n", host, port)
				fmt.Printf("Username: %s\n", creds.Username)
				fmt.Printf("Password: %s\n", creds.Password)
				fmt.Println("")
			} else {
				fmt.Printf("OWL Console %s starting at http://%s:%d\n", version, host, port)
			}

			if err := srv.Start(); err != nil {
				log.Fatalf("server error: %v", err)
			}
		},
	}

	rootCmd.Flags().IntVarP(&port, "port", "p", 8080, i18n.T("serve.flag_port"))
	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", i18n.T("serve.flag_host"))
	rootCmd.Flags().BoolVar(&dev, "dev", false, i18n.T("serve.flag_dev"))
	rootCmd.Flags().BoolVar(&resetAdmin, "reset-admin", false, i18n.T("serve.flag_reset_admin"))
	rootCmd.Flags().BoolVar(&aiDebug, "ai-debug", false, i18n.T("serve.flag_ai_debug"))

	return rootCmd
}
