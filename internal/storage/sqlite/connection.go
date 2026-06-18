package sqlite

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/fanboykun/webhook-hub/internal/config"
	sqlite_adapter "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	glogger "gorm.io/gorm/logger"
)

func Open(cfg config.DatabaseConfig, logger *slog.Logger) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}

	db, err := gorm.Open(sqlite_adapter.Open(cfg.Path), &gorm.Config{
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("open sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(max(1, cfg.MaxOpenConnections))
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		fmt.Sprintf("PRAGMA busy_timeout = %d;", max(int(cfg.BusyTimeout.Milliseconds()), 5000)),
		"PRAGMA synchronous = NORMAL;",
	}
	for _, stmt := range pragmas {
		if err := db.Exec(stmt).Error; err != nil {
			return nil, fmt.Errorf("apply pragma %q: %w", stmt, err)
		}
	}

	if err := applyMigrations(db, logger); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Ping(ctx context.Context) error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
