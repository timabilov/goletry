package models

import (
	"regexp"

	"github.com/go-playground/validator"
)

type Platform string

const (
	PlatformIOS     Platform = "ios"
	PlatformAndroid Platform = "android"
	PlatformWeb     Platform = "web"
)

func (l *Platform) Scan(value interface{}) error {
	*l = Platform(value.(string))
	return nil
}

func (l Platform) Value() string {
	return string(l)
}

func ScanPlatform(value string) Platform {
	return Platform(value)
}
func ValidatePlatform(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	matched, _ := regexp.MatchString("^ios|android|web$", string(value))
	return matched
}

func ValidatePlatformRaw(value string) bool {
	matched, _ := regexp.MatchString("^ios|android|web$", string(value))
	print(matched)
	return matched
}
