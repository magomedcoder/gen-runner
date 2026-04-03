package provider

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/magomedcoder/gen/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewDB(ctx context.Context, database *config.DatabaseConfig, appLogLevel string) (*gorm.DB, error) {
	dsn, err := database.PostgresDSN()
	if err != nil {
		return nil, err
	}

	gormLogLevel := logger.Warn
	switch strings.ToLower(strings.TrimSpace(appLogLevel)) {
	case "debug":
		gormLogLevel = logger.Info
	case "error", "fatal", "panic":
		gormLogLevel = logger.Error
	}

	sqlLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             400 * time.Millisecond,
			LogLevel:                  gormLogLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		PrepareStmt: true,
		Logger:      sqlLogger,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к базе данных: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("получение sql.DB: %w", err)
	}

	maxOpen := database.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}

	maxIdle := database.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = max(maxOpen/4, 2)

		if maxIdle > maxOpen {
			maxIdle = maxOpen
		}
	}

	life := 30 * time.Minute
	if s := strings.TrimSpace(database.ConnMaxLifetime); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			life = d
		}
	}

	idle := 5 * time.Minute
	if s := strings.TrimSpace(database.ConnMaxIdleTime); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			idle = d
		}
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(life)
	sqlDB.SetConnMaxIdleTime(idle)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ошибка проверки соединения с базой данных: %w", err)
	}

	return gdb, nil
}
