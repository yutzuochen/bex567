package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := sql.Open("mysql", "app:app_password@tcp(127.0.0.1:3308)/business_exchange?parseTime=true&charset=utf8mb4")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	// 檢查是否還有 localhost URL
	rows, err := db.Query("SELECT id, filename, url FROM images WHERE url LIKE '%127.0.0.1%' OR url LIKE '%localhost%' LIMIT 10")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	fmt.Println("🔍 檢查遺漏的 localhost URL:")
	count := 0
	for rows.Next() {
		var id int
		var filename, url string
		rows.Scan(&id, &filename, &url)
		fmt.Printf("  ID: %d, Filename: %s, URL: %s\n", id, filename, url)
		count++
	}

	if count == 0 {
		fmt.Println("✅ 沒有發現遺漏的 localhost URL")
	} else {
		fmt.Printf("⚠️  發現 %d 個遺漏的 localhost URL\n", count)
	}

	// 檢查正確的 URL 數量
	var correctCount int
	err = db.QueryRow("SELECT COUNT(*) FROM images WHERE url LIKE '%business-exchange-backend%'").Scan(&correctCount)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("✅ 正確的 URL 數量: %d\n", correctCount)

	// 檢查總數
	var totalCount int
	err = db.QueryRow("SELECT COUNT(*) FROM images").Scan(&totalCount)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("📊 總圖片數量: %d\n", totalCount)
}
