package controllers

import (
	"lessnoteapi/models"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func GetOrCreatePlayer(db *gorm.DB, deviceid string) *models.Player {
	//
	return nil
}

func BoolPointer(b bool) *bool {
	return &b
}

func StrPointer(b string) *string {
	return &b
}

func NilTime() *time.Time {

	return nil
}
func UIntToStr(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
func IntPointer(i int) *int {
	return &i
}

func Int64Pointer(i int64) *int64 {
	return &i
}

func UIntPointer(u uint) *uint {
	return &u
}

func Float32Pointer(u float32) *float32 {
	return &u
}

func Float64Pointer(u float64) *float64 {
	return &u
}

func NilFloat32() *float32 {
	return nil
}
func NilInt64() *int64 {
	return nil
}

func NilFloat64() *float64 {
	return nil
}

func NilString() *string {
	return nil
}

func IfThenElse(condition bool, a interface{}, b interface{}) interface{} {
	if condition {
		return a
	}
	return b
}

func GenerateUserToken(userPk string, c echo.Context, hours uint64) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   userPk,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 72)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	t, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.Logger().Errorf("Error when signing user token for %s. Error %s ", userPk, err)
		// sentry.CaptureMessage("Error when signing user token for %s. Error %s ", userPk, err)
	}
	return t
}

func GenerateRefreshToken(userPk string) (string, error) {
	refreshToken := jwt.New(jwt.SigningMethodHS256)
	rtClaims := refreshToken.Claims.(jwt.MapClaims)
	rtClaims["sub"] = userPk
	rtClaims["exp"] = time.Now().Add(time.Hour * 24 * 30 * 12).Unix()
	rt, err := refreshToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return "", err
	}
	return rt, nil
}

var inviteLetters = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")

func RandomInviteCode(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = inviteLetters[rand.Intn(len(inviteLetters))]
	}
	return string(b)
}
