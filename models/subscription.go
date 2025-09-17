package models

import (
	"regexp"

	"github.com/go-playground/validator"
)

type Subscription string

const (
	Free    Subscription = "free" // basic
	Trial   Subscription = "trial"
	Pro     Subscription = "pro"
	ProPlus Subscription = "pro_plus" // pro +
)

func (l *Subscription) Scan(value interface{}) error {
	*l = Subscription(value.(string))
	return nil
}

func (l Subscription) Value() (string, error) {
	return string(l), nil
}

func ValidateSubscription(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	matched, _ := regexp.MatchString("^free|basic|full$", string(value))
	return matched
}

func ValidateSubscriptionRaw(value string) bool {
	matched, _ := regexp.MatchString("^free|basic|full$", string(value))
	return matched
}
