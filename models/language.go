package models

import (
	"regexp"
	"strings"

	"github.com/go-playground/validator"
)

type Language string

const (
	AZ Language = "az"
	TR Language = "tr"
	EN Language = "en"
)

func (l *Language) Scan(value interface{}) error {
	*l = Language(value.(string))
	return nil
}

func (l Language) Value() (string, error) {
	return string(l), nil
}
func (l Language) Emoji() string {
	msg := "?"
	value := strings.ToLower(string(l))
	switch value {
	case "az":
		msg = "🇦🇿"
	case "tr":
		msg = "🇹🇷"
	case "en":
		msg = "🇺🇸"
	}

	return msg
}
func ValidateLanguage(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	matched, _ := regexp.MatchString("^az|tr|en$", string(value))
	return matched
}

func ValidateLanguageRaw(value string) bool {

	matched, _ := regexp.MatchString("^az|tr|en$", value)
	return matched
}
