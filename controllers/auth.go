package controllers

import (
	"context"
	"errors"
	"fmt"
	"letryapi/models"
	"letryapi/services"
	"letryapi/tasks"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	firebase "firebase.google.com/go/v4"
	apple "github.com/Timothylock/go-signin-with-apple/apple"
	"github.com/getsentry/sentry-go"
	"github.com/golang-jwt/jwt/v4"
	"github.com/hibiken/asynq"
	echojwt "github.com/labstack/echo-jwt"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type SetAvatarUploadFileRequest struct {
	FileName *string `json:"file_name" validate:"required,max=1000"`
}
type AuthController struct {
	Google      services.GoogleServiceProvider
	FirebaseApp *firebase.App
	AWSService  services.AWSServiceProvider
}

func (m *AuthController) ProfileRoutes(g *echo.Group) {
	g.POST("/google/v2", func(c echo.Context) (err error) {
		time.Sleep(2 * time.Second)
		googleCreds := new(models.GoogleAuthSignIn)

		// c.Request().Body
		signUp := new(models.SignUpIn)
		if c.QueryParam("testblock") == "true" {
			fmt.Println("Test block is enabled, returning 403")
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Sorry, your access is blocked"})
		}
		if c.QueryParam("verify") == "true" {
			if err := c.Bind(googleCreds); err != nil {
				return err
			}
			if !models.ValidatePlatformRaw(googleCreds.Platform) {
				return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Please provide proper platform parameter"})
			}

			if err = c.Validate(googleCreds); err != nil {
				return err
			}
		} else {
			if err := c.Bind(signUp); err != nil {
				return err
			}

			if !models.ValidatePlatformRaw(signUp.Platform) {
				return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Please provide proper platform parameter"})
			}

			if err = c.Validate(signUp); err != nil {
				return err
			}
		}
		idToken := IfThenElse(googleCreds.IdToken == "", signUp.IdToken, googleCreds.IdToken).(string)
		platform := IfThenElse(googleCreds.Platform == "", signUp.Platform, googleCreds.Platform).(string)
		payload, err := m.Google.ValidateIdToken(context.Background(), idToken, os.Getenv("GOOGLE_CLIENT_ID"))

		if err != nil {
			fmt.Println(err)
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Couldn't verify credentials"})
		}
		sub, ok := payload.Claims["sub"]
		if !ok {
			sentry.CaptureMessage(fmt.Sprintf("Error when fetching user data %s", payload.Claims))
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Couldn't verify credentials"})
		}
		var googleId string = sub.(string)

		googleEmail, ok := payload.Claims["email"]
		if !ok {
			sentry.CaptureMessage(fmt.Sprintf("Error when fetching user data email %s", payload.Claims))
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Couldn't verify credentials"})
		}

		pictureUrl, ok := payload.Claims["picture"].(string)
		googleName, ok := payload.Claims["name"].(string)

		db := c.Get("__db").(*gorm.DB)
		var user *models.UserAccount
		r := db.Preload("Memberships.Company").Where("google_id = ?", googleId).Limit(1).Find(&user)
		if r.Error != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{"message": "Internal server error"})
		}
		fmt.Println(r)
		fmt.Println(c.QueryParam("verify"))
		if c.QueryParam("verify") == "true" {
			if r.RowsAffected > 0 {
				if user.Banned {
					return echo.ErrForbidden
				}
				memberships := user.Memberships
				var role string
				var companyName string
				var company models.Company
				if len(memberships) > 0 {
					company = user.Memberships[0].Company
					role = string(user.Memberships[0].Role)
				}
				refreshToken, err := GenerateRefreshToken(fmt.Sprint(user.ID))
				if err != nil {
					fmt.Println(err)
					return echo.ErrInternalServerError
				}
				fmt.Println("Admin access: ", company.FullAdminAccess)

				return c.JSON(http.StatusOK, map[string]interface{}{
					"company": models.CompanyInfoOut{
						Id:               company.ID,
						OwnerId:          company.OwnerID,
						Active:           company.Active,
						Subscription:     company.Subscription,
						TrialStartedDate: company.TrialStartedDate,
						TrialDays:        company.TrialDays,
						FullAdminAccess:  company.FullAdminAccess,
					},
					"company_id":   company.ID,
					"company_name": companyName,
					"id":           user.ID,
					"name":         user.Name,
					"role":         role,
					"was_invited":  user.Status == "INVITATION_PENDING",
					"email":        googleEmail, "new": user.Status == "STARTED_AUTH" || user.Status == "INVITATION_PENDING", "avatar": user.AvatarURL,
					"access_token":  GenerateUserToken(fmt.Sprint(user.ID), c, 72),
					"refresh_token": refreshToken,
				})
			} else {
				// var existingUserByEmail models.UserAccount
				r := db.Where("email = ?", googleEmail).Limit(1).Find(&user)
				if r.RowsAffected > 0 {
					user.AvatarURL = pictureUrl
					user.GoogleID = googleId
					user.Name = googleName
					user.LastIp = c.RealIP()
					user.Platform = models.ScanPlatform(platform)
					db.Save(&user)
					refreshToken, err := GenerateRefreshToken(fmt.Sprint(user.ID))
					if err != nil {
						fmt.Println(err)
						return echo.ErrInternalServerError
					}

					memberships := user.Memberships
					var role string
					if len(memberships) > 0 {
						role = string(user.Memberships[0].Role)
					}
					return c.JSON(http.StatusOK, map[string]interface{}{
						"email": googleEmail,
						"new":   user.Status == "STARTED_AUTH" || user.Status == "INVITATION_PENDING", "avatar": user.AvatarURL,
						"role":          role, // TODO FIX
						"was_invited":   user.Status == "INVITATION_PENDING",
						"name":          googleName,
						"access_token":  GenerateUserToken(fmt.Sprint(user.ID), c, 72),
						"refresh_token": refreshToken,
					})
				} else {

					user = &models.UserAccount{
						Name:      googleName,
						Email:     googleEmail.(string),
						GoogleID:  googleId,
						Platform:  models.ScanPlatform(platform),
						LastIp:    c.RealIP(),
						Status:    "STARTED_AUTH",
						AvatarURL: pictureUrl,
					}
					db.Create(&user)
				}
			}
			refreshToken, err := GenerateRefreshToken(fmt.Sprint(user.ID))
			if err != nil {
				fmt.Println(err)
				return echo.ErrInternalServerError
			}
			memberships := user.Memberships
			var role string
			if len(memberships) > 0 {
				role = string(user.Memberships[0].Role)
			}

			// this i guess on new auth only
			return c.JSON(http.StatusOK, map[string]interface{}{
				"email": googleEmail,
				"new":   r.RowsAffected == 0 || user.Status == "STARTED_AUTH" || user.Status == "INVITATION_PENDING", "avatar": user.AvatarURL,
				"role":          role,
				"was_invited":   user.Status == "INVITATION_PENDING",
				"name":          user.Name,
				"access_token":  GenerateUserToken(fmt.Sprint(user.ID), c, 72),
				"refresh_token": refreshToken,
			})
		}
		if r.RowsAffected > 0 {

			user.Name = signUp.Name
			user.Status = "FINISHED_AUTH"
			user.UTMSource = signUp.UTMSource
			// var company *models.Company
			company := &models.Company{
				Name:         signUp.Company,
				OwnerID:      user.ID,
				Subscription: models.Free,
				// TrialStartedDate:
				TrialDays: UIntPointer(14),
				Currency:  "₼",
				Language:  "az",
				Active:    true,
			}
			db.Create(&company)
			var user_membership = &models.UserCompanyRole{
				CompanyID:     company.ID,
				UserAccountID: user.ID,
				Active:        true,
				Role:          "OWNER",
			}
			db.Save(&user)

			db.Save(&user_membership)
			fmt.Println("User onboarding finished google: ", googleEmail, googleId)
			return c.JSON(http.StatusOK, map[string]interface{}{
				"company_id":   company.ID,
				"company_name": company.Name,
				"company": models.CompanyInfoOut{
					Id:               company.ID,
					OwnerId:          company.OwnerID,
					Active:           company.Active,
					Subscription:     company.Subscription,
					TrialStartedDate: company.TrialStartedDate,
					TrialDays:        company.TrialDays,
					FullAdminAccess:  company.FullAdminAccess,
				},
				// "subscription": nil,
				"id":           user.ID,
				"name":         user.Name,
				"email":        user.Email,
				"role":         string(user_membership.Role),
				"new":          true,
				"avatar":       user.AvatarURL,
				"access_token": GenerateUserToken(fmt.Sprint(user.ID), c, 72),
			})
		} else {
			// sentry.CaptureMessage(fmt.Sprintf("Error when finishing user creation, no user found in database %s %s", googleEmail, googleId))
			c.Logger().Warnf("Error when finishing user creation, no user found in database %s %s", googleEmail, googleId)
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{"message": "Sorry, something wrong happened, please try again!"})
		}

	})

	g.POST("/apple", func(c echo.Context) error {
		var req models.AppleAuthRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		}

		signUp := new(models.SignUpIn)
		// Your 10-character Team ID
		teamID := "VUWPX5QW93"
		keyID := "5LCB7TR346"
		// ClientID is the "Services ID" value that you get when navigating to your "sign in with Apple"-enabled service ID
		clientID := "com.skripe.leitnerai"

		// Find the 10-char Key ID value from the portal

		// The contents of the p8 file/key you downloaded when you made the key in the portal
		secret, err := services.DecodeBase64EnvPrivateKey("APPLE_SIGNIN_PKEY_BASE64")

		if err != nil {
			log.Println("Error getting Apple private key:", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
		}
		secret, err = apple.GenerateClientSecret(secret, teamID, clientID, keyID)
		// Create Apple client
		client := apple.New()

		// Verify the token
		vReq := apple.AppValidationTokenRequest{
			ClientID:     clientID,
			ClientSecret: secret,
			Code:         req.AuthorizationCode,
		}

		var resp apple.ValidationResponse

		// Do the verification
		err = client.VerifyAppToken(context.Background(), vReq, &resp)
		if err != nil {
			fmt.Println("error verifying: " + err.Error())
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Couldn't verify credentials"})
		}

		if resp.Error != "" {
			fmt.Printf("apple returned an error: %s - %s\n", resp.Error, resp.ErrorDescription)
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Couldn't verify credentials through Apple"})
		}

		// Get the unique user ID
		unique, err := apple.GetUniqueID(resp.IDToken)
		if err != nil {
			fmt.Println("failed to get unique ID: " + err.Error())
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Couldn't get your unique identifier"})
		}

		// Get the email
		claim, err := apple.GetClaims(resp.IDToken)
		if err != nil {
			fmt.Println("failed to get claims: " + err.Error())
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Couldn't get your information"})
		}

		appleEmail, ok := (*claim)["email"].(string)
		emailVerified, verifiedOk := (*claim)["email_verified"].(bool)
		isPrivateEmail, isPrivateEmailOk := (*claim)["is_private_email"].(bool)
		fmt.Println("[Apple signin] email:", appleEmail, " verified:", emailVerified, " private:", isPrivateEmail, resp.IDToken)
		if !ok {
			fmt.Println(fmt.Sprintf("[Apple signin] no email in token  %s", claim))
		}

		if !verifiedOk || !isPrivateEmailOk {
			log.Println("[Apple signin] Email not verified or is_private_email missing from claims")
		}
		platform := IfThenElse(req.Platform == "", req.Platform, req.Platform).(string)
		var appleId string = unique

		db := c.Get("__db").(*gorm.DB)
		var user *models.UserAccount

		var r *gorm.DB
		if appleEmail == "" {
			r = db.Preload("Memberships.Company").Where("apple_id = ?", appleId).Limit(1).Find(&user)
		} else {
			r = db.Preload("Memberships.Company").Where("apple_id = ? or email = ?", appleId, appleEmail).Limit(1).Find(&user)

		}
		if r.Error != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{"message": "Internal server error"})
		}
		fmt.Println(r)
		fmt.Println(c.QueryParam("verify"))
		if c.QueryParam("verify") == "true" {
			if r.RowsAffected > 0 {
				if user.Banned {
					return echo.ErrForbidden
				}
				memberships := user.Memberships
				var role string
				var companyName string
				var company models.Company
				if len(memberships) > 0 {
					company = user.Memberships[0].Company
					role = string(user.Memberships[0].Role)
				}
				refreshToken, err := GenerateRefreshToken(fmt.Sprint(user.ID))
				if err != nil {
					fmt.Println(err)
					return echo.ErrInternalServerError
				}
				fmt.Println("Admin access: ", company.FullAdminAccess)
				user.AppleID = appleId
				db.Save(&user)
				return c.JSON(http.StatusOK, map[string]interface{}{
					"company": models.CompanyInfoOut{
						Id:               company.ID,
						OwnerId:          company.OwnerID,
						Active:           company.Active,
						Subscription:     company.Subscription,
						TrialStartedDate: company.TrialStartedDate,
						TrialDays:        company.TrialDays,
						FullAdminAccess:  company.FullAdminAccess,
					},
					"company_id":   company.ID,
					"company_name": companyName,
					"id":           user.ID,
					"name":         user.Name,
					"role":         role,
					"was_invited":  user.Status == "INVITATION_PENDING",
					"email":        appleEmail, "new": user.Status == "STARTED_AUTH" || user.Status == "INVITATION_PENDING", "avatar": user.AvatarURL,
					"access_token":  GenerateUserToken(fmt.Sprint(user.ID), c, 72),
					"refresh_token": refreshToken,
				})
			} else {
				// var existingUserByEmail models.UserAccount
				if appleEmail == "" {
					fmt.Println("[Apple signin] New user but no email in claims:", resp.IDToken)
					return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "It seems that you are sign in the first time and no email provided by Apple. Please try it again or contact us at support@skripe.com."})
				}
				r := db.Where("email = ?", appleEmail).Limit(1).Find(&user)
				if r.RowsAffected > 0 {
					if user.AvatarURL == "" {

						user.AvatarURL = "https://pub-df730af6a36c46a58d6d948f149dae31.r2.dev/user-circle.png"
					}
					user.AppleID = appleId
					if user.Name == "" && appleEmail != "" {

						user.Name = appleEmail
					}
					user.LastIp = c.RealIP()

					user.Platform = models.ScanPlatform(platform)
					db.Save(&user)
					refreshToken, err := GenerateRefreshToken(fmt.Sprint(user.ID))
					if err != nil {
						fmt.Println(err)
						return echo.ErrInternalServerError
					}

					memberships := user.Memberships
					var role string
					if len(memberships) > 0 {
						role = string(user.Memberships[0].Role)
					}
					return c.JSON(http.StatusOK, map[string]interface{}{
						"email": appleEmail,
						"new":   user.Status == "STARTED_AUTH" || user.Status == "INVITATION_PENDING", "avatar": user.AvatarURL,
						"role":          role, // TODO FIX
						"was_invited":   user.Status == "INVITATION_PENDING",
						"name":          appleEmail,
						"access_token":  GenerateUserToken(fmt.Sprint(user.ID), c, 72),
						"refresh_token": refreshToken,
					})
				} else {

					user = &models.UserAccount{
						Name:      appleEmail,
						Email:     appleEmail,
						AppleID:   appleId,
						Platform:  models.ScanPlatform(platform),
						LastIp:    c.RealIP(),
						Status:    "STARTED_AUTH",
						AvatarURL: "https://pub-df730af6a36c46a58d6d948f149dae31.r2.dev/user-circle.png",
					}
					db.Create(&user)
				}
			}
			refreshToken, err := GenerateRefreshToken(fmt.Sprint(user.ID))
			if err != nil {
				fmt.Println(err)
				return echo.ErrInternalServerError
			}
			memberships := user.Memberships
			var role string
			if len(memberships) > 0 {
				role = string(user.Memberships[0].Role)
			}

			// this i guess on new auth only
			return c.JSON(http.StatusOK, map[string]interface{}{
				"email": appleEmail,
				"new":   r.RowsAffected == 0 || user.Status == "STARTED_AUTH" || user.Status == "INVITATION_PENDING", "avatar": user.AvatarURL,
				"role":          role,
				"was_invited":   user.Status == "INVITATION_PENDING",
				"name":          user.Name,
				"access_token":  GenerateUserToken(fmt.Sprint(user.ID), c, 72),
				"refresh_token": refreshToken,
			})
		}
		if r.RowsAffected > 0 {
			// user.Name = googleSignUp.Name

			user.Status = "FINISHED_AUTH"
			user.UTMSource = signUp.UTMSource
			// var company *models.Company
			company := &models.Company{
				Name:         signUp.Company,
				OwnerID:      user.ID,
				Subscription: models.Free,
				// TrialStartedDate:
				TrialDays: UIntPointer(14),
				Currency:  "₼",
				Language:  "az",
				Active:    true,
			}
			db.Create(&company)
			var user_membership = &models.UserCompanyRole{
				CompanyID:     company.ID,
				UserAccountID: user.ID,
				Active:        true,
				Role:          "OWNER",
			}
			db.Save(&user)

			db.Save(&user_membership)
			fmt.Println("User onboarding finished apple: ", appleEmail, appleId)
			return c.JSON(http.StatusOK, map[string]interface{}{
				"company_id":   company.ID,
				"company_name": company.Name,
				"company": models.CompanyInfoOut{
					Id:               company.ID,
					OwnerId:          company.OwnerID,
					Active:           company.Active,
					Subscription:     company.Subscription,
					TrialStartedDate: company.TrialStartedDate,
					TrialDays:        company.TrialDays,
					FullAdminAccess:  company.FullAdminAccess,
				},
				// "subscription": nil,
				"id":           user.ID,
				"name":         user.Name,
				"email":        user.Email,
				"role":         string(user_membership.Role),
				"new":          true,
				"avatar":       user.AvatarURL,
				"access_token": GenerateUserToken(fmt.Sprint(user.ID), c, 72),
			})
		} else {
			// sentry.CaptureMessage(fmt.Sprintf("Error when finishing user creation, no user found in database %s %s", googleEmail, googleId))
			c.Logger().Warnf("Error when finishing user creation, no user found in database %s %s", appleEmail, appleId)
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{"message": "Sorry, something wrong happened, please try again!"})
		}
	})

	g.POST("/apple/finish", func(c echo.Context) error {
		var req models.ProfileIn
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		}
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		if user.ID < 1 {
			return echo.ErrForbidden
		}
		if user.Status == "FINISHED_AUTH" {
			return echo.ErrBadRequest
		}
		// user.Name = googleSignUp.Name
		user.Status = "FINISHED_AUTH"
		user.UTMSource = req.UTMSource
		// var company *models.Company
		company := &models.Company{
			Name:         req.Company,
			OwnerID:      user.ID,
			Subscription: models.Free,
			// TrialStartedDate:
			TrialDays: UIntPointer(14),
			Currency:  "₼",
			Language:  "az",
			Active:    true,
		}
		db.Create(&company)
		var user_membership = &models.UserCompanyRole{
			CompanyID:     company.ID,
			UserAccountID: user.ID,
			Active:        true,
			Role:          "OWNER",
		}
		db.Save(&user)

		db.Save(&user_membership)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"company_id":   company.ID,
			"company_name": company.Name,
			"company": models.CompanyInfoOut{
				Id:               company.ID,
				OwnerId:          company.OwnerID,
				Active:           company.Active,
				Subscription:     company.Subscription,
				TrialStartedDate: company.TrialStartedDate,
				TrialDays:        company.TrialDays,
				FullAdminAccess:  company.FullAdminAccess,
			},
			// "subscription": nil,
			"id":           user.ID,
			"name":         user.Name,
			"email":        user.Email,
			"role":         string(user_membership.Role),
			"new":          true,
			"avatar":       user.AvatarURL,
			"access_token": GenerateUserToken(fmt.Sprint(user.ID), c, 72),
		})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), NoMembershipUserMiddleware)

	g.POST("/refresh-token", func(c echo.Context) error {
		type tokenReqBody struct {
			RefreshToken string `json:"refresh_token"`
		}
		tokenReq := new(tokenReqBody)

		if err := c.Bind(&tokenReq); err != nil {
			fmt.Println(err)
			return echo.ErrBadRequest
		}

		if tokenReq.RefreshToken == "" {
			fmt.Println("Refresh token is empty")
			return echo.ErrBadRequest
		}
		// Parse takes the token string and a function for looking up the key.
		// The latter is especially useful if you use multiple keys for your application.
		// The standard is to use 'kid' in the head of the token to identify
		// which key to use, but the parsed token (head and claims) is provided
		// to the callback, providing flexibility.
		token, err := jwt.Parse(tokenReq.RefreshToken, func(token *jwt.Token) (interface{}, error) {
			// Don't forget to validate the alg is what you expect:
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				fmt.Println("process?", ok)
				return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}
			// hmacSampleSecret is a []byte containing your secret, e.g. []byte("my_secret_key")
			return []byte(os.Getenv("JWT_SECRET")), nil
		})
		if err != nil {
			fmt.Println(err)
			return echo.ErrBadRequest
		}
		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			db := c.Get("__db").(*gorm.DB)
			// Get the user record from database or
			// run through your business logic to verify if the user can log in
			data, errConvert := claims["sub"].(string)
			if !errConvert {
				fmt.Println("Cannot convert sub to string!", err)
				return echo.ErrInternalServerError
			}
			userId, err := strconv.Atoi(data)
			if err != nil {
				// todo sentry
				fmt.Println("Error parsing sub of the user!!", err)
				return echo.ErrInternalServerError
			}
			if userId < 1 {
				fmt.Println("Refresh: sub is:", userId)
				return echo.ErrBadRequest
			}
			var user *models.UserAccount
			result := db.First(&user, userId)
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				// todo sentry
				fmt.Println("Requested user not found!", userId)
				if user == nil {
					return echo.ErrForbidden
				}
			}
			if result.Error != nil {
				// todo sentry
				fmt.Println("Error getting user while refreshing token", userId)
				return echo.ErrInternalServerError
			}
			if !user.Banned {

				t := GenerateUserToken(fmt.Sprint(userId), c, 72)
				rt, err := GenerateRefreshToken(fmt.Sprint(userId))

				if err != nil {
					fmt.Println("Error refreshing token ", err)
					return echo.ErrInternalServerError
				}
				// newTokenPair, err := generateTokenPair()

				return c.JSON(http.StatusOK, echo.Map{
					"access_token":  t,
					"refresh_token": rt,
				})
			}

			return echo.ErrUnauthorized
		}

		return err
	})

	g.GET("/my/companies", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var invites []models.UserCompanyRole
		db.Where(" user_account_id = ?", user.ID).Joins("Company").Find(&invites)

		var companies = []models.CompanyInfoRoleV2Out{}

		for _, memberships := range invites {
			companies = append(companies, models.CompanyInfoRoleV2Out{
				Role: string(memberships.Role),
				CompanyInfoOut: models.CompanyInfoOut{

					Name:             memberships.Company.Name,
					OwnerId:          memberships.Company.OwnerID,
					Id:               memberships.CompanyID,
					Active:           memberships.Active,
					Subscription:     memberships.Company.Subscription,
					TrialStartedDate: memberships.Company.TrialStartedDate,
					TrialDays:        memberships.Company.TrialDays,
				},
			})
		}
		return c.JSON(http.StatusOK, echo.Map{
			"invites": companies,
		})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.GET("/me", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var companyDb models.Company
		r := db.Limit(1).Find(&companyDb, "id = ?", user.Memberships[0].CompanyID)
		if r.RowsAffected == 0 {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Something went wrong",
			})
		}

		var companies = []models.CompanyInfoRoleV2Out{}

		for _, memberships := range user.Memberships {
			companies = append(companies, models.CompanyInfoRoleV2Out{
				Role: string(memberships.Role),
				CompanyInfoOut: models.CompanyInfoOut{

					Name:             memberships.Company.Name,
					Id:               memberships.CompanyID,
					OwnerId:          memberships.Company.OwnerID,
					Active:           memberships.Active,
					Subscription:     memberships.Company.Subscription,
					TrialStartedDate: memberships.Company.TrialStartedDate,
					TrialDays:        memberships.Company.TrialDays,
				},
			})
		}
		fullbodyAvatarUrl := user.UserFullBodyImageURL
		if user.UserFullBodyImageURL != nil && *user.UserFullBodyImageURL != "" {

			bucketName := services.GetEnv("R2_BUCKET_NAME", "") // Assuming you have a way to get this
			avatarR2URL, err := m.AWSService.GetPresignedR2FileReadURL(context.
				Background(), bucketName, *user.UserFullBodyImageURL,
			)

			if err != nil {
				// The fallback also failed. This is a critical error.
				log.Printf("CRITICAL:  R2 avatar could not fetch for key '%s': %v", *user.UserFullBodyImageURL, err)
				sentry.CaptureException(err)
				// imageUrl remains empty, but we don't fail the entire request.
			}
			fullbodyAvatarUrl = &avatarR2URL
		}
		return c.JSON(http.StatusOK, models.UserMeInfoV2Out{
			Name:                                 user.Name,
			MyCompanies:                          companies,
			Email:                                user.Email,
			Status:                               user.Status,
			AvatarURL:                            user.AvatarURL,
			FullBodyAvatarUrl:                    fullbodyAvatarUrl,
			FullBodyAvatarSet:                    user.FullBodyAvatarSet,
			FullBodyAvatarStatus:                 user.FullBodyAvatarStatus,
			FullBodyAvatarProcessingErrorMessage: user.FullBodyAvatarProcessingErrorMessage,
			ReceiveSalesNotifications:            user.ReceiveNotifications,
		})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.POST("/settings", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		var settingsIn = new(models.UserSettingsIn)
		db := c.Get("__db").(*gorm.DB)
		if err := c.Bind(settingsIn); err != nil {
			return err
		}
		// TODO
		// if string(user.Memberships[0].Company.Subscription) == "free" {
		// 	return echo.ErrForbidden
		// }
		user.ReceiveNotifications = settingsIn.ReceiveSalesNotifications
		db.Save(&user)
		// user.ReceiveSalesNotifications = settingsIn.ReceiveSalesNotifications
		return c.JSON(http.StatusOK, settingsIn)

	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.GET("/latest-banner", func(c echo.Context) error {

		return c.JSON(http.StatusOK, echo.Map{
			"key":        "20_off_test",
			"has_action": false,
			"az": echo.Map{
				"text":        "İndi abunə ol və 20% endirim qazan",
				"actionTitle": "Yararlan",
			},
			"en": echo.Map{
				"text":        "Special offer for you, subscribe now to get 20% off",
				"actionTitle": "Get 20% off",
			},
		})

	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.POST("/register-push", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var tokenRequest = new(models.UserPushIn)

		if err := c.Bind(tokenRequest); err != nil {
			return err
		}

		if !models.ValidatePlatformRaw(string(tokenRequest.Platform)) {
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Please provide proper platform parameter"})
		}
		var pushData models.UserPushToken = models.UserPushToken{
			Platform:      models.ScanPlatform(tokenRequest.Platform),
			Token:         tokenRequest.Token,
			UserAccountID: user.ID,
			Active:        true,
		}

		// same token/device can sign in to diff accs and still receive pushes.
		// we try to delete other session but we cannot errly on that
		result := db.Where("token = ? and user_account_id = ?", tokenRequest.Token, user.ID).FirstOrCreate(&pushData)
		if result.Error != nil {
			log.Println(result.Error)
			return echo.ErrInternalServerError
		}
		if result.RowsAffected >= 1 {
			fmt.Println("Token created for user ", user.ID, "Platform: ", tokenRequest.Platform)
		}
		fmt.Println("Push id ", pushData.ID, " Token ", pushData.Token, " Platform: ", pushData.Platform, "User ID:", pushData.UserAccountID)
		return c.JSON(http.StatusOK, echo.Map{
			"message": "registered",
			"push_id": pushData.ID,
		})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.POST("/delete-push", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var tokenRequest = new(models.UserPushIn)

		if err := c.Bind(tokenRequest); err != nil {
			return err
		}

		if !models.ValidatePlatformRaw(string(tokenRequest.Platform)) {
			return c.JSON(http.StatusForbidden, map[string]interface{}{"message": "Please provide proper platform parameter"})
		}

		// same token/device can sign in to diff accs and still receive pushes.
		result := db.Where("token = ? and user_account_id = ? and platform = ?", tokenRequest.Token, user.ID, tokenRequest.Platform).Delete(&models.UserPushToken{})
		if result.Error != nil {
			log.Println(result.Error)
			return echo.ErrInternalServerError
		}
		if result.RowsAffected >= 1 {
			fmt.Println("Token deleted for user ", user.ID, "Platform: ", tokenRequest.Platform)
		}
		return c.JSON(http.StatusOK, echo.Map{
			"message": "deleted",
			"deleted": result.RowsAffected > 0,
		})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.POST("/logout", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var tokenRequest = new(models.UserPushIn)
		if err := c.Bind(tokenRequest); err != nil {
			return err
		}

		db.Delete("user_account_id = ? and token = ?", user.ID, tokenRequest.Token)

		return c.JSON(http.StatusOK, echo.Map{
			"message": "logged out",
		})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.POST("/delete-account", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		now := time.Now()
		user.ConfirmedDeleteDate = &now
		db.Save(user)
		// db.Delete("user_account_id = ? and token = ?", user.ID, tokenRequest.Token)
		return c.JSON(http.StatusOK, echo.Map{
			"message": "logged out",
		})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)

	g.POST("/set-avatar", func(c echo.Context) error {
		// Get user from context
		user, ok := c.Get("currentUser").(models.UserAccount)
		if !ok {
			return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
		}
		var req SetAvatarUploadFileRequest
		if err := c.Bind(&req); err != nil {
			fmt.Println(err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}

		// Validate request
		if err := c.Validate(req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		// Parse and validate noteId

		// Get database from context
		db, ok := c.Get("__db").(*gorm.DB)
		if !ok {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Our service is not available, please try again a bit later"})
		}

		asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
		if !ok {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
		}

		// Find note and verify ownership
		// Return success response
		var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
		// todo clean and map the same file name as in FE UI otherwise **FAIL**
		safeFileName := fmt.Sprintf("fullbodyavatars/%v/%s", user.ID, *req.FileName)

		// uploadUrl, presignErr = controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
		// fileName := strings.ReplaceAll(*req.FileName, " ", "")
		// fileName = strings.ReplaceAll(*req.FileName, "-", "")
		// safeFileName := fmt.Sprintf("notes/%s", fileName)
		uploadUrl, presignErr := m.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
		if presignErr != nil {
			log.Printf("Unable to presign generate for avatar upload %s!, %s", user.Name, presignErr)
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Error while uploading your avatar, please try again",
			})
		}
		user.UserFullBodyImageURL = &safeFileName
		user.FullBodyAvatarStatus = "processing"
		fmt.Println("Presetting user avatar url to ", safeFileName)

		task, err := tasks.NewFullBodyAvatarGenerateTask(user.ID)
		if err != nil {
			sentry.CaptureException(err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Sorry, could not process avatar, please try again"})
		}
		info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Queue("generate"))
		if err != nil {
			sentry.CaptureException(err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Sorry, could not process avatar, please try again"})
		}
		fmt.Printf("[Queue] Process Avatar %s task submitted, User ID: %v Task ID %v ", safeFileName, user.ID, info.ID)
		if err := db.Save(&user).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to save your avatar"})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "Avatar is updated successfully", "upload_url": uploadUrl, "processing_status": user.FullBodyAvatarStatus, "file_name": *req.FileName})
	}, echojwt.JWT([]byte(os.Getenv("JWT_SECRET"))), UserMiddleware)
}
