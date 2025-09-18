package controllers

import (
	"fmt"
	"letryapi/models"
	"letryapi/services"
	"net/http"
	"net/mail"
	"time"

	firebase "firebase.google.com/go/v4"
	"github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type CompanyController struct {
	AWSService  services.AWSServiceProvider
	FirebaseApp *firebase.App
}

func (controller *CompanyController) CompanyRoutes(g *echo.Group) {

	g.GET("/overview", func(c echo.Context) error {
		companyId := c.Get("currentCompany").(uint)
		db := c.Get("__db").(*gorm.DB)
		var currentCompany models.Company
		r := db.Preload("Members.UserAccount", func(db *gorm.DB) *gorm.DB {
			return db.Order("user_accounts.id asc")
		}).Limit(1).Find(&currentCompany, "id = ?", companyId)
		if r.RowsAffected == 0 {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Something happened",
			})
		}
		// var models
		var members []models.MemberInfoOut

		allowedLLMInfo := currentCompany.FullAdminAccess
		var dailyNoteCount int64
		// if currentCompany.EnforcedDailyNoteLimit == nil {

		today := time.Now().UTC().Format("2006-01-02")
		if err := db.Model(&models.ClothingTryonGeneration{}).Where("company_id = ? AND DATE(created_at) = ?", companyId, today).Count(&dailyNoteCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get note data"})
		}
		var totalGenerationCount int64
		if err := db.Model(&models.ClothingTryonGeneration{}).Where("company_id = ?", companyId).Count(&totalGenerationCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get note data"})
		}
		// }
		var enforcedLlmModel *int32
		if allowedLLMInfo {
			enforcedLlmModel = currentCompany.EnforcedLLMModel
		}
		return c.JSON(http.StatusOK, &models.CompanyOverviewOut{
			Name:                   currentCompany.Name,
			Address:                currentCompany.Address,
			ImageUrl:               currentCompany.ImageUrl,
			Subscription:           string(currentCompany.Subscription),
			TodayCreatedNotesCount: &dailyNoteCount,
			DefaultDailyNoteLimit:  2,
			DefaulTotalNoteLimit:   2,
			TotalCreatedNotesCount: &totalGenerationCount,
			Members:                members,
			OwnerID:                currentCompany.OwnerID,
			Currency:               currentCompany.Currency,
			Language:               currentCompany.Language,
			LLMModel:               enforcedLlmModel,
			FullAdminAccess:        currentCompany.FullAdminAccess,
		})
	})

	g.POST("/update", func(c echo.Context) error {
		companyId := c.Get("currentCompany").(uint)
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var currentCompany models.Company
		r := db.Preload("Members.UserAccount", func(db *gorm.DB) *gorm.DB {
			return db.Order("user_accounts.id asc")
		}).Limit(1).Find(&currentCompany, "id = ?", companyId)
		if r.RowsAffected == 0 {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Something happened",
			})
		}
		fmt.Println("Updating company data")
		companyUpdateData := new(models.CompanyUpdateIn)

		if err := c.Bind(&companyUpdateData); err != nil {
			fmt.Println("Fail to bind data")
			fmt.Println(err)
			return echo.ErrBadRequest
		}
		if companyUpdateData.Name != nil {
			currentCompany.Name = *companyUpdateData.Name
		}
		if companyUpdateData.Language != nil {

			currentCompany.Language = *companyUpdateData.Language
		}
		allowedLllmInfo := currentCompany.FullAdminAccess
		if companyUpdateData.LLMModel != nil {
			if !allowedLllmInfo {
				fmt.Println("User tried to update LLM model without permission!", user.Email, companyUpdateData.LLMModel)
				sentry.CaptureException(fmt.Errorf("User %s tried to update LLM model to %v without permission!", user.Email, companyUpdateData.LLMModel))
				return c.JSON(http.StatusForbidden, echo.Map{
					"message": "Bad request",
				})
			}
			currentCompany.EnforcedLLMModel = companyUpdateData.LLMModel
		}
		// currentCompany.Address = companyUpdateData.Address
		// currentCompany.BusinessPhone = companyUpdateData.BusinessPhone
		// currentCompany.WhatsAppNumber = companyUpdateData.WhatsAppNumber
		// currentCompany.InstagramURL = companyUpdateData.InstagramURL
		// currentCompany.FacebookURL = companyUpdateData.FacebookURL
		// currentCompany.TiktokURL = companyUpdateData.TiktokURL
		// currentCompany.LocationPin = companyUpdateData.LocationPin
		// currentCompany.IsOnlineShopEnabled = companyUpdateData.IsOnlineShopEnabled
		// currentCompany.ShopCustomSubDomain = companyUpdateData.ShopCustomSubDomain
		// currentCompany.IsShopPrivate = companyUpdateData.IsShopPrivate

		db.Save(&currentCompany)
		return c.JSON(http.StatusOK, &models.CompanyOverviewOut{
			Name:         currentCompany.Name,
			Address:      currentCompany.Address,
			ImageUrl:     currentCompany.ImageUrl,
			Subscription: string(currentCompany.Subscription),
			OwnerID:      currentCompany.OwnerID,
			Currency:     currentCompany.Currency,
			Language:     currentCompany.Language,
		})
	})

	g.GET("/members", func(c echo.Context) error {
		// user := c.Get("currentUser").(models.UserAccount)
		companyId := c.Get("currentCompany").(uint)
		db := c.Get("__db").(*gorm.DB)
		var currentCompany models.Company
		if string(currentCompany.Subscription) == "free" {
			return echo.ErrForbidden
		}
		r := db.Preload("Members.UserAccount", func(db *gorm.DB) *gorm.DB {
			return db.Order("user_accounts.id asc")
		}).Limit(1).Find(&currentCompany, "id = ?", companyId)
		if r.RowsAffected == 0 {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Something happened",
			})
		}
		// var models
		var members []models.MemberInfoOut
		for _, member := range currentCompany.Members {
			memberUser := member.UserAccount
			// fmt.Println("Member: ", memberUser.Name, memberUser.Status, memberUser)
			members = append(members, models.MemberInfoOut{
				Active: member.Active,
				Role:   member.Role,
				UserInfo: models.UserInfoOut{
					Id:          memberUser.ID,
					Name:        memberUser.Name,
					CompanyName: currentCompany.Name,
					Email:       memberUser.Email,
					Status:      memberUser.Status,
					AvatarURL:   memberUser.AvatarURL,
				},
				InviteCode: member.InviteCode,
			})
		}
		return c.JSON(http.StatusOK, members)
	})

	g.GET("/members-by-id", func(c echo.Context) error {
		// user := c.Get("currentUser").(models.UserAccount)
		companyId := c.Get("currentCompany").(uint)
		db := c.Get("__db").(*gorm.DB)
		var currentCompany models.Company
		if string(currentCompany.Subscription) == "free" {
			return echo.ErrForbidden
		}
		r := db.Preload("Members.UserAccount", func(db *gorm.DB) *gorm.DB {
			return db.Order("user_accounts.id asc")
		}).Limit(1).Find(&currentCompany, "id = ?", companyId)
		if r.RowsAffected == 0 {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Something happened",
			})
		}
		// var models
		var members map[uint]models.MemberInfoOut = make(map[uint]models.MemberInfoOut)
		for _, member := range currentCompany.Members {
			memberUser := member.UserAccount
			// fmt.Println("Member: ", memberUser.Name, memberUser.Status, memberUser)
			members[memberUser.ID] = models.MemberInfoOut{
				Active: member.Active,
				Role:   member.Role,
				UserInfo: models.UserInfoOut{
					Id:          memberUser.ID,
					Name:        memberUser.Name,
					CompanyName: currentCompany.Name,
					Email:       memberUser.Email,
					Status:      memberUser.Status,
					AvatarURL:   memberUser.AvatarURL,
				},
				InviteCode: member.InviteCode,
			}
		}
		return c.JSON(http.StatusOK, members)
	})

	g.POST("/members", func(c echo.Context) error {
		db := c.Get("__db").(*gorm.DB)

		companyId := c.Get("currentCompany").(uint)
		memberData := new(models.MemberAddIn)
		if err := c.Bind(memberData); err != nil {
			fmt.Println("Parse error", err)
			return err
		}

		_, err := mail.ParseAddress(memberData.Email)

		if err != nil {
			return c.JSON(400, echo.Map{
				"message": "Invalid email",
			})
		}

		var newUser *models.UserAccount
		r := db.Model(&models.UserAccount{}).Where("email = ?", memberData.Email).Find(&newUser)
		if r.Error != nil {
			return c.JSON(http.StatusInternalServerError, map[string]interface{}{"message": "Internal server error"})
		}

		if r.RowsAffected == 0 {

			newUser = &models.UserAccount{
				Name:      "-",
				Email:     memberData.Email,
				GoogleID:  "",
				Platform:  models.PlatformAndroid,
				LastIp:    "",
				Status:    "INVITATION_PENDING",
				AvatarURL: "",
			}
			db.Create(&newUser)
		} else {

			var userMembership models.UserCompanyRole
			r := db.Model(&models.UserCompanyRole{}).Where("user_account_id = ? and company_id = ?", newUser.ID, companyId).Limit(1).Find(&userMembership)

			if r.Error != nil {
				fmt.Println("Error when checking for existing membership", r.Error)
				return c.JSON(500, echo.Map{
					"message": "Something went wrong",
				})
			}

			if r.RowsAffected > 0 {
				return c.JSON(400, echo.Map{
					"message": "User already invited",
				})
			}
		}

		var user_membership = &models.UserCompanyRole{
			CompanyID:     companyId,
			UserAccountID: newUser.ID,
			Active:        false,
			Role:          memberData.Role,
			InviteCode:    StrPointer(RandomInviteCode(7)),
		}
		db.Save(&user_membership)
		return c.JSON(http.StatusOK, echo.Map{
			"message":     "User invited!",
			"invite_code": user_membership.InviteCode,
			"email":       memberData.Email,
		})
	})

	g.POST("/start-trial", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		companyId := c.Get("currentCompany").(uint)
		db := c.Get("__db").(*gorm.DB)
		var membershipDb models.UserCompanyRole

		r := db.Where("company_id = ? and user_account_id = ?", companyId, user.ID).Joins("Company").First(&membershipDb)

		if r.Error != nil {
			fmt.Println("Error while fetching member info")
			return echo.ErrInternalServerError
		}
		if r.RowsAffected == 0 {
			return echo.ErrNotFound
		}
		company := membershipDb.Company
		fmt.Println("Updating company sub, current: ", string(company.Subscription))
		if membershipDb.Role != models.OWNER {
			return c.JSON(http.StatusForbidden, echo.Map{
				"message": "You are not allowed to start trial",
			})
		}

		if string(company.Subscription) == "free" && company.TrialStartedDate == nil {
			company.Subscription = models.Trial
			company.TrialStartedDate = Int64Pointer(time.Now().UnixMilli())
			company.TrialDays = UIntPointer(14)
			db.Save(&company)
			return c.JSON(http.StatusOK, models.CompanyInfoOut{
				Name:             company.Name,
				OwnerId:          company.OwnerID,
				Id:               company.ID,
				Active:           company.Active,
				Subscription:     company.Subscription,
				TrialStartedDate: company.TrialStartedDate,
				TrialDays:        company.TrialDays,
			})
		} else {

			return echo.NewHTTPError(http.StatusConflict, "Contact: sales@skripe.com")
		}

	})

}
