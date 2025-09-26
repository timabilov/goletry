package controllers

import (
	"errors"
	"fmt"
	"letryapi/models"
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func NoMembershipUserMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		db := c.Get("__db").(*gorm.DB)
		userRaw := c.Get("user")
		if userRaw == nil {
			return echo.ErrUnauthorized
		}
		user := userRaw.(*jwt.Token)
		claims := user.Claims.(jwt.MapClaims)
		userId := claims["sub"]
		if userId == nil || userId == "" {
			log.Println("Error while getting the token information!")
			return echo.ErrUnauthorized
		}

		var currentUser models.UserAccount
		db.First(&currentUser, userId)
		// todo check if has company?

		c.Set("currentUser", currentUser)
		fmt.Printf("Fetched user %s \n", currentUser.Name)
		return next(c)
	}
}

func UserMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		db := c.Get("__db").(*gorm.DB)
		userRaw := c.Get("user")
		if userRaw == nil {
			return echo.ErrUnauthorized
		}
		user := userRaw.(*jwt.Token)
		claims := user.Claims.(jwt.MapClaims)
		userId := claims["sub"]
		if userId == nil || userId == "" {
			log.Println("Error while getting the token information!")
			return echo.ErrUnauthorized
		}

		var currentUser models.UserAccount
		db.Preload("Memberships.Company").First(&currentUser, userId)
		// todo check if has company?
		if len(currentUser.Memberships) == 0 {
			// just indicator..
			return echo.NewHTTPError(http.StatusLocked)
		}
		c.Set("currentUser", currentUser)
		fmt.Printf("Fetched user %s memberships: %v \n ", currentUser.Name, len(currentUser.Memberships))
		return next(c)
	}
}

func UserOnlyMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		db := c.Get("__db").(*gorm.DB)
		userRaw := c.Get("user")
		if userRaw == nil {
			return echo.ErrUnauthorized
		}
		user := userRaw.(*jwt.Token)
		claims := user.Claims.(jwt.MapClaims)
		userId := claims["sub"]
		if userId == nil || userId == "" {
			log.Println("Error while getting the token information!")
			return echo.ErrUnauthorized
		}

		var currentUser models.UserAccount
		db.Preload("Memberships.Company").First(&currentUser, userId)
		c.Set("currentUser", currentUser)

		fmt.Printf("Fetched only user access %s memberships: %v \n ", currentUser.Name, len(currentUser.Memberships))
		return next(c)
	}
}
func UserCompanyMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		db := c.Get("__db").(*gorm.DB)
		var companyId uint
		err := echo.PathParamsBinder(c).Uint("companyId", &companyId).BindError() // returns first binding error

		if err != nil {
			return echo.ErrBadRequest
		}

		userRaw := c.Get("user")
		if userRaw == nil {
			return echo.ErrUnauthorized
		}
		user := userRaw.(*jwt.Token)
		claims := user.Claims.(jwt.MapClaims)
		userId := claims["sub"]
		fmt.Println(claims)
		if userId == nil || userId == "" {
			log.Println("Error while getting the token information!")
			return echo.ErrUnauthorized
		}

		var currentUser models.UserAccount
		result := db.Preload("Memberships.Company").Where("ID = ?", userId).Take(&currentUser)

		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return echo.ErrNotFound
		}
		if result.Error != nil {
			fmt.Println("Failed to fetch team info", result.Error)
			return echo.ErrInternalServerError
		}
		if len(currentUser.Memberships) == 0 {
			// just indicator
			return echo.NewHTTPError(http.StatusLocked)
		}
		if len(currentUser.Memberships) != 1 {
			fmt.Println(len(currentUser.Memberships), " companies fetched for user, expected one!", currentUser.Name, " path company id", companyId)
			return echo.ErrInternalServerError
		}
		if currentUser.Banned {
			return echo.NewHTTPError(http.StatusLocked)
		}
		if !currentUser.Memberships[0].Active {
			fmt.Println("Not active member accessing company data user id", currentUser.ID, "member id ", currentUser.Memberships[0].ID)
			return echo.NewHTTPError(http.StatusLocked)
		}
		c.Set("currentUser", currentUser)
		c.Set("currentCompany", companyId)
		fmt.Println("Fetched user ", currentUser.Name, " company fetched ", currentUser.Memberships[0].Company.Name)
		return next(c)
	}
}
