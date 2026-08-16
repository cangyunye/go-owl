package main

import (
	"fmt"
	"log"
	"os"

	"github.com/cangyunye/go-owl/cmd/plugins/serve"
	"github.com/spf13/cobra"
)

func main() {
	var (
		port        int
		host        string
		dev         bool
		resetAdmin  bool
		aiDebug     bool
	)

	rootCmd := &cobra.Command{
		Use:   "owl-serve",
		Short: "OWL Web 管理控制台服务",
		Long: `启动 OWL 的 Web 管理控制台，提供基于浏览器的节点管理和操作界面。

功能：
- 节点浏览、搜索和管理
- 基于角色的多用户访问控制
- RESTful JSON API`,
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
				fmt.Printf("URL:      http://%s:%d\n", host, port)
				fmt.Printf("Username: %s\n", creds.Username)
				fmt.Printf("Password: %s\n", creds.Password)
				fmt.Println("")
			} else if !resetAdmin {
				fmt.Printf("OWL Console starting at http://%s:%d\n", host, port)
			}

			if err := srv.Start(); err != nil {
				log.Fatalf("server error: %v", err)
			}
		},
	}

	rootCmd.Flags().IntVarP(&port, "port", "p", 8080, "HTTP port")
	rootCmd.Flags().StringVar(&host, "host", "127.0.0.1", "HTTP host")
	rootCmd.Flags().BoolVar(&dev, "dev", false, "Development mode (frontend from filesystem)")
	rootCmd.Flags().BoolVar(&resetAdmin, "reset-admin", false, "Reset admin password")
	rootCmd.Flags().BoolVar(&aiDebug, "ai-debug", false, "Enable AI debug mode (logs full prompt/reply text)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
