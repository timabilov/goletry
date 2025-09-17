package dbhelper

import (
	"fmt"
	"lessnoteapi/models"
	"lessnoteapi/services"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func SetupDB() *gorm.DB {

	db, err := gorm.Open(postgres.Open(
		fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s",
			services.GetEnv("DB_USERNAME", ""),
			services.GetEnv("DB_PASSWORD", ""),
			services.GetEnv("DB_HOST", ""),
			services.GetEnv("DB_PORT", ""),
			services.GetEnv("DB_NAME", ""),
		),
	), &gorm.Config{
		// Transac
	})
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
	Migrate(db, &models.UserPushToken{})
	Migrate(db, &models.Note{})
	Migrate(db, &models.Question{})
	Migrate(db, &models.Folder{})

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
