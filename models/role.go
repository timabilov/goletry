package models

import (
	"regexp"
	"strings"

	"github.com/go-playground/validator"
)

type Role string

const (
	ADMIN Role = "ADMIN"
	SALES Role = "SALES"
	OWNER Role = "OWNER"
)

func (l *Role) Scan(value interface{}) error {
	*l = Role(value.(string))
	return nil
}

func (l Role) Value() (string, error) {
	return string(l), nil
}
func (l Role) Emoji() string {
	msg := "?"
	value := strings.ToLower(string(l))
	switch value {
	case "OWNER":
		msg = "***"
	case "ADMIN":
		msg = "**"
	case "SALES":
		msg = "🇺*"
	}

	return msg
}
func ValidateRole(fl validator.FieldLevel) bool {
	value := fl.Field().String()

	matched, _ := regexp.MatchString("^ADMIN|SALES|OWNER$", string(value))
	return matched
}

func ValidateRoleRaw(value string) bool {

	matched, _ := regexp.MatchString("^ADMIN|SALES|OWNER$", value)
	return matched
}
