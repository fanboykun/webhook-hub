package sqlite

import (
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func applyMigrations(db *gorm.DB, logger *slog.Logger) error {
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL
	);`).Error; err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		versions = append(versions, entry.Name())
	}
	sort.Strings(versions)

	for _, version := range versions {
		var count int64
		if err := db.Table("schema_migrations").Where("version = ?", version).Count(&count).Error; err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		body, err := migrationFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(string(body)).Error; err != nil {
				return fmt.Errorf("apply migration %s: %w", version, err)
			}
			return tx.Exec("INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", version, time.Now().UTC()).Error
		}); err != nil {
			return err
		}

		logger.Info("migration.applied", "version", version)
	}

	return nil
}
