package provider

import (
	"context"
	"fmt"
	"github.com/magomedcoder/gen/internal/config"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewDB(ctx context.Context, conf *config.Config, appLogLevel string) (*gorm.DB, error) {
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

	gormConfig := &gorm.Config{
		PrepareStmt: true,
		Logger:      sqlLogger,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	}

	gdb, err := gorm.Open(postgres.New(postgres.Config{
		DSN: fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Europe/Moscow",
			conf.Database.Host,
			conf.Database.Port,
			conf.Database.Username,
			conf.Database.Password,
			conf.Database.Database,
		),
	}), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к базе данных: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("получение sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Minute)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ошибка проверки соединения с базой данных: %w", err)
	}

	return gdb, nil
}
