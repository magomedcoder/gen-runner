package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/magomedcoder/gen/config"
)

func CheckDatabase(ctx context.Context, dbCfg config.DatabaseConfig) error {
	targetDB, err := dbCfg.TargetDBName()
	if err != nil {
		return fmt.Errorf("конфигурация базы данных: %w", err)
	}
	adminDSN, err := dbCfg.AdminPostgresDSN()
	if err != nil {
		return fmt.Errorf("конфигурация базы данных (admin): %w", err)
	}

	db, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return fmt.Errorf("ошибка подключения к postgres: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ошибка проверки соединения с postgres: %w", err)
	}

	var exists int
	err = db.QueryRowContext(ctx, "SELECT 1 FROM pg_database WHERE datname = $1", targetDB).Scan(&exists)
	if err == nil {
		return nil
	}

	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("ошибка проверки существования БД: %w", err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", quoteIdentifier(targetDB)))
	if err != nil {
		return fmt.Errorf("ошибка создания базы данных %s: %w", targetDB, err)
	}

	return nil
}

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
