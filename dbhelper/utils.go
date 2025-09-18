package dbhelper

import (
	"letryapi/models"
	"log"

	"gorm.io/gorm"
)

func SetupCleaner(db *gorm.DB) func() {

	return func() {

		db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.ClothingTryonGeneration{})
		db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.Clothing{})
		db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.UserCompanyRole{})
		db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.Company{})
		db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.UserPushToken{})
		db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.UserAccount{})

	}
}

func Migrate(db *gorm.DB, model interface{}) {
	err := db.AutoMigrate(model)
	if err != nil {
		log.Printf("Error while migrating %s", model)
		log.Fatal(err)
	}
}
