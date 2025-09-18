package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"letryapi/models"
	"letryapi/services"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type WebhooksController struct {
	Google      services.GoogleServiceProvider
	FirebaseApp *firebase.App
}

func (wc *WebhooksController) SetupRoutes(g *echo.Group) {

	g.POST("/rc-subscription-webhooks", func(c echo.Context) error {
		fmt.Println("Received webhook for subscription event auth: ", c.Request().Header.Get("Authorization"))
		if c.Request().Header.Get("Authorization") != "Bearer "+os.Getenv("RC_WEBHOOK_TOKEN") {
			// ip and other info logging
			fmt.Println("Invalid Authorization header for webhook!")
			fmt.Println("[Malicious] IP: ", c.RealIP(), "User agent: ", c.Request().Header.Get("User-Agent"), "Authorization: ", c.Request().Header.Get("Authorization"))
			return echo.ErrUnauthorized
		}

		db, ok := c.Get("__db").(*gorm.DB)
		if !ok {
			fmt.Println("error getting DB for subscription!")
			return echo.ErrInternalServerError
		}

		b, err := io.ReadAll(c.Request().Body)
		if err != nil {
			fmt.Println(err)
			return echo.ErrInternalServerError
		}
		var eventData map[string]interface{}
		fmt.Println("Event: ", string(b))
		err = json.NewDecoder(bytes.NewReader(b)).Decode(&eventData)
		if err != nil {
			fmt.Println("error parsing event json!")
			return echo.ErrInternalServerError
		}

		event, ok := eventData["event"].(map[string]interface{})
		if !ok {
			fmt.Println("Cannot parse event!")
			return echo.ErrInternalServerError
		}
		appUserId, ok := event["app_user_id"].(string)

		eventType, ok := event["type"].(string)
		if eventType == "TRANSFER" {
			fmt.Println("Transfer skip..")
			return c.JSON(http.StatusOK, echo.Map{
				"message": "OK TRANSFER",
			})
		}
		bot, err := tgbotapi.NewBotAPI(os.Getenv("TG_TOKEN"))

		if strings.Contains(appUserId, "$RCAnonymousID") {
			appUserId = event["original_app_user_id"].(string)
			if strings.Contains(appUserId, "$RCAnonymousID") {
				fmt.Println("Anonymous ID couldnt verify the user!", appUserId)
				msg := tgbotapi.NewMessage(-1002078967836, fmt.Sprintf("Unkown user %s event: %s ", appUserId, eventType))
				if bot != nil {

					_, err := bot.Send(msg)

					if err != nil {
						fmt.Println(err)
					}
				}
				return c.JSON(http.StatusOK, echo.Map{
					"message": "Error unknown user",
				})
			}
		}
		if !ok {

			fmt.Println("Cannot parse app user id!")
			return echo.ErrInternalServerError
		}

		time.Sleep(time.Second * 4)
		b, err = wc.Google.GetUserSubscriptionStatus(context.Background(), appUserId)
		if err != nil {
			fmt.Println(err)
			return echo.ErrInternalServerError
		}
		fmt.Println("Status sub: ", string(b))

		var subData map[string]interface{}

		err = json.NewDecoder(bytes.NewReader(b)).Decode(&subData)
		if err != nil {
			fmt.Println("Error decoding user subscription status", err)
			return echo.ErrInternalServerError
		}

		// var subscriber map[string]interface{}
		subscriber, ok := subData["subscriber"].(map[string]interface{})

		if !ok {

			fmt.Println("Error readin sub status of user ", appUserId)
			return echo.ErrInternalServerError
		}

		entitlements, ok := subscriber["entitlements"].(map[string]interface{})

		if !ok {

			fmt.Println("Error readin sub status of user ", appUserId)
			return echo.ErrInternalServerError
		}

		pro_entitlement, pro_ok := entitlements["pro"].(map[string]interface{})
		// pro_plus_entitlement, pro_plus_ok := entitlements["Pro Plus"].(map[string]interface{})
		time_layout := "2006-01-02T15:04:05Z"

		var user models.UserAccount
		userId, err := strconv.ParseUint(appUserId, 10, 32)
		if err != nil {
			fmt.Println("Cannot get company id parse to update sub!", appUserId)
			return echo.ErrInternalServerError
		}
		result := db.First(&user, userId)
		if result.Error != nil {
			fmt.Println("Cannot get company to update sub!", appUserId)
			return echo.ErrInternalServerError
		}

		if err != nil {
			fmt.Println("Error initializing telegram BOT!")
		}
		if eventType == "EXPIRATION" {
			reason, _ := event["expiration_reason"]
			var planString = string(models.Free)
			user.Subscription = &planString
			// user.ExpirationDate = &t
			db.Save(&user)
			var companies []models.Company
			result = db.Where("owner_id = ?", userId).Find(&companies)
			if result.Error != nil {
				fmt.Println("Error getting user companies", result.Error)
				return echo.ErrInternalServerError
			}

			var companiesString = ""
			for index, company := range companies {
				company.Subscription = models.Free
				companiesString += fmt.Sprintf("%s", company.Name)
				if index != len(companies)-1 {
					companiesString += ","
				}
				db.Save(&company)
				fmt.Println("No active sub/entitlements found for user , updating ", user.Name, company.Name)
			}
			msg := tgbotapi.NewMessage(-1002078967836, fmt.Sprintf("🛑 %s(%s) %s reason %s", user.Name, companiesString, eventType, reason))
			if bot != nil {

				_, err := bot.Send(msg)

				if err != nil {
					fmt.Println(err)
				}
			}

			services.SendNotification(wc.FirebaseApp, db, user.ID, "Subscription expired", "Oh, no! Your will not be able to create notes. Subscribe again to continue enjoying Lessnote! 🔥", nil)

			return c.JSON(http.StatusOK, echo.Map{
				"message": "expire ok",
			})
		}

		if eventType == "CANCELLATION" {
			reason, _ := event["cancel_reason"]
			var planString = string(models.Free)
			user.Subscription = &planString
			// user.ExpirationDate = &t
			db.Save(&user)
			var companies []models.Company
			result = db.Where("owner_id = ?", userId).Find(&companies)
			if result.Error != nil {
				fmt.Println("Error getting user companies", result.Error)
				return echo.ErrInternalServerError
			}

			var companiesString = ""
			for index, company := range companies {
				// company.Subscription = models.Free
				companiesString += fmt.Sprintf("%s", company.Name)
				if index != len(companies)-1 {
					companiesString += ","
				}
				// db.Save(&company)
				// fmt.Println("No active sub/entitlements found for user , updating ", user.Name, company.Name)
			}

			msg := tgbotapi.NewMessage(-1002078967836, fmt.Sprintf("🛑 %s(%s)  %s reason %s", user.Name, companiesString, eventType, reason))
			if bot != nil {

				_, err := bot.Send(msg)

				if err != nil {
					fmt.Println(err)
				}
			}

			if reason == "UNSUBSCRIBE" {

				services.SendNotification(wc.FirebaseApp, db, user.ID, "Subscription cancelled", "Ready to take a survey for a discount just for one feedback? 🔥 sales@skripe.com. ", nil)
			} else if reason == "BILLING_ERROR" {
				services.SendNotification(wc.FirebaseApp, db, user.ID, "Payment error", "Please update your payment to keep your subscription active! 😮 ", nil)
			}

			return c.JSON(http.StatusOK, echo.Map{
				"message": "cancel ok",
			})
		}
		// if pro_plus_ok {

		// 	fmt.Println(pro_plus_entitlement["expires_date"])

		// 	expires, ok := pro_plus_entitlement["expires_date"].(string)
		// 	if !ok {
		// 		fmt.Println("Error parsing Pro plus expiration date")
		// 		return echo.ErrInternalServerError
		// 	}
		// 	t, err := time.Parse(time_layout, expires)

		// 	if err != nil {
		// 		fmt.Println(err)
		// 	}
		// 	fmt.Println(t, time.Now(), appUserId)
		// 	var planString = string(models.ProPlus)
		// 	user.Subscription = &planString
		// 	user.ExpirationDate = &t
		// 	db.Save(&user)
		// 	if t.After(time.Now()) {
		// 		var companies []models.Company
		// 		result := db.Where("owner_id = ?", userId).Find(&companies)
		// 		if result.Error != nil {
		// 			fmt.Println("Error getting user companies", result.Error)
		// 			return echo.ErrInternalServerError
		// 		}
		// 		var companiesString = ""
		// 		for index, company := range companies {
		// 			companiesString += fmt.Sprintf("%s", company.Name)
		// 			if index != len(companies)-1 {
		// 				companiesString += ","
		// 			}
		// 			company.Subscription = models.ProPlus
		// 			db.Save(&company)
		// 			fmt.Println("Sub Pro plus active for ", company.Name)
		// 		}
		// 		if eventType == "INITIAL_PURCHASE" {
		// 			msg := tgbotapi.NewMessage(-1002078967836, fmt.Sprintf("🎉⚡️🔥 %s(%s) subscribed for: %s ", user.Name, companiesString, string(models.ProPlus)))
		// 			if bot != nil {

		// 				_, err := bot.Send(msg)

		// 				if err != nil {
		// 					fmt.Println(err)
		// 				}
		// 			}
		// 		}
		// 		periodType, ok := event["period_type"].(string)
		// 		if ok && periodType == "PROMOTIONAL" {
		// 			services.SendNotification(wc.FirebaseApp, db, user.ID, "Promo activated 🎉", fmt.Sprintf("Your %s subscription is now active until %s", "Pro Plus", t.Format("2006-01-02")))
		// 		}
		// 		return c.JSON(http.StatusOK, echo.Map{
		// 			"message": "Pro Plus is active",
		// 		})
		// 	}
		// }
		if pro_ok {
			fmt.Println(pro_entitlement["expires_date"].(string))
			expires, ok := pro_entitlement["expires_date"].(string)

			if !ok {
				fmt.Println("Error parsing Pro expiration date")
				return echo.ErrInternalServerError
			}
			t, err := time.Parse(time_layout, expires)

			if err != nil {
				fmt.Println(err)
			}
			fmt.Println(t, time.Now(), appUserId)
			var planString = string(models.Pro)
			user.Subscription = &planString
			user.ExpirationDate = &t
			db.Save(&user)
			if t.After(time.Now()) {
				var companies []models.Company
				result := db.Where("owner_id = ?", userId).Find(&companies)
				if result.Error != nil {
					fmt.Println("Error getting user companies", result.Error)
					return echo.ErrInternalServerError
				}
				var companiesString = ""
				for index, company := range companies {
					companiesString += fmt.Sprintf("%s", company.Name)
					if index != len(companies)-1 {
						companiesString += ","
					}
					company.Subscription = models.Pro
					db.Save(&company)
					fmt.Println("Sub Pro  active for ", company.Name)
				}
				if eventType == "INITIAL_PURCHASE" {

					msg := tgbotapi.NewMessage(-1002078967836, fmt.Sprintf("🎉⚡️🔥 %s(%s) subscription update: %s ", user.Name, companiesString, string(models.Pro)))
					_, err := bot.Send(msg)

					if err != nil {
						fmt.Println(err)
					}
				}
				// db.Commit()
				periodType, ok := event["period_type"].(string)
				if ok && periodType == "PROMOTIONAL" {
					services.SendNotification(wc.FirebaseApp, db, user.ID, "Promo activated 🎉", fmt.Sprintf("Your %s subscription is now active until %s", "Pro", t.Format("2006-01-02")), nil)
				}
				return c.JSON(http.StatusOK, echo.Map{
					"message": "Pro is active",
				})
			}
		}

		fmt.Println("No active sub/entitlements found for user, updating backend sub ", appUserId)
		var planString = string(models.Free)
		user.Subscription = &planString
		// user.ExpirationDate = &t
		db.Save(&user)
		var companies []models.Company
		result = db.Where("owner_id = ?", userId).Find(&companies)
		if result.Error != nil {
			fmt.Println("Error getting user companies", result.Error)
			return echo.ErrInternalServerError
		}
		var companiesString = ""
		for index, company := range companies {
			companiesString += fmt.Sprintf("%s", company.Name)
			if index != len(companies)-1 {
				companiesString += ","
			}
			company.Subscription = models.Free
			db.Save(&company)
			fmt.Println("No active sub/entitlements found for user , updating ", user.Name, company.Name)
		}
		msg := tgbotapi.NewMessage(-1002078967836, fmt.Sprintf("⚠️ %s(%s) subscription updated : %s %s", user.Name, companiesString, string(models.Free), eventType))
		_, err = bot.Send(msg)

		if err != nil {
			fmt.Println(err)
		}

		if err != nil {
			fmt.Println(err)
		}
		// db.Commit()
		return c.JSON(http.StatusOK, echo.Map{
			"message": "OK ",
		})
	})
}
