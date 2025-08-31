package main

import (
	"flag"
	"log"

	"auction_service/internal/config"
	"auction_service/internal/database"
)

func main() {
	var action string
	flag.StringVar(&action, "action", "", "Migration action: up, down, force, or status")
	flag.Parse()

	if action == "" {
		log.Fatal("Please specify action: -action=up|down|force|status")
	}

	// 載入配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// 執行遷移
	if err := database.RunMigrations(cfg, action); err != nil {
		log.Fatal("Migration failed:", err)
	}
}