package dbhelper

import (
	"fmt"
	"letryapi/models"
	"letryapi/services"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func SetupDB() *gorm.DB {
	sslMode := "?sslmode=require"
	if _, isProduction := os.LookupEnv("PORT"); !isProduction {
		sslMode = ""
	}

	db, err := gorm.Open(postgres.Open(
		fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s%s",
			services.GetEnv("DB_USERNAME", ""),
			services.GetEnv("DB_PASSWORD", ""),
			services.GetEnv("DB_HOST", ""),
			services.GetEnv("DB_PORT", ""),
			services.GetEnv("DB_NAME", ""),
			sslMode,
		),
	), &gorm.Config{
		// Transac
	})

	if err != nil {
		fmt.Println("Failed to connect to database", err)
		panic(err)
	}
	sqlDB, err := db.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(300)
	sqlDB.SetConnMaxLifetime(time.Minute * 5)
	db.Logger.LogMode(logger.LogLevel(logger.Info))
	if err != nil {
		// panic("failed to connect to database")
		panic(err)
	}
	db.Raw("CREATE EXTENSION if not exists pgcrypto;")
	Migrate(db, &models.UserAccount{})
	Migrate(db, &models.UserCompanyRole{})
	Migrate(db, &models.Company{})
	Migrate(db, &models.ClothingTryonGeneration{})
	Migrate(db, &models.Clothing{})
	Migrate(db, &models.UserPushToken{})

	return db
}

func SetupTestDB() *gorm.DB {
	os.Setenv("DB_USERNAME", "fastpos")
	os.Setenv("DB_PASSWORD", "fastpos")
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_NAME", "fastpos")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("RC_WEBHOOK_TOKEN", "fake")
	return SetupDB()
}
