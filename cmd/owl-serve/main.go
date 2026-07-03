package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cangyunye/go-owl/cmd/plugins/serve"
)

func main() {
	port := flag.Int("port", 8080, "HTTP port")
	host := flag.String("host", "127.0.0.1", "HTTP host")
	dev := flag.Bool("dev", false, "Development mode (frontend from filesystem)")
	resetAdmin := flag.Bool("reset-admin", false, "Reset admin password and exit")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("cannot get home dir: %v", err)
	}

	dbPath := home + "/.owl/owl.db"

	cfg := &serve.Config{
		DBPath:     dbPath,
		ListenAddr: fmt.Sprintf("%s:%d", *host, *port),
		DevMode:    *dev,
	}

	srv := serve.NewServer(cfg)

	if *resetAdmin {
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			log.Fatalf("database not found at %s, start the server first to initialize", dbPath)
		}
		creds, err := srv.ResetAdmin()
		if err != nil {
			log.Fatalf("reset admin: %v", err)
		}
		fmt.Println("Admin password has been reset.")
		fmt.Printf("Username: %s\n", creds.Username)
		fmt.Printf("Password: %s\n", creds.Password)
		fmt.Println("\nPlease save this password. It will not be shown again.")
		return
	}

	creds, err := srv.Init()
	if err != nil {
		log.Fatalf("server init: %v", err)
	}

	if creds != nil {
		fmt.Printf("\nURL:      http://%s:%d\n", *host, *port)
		fmt.Printf("Username: %s\n", creds.Username)
		fmt.Printf("Password: %s\n", creds.Password)
		fmt.Println("")
	} else {
		fmt.Printf("OWL Console starting at http://%s:%d\n", *host, *port)
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
