package database

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"auction_service/internal/config"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func RunMigrations(cfg *config.Config, action string) error {
	// 直接連接資料庫
	db, err := sql.Open("mysql", cfg.GetDBDSN())
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migrate driver: %w", err)
	}

	// 取得 migrations 目錄的絕對路徑
	migrationsPath, err := filepath.Abs("./migrations")
	if err != nil {
		return fmt.Errorf("failed to get migrations path: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", migrationsPath),
		"mysql",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	switch action {
	case "up":
		err = m.Up()
		if err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("failed to run migrations up: %w", err)
		}
		fmt.Println("Migrations applied successfully")
	case "down":
		err = m.Down()
		if err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("failed to run migrations down: %w", err)
		}
		fmt.Println("Migrations rolled back successfully")
	case "force":
		// Force version to 3 to clean dirty state (tables already exist)
		err = m.Force(3)
		if err != nil {
			return fmt.Errorf("failed to force migration version: %w", err)
		}
		fmt.Println("Migration version forced to 3, dirty state cleared")
	case "status":
		version, dirty, err := m.Version()
		if err != nil {
			return fmt.Errorf("failed to get migration status: %w", err)
		}
		fmt.Printf("Current version: %d, Dirty: %v\n", version, dirty)
	default:
		return fmt.Errorf("unknown migration action: %s", action)
	}

	return nil
}