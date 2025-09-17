package controllers

import (
	"fmt"
	"lessnoteapi/models"
	"net/http"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type ProfileController struct {
}

func (controller *ProfileController) ProfileRoutes(g *echo.Group) {
	g.GET("/me", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var companyDb models.Company
		r := db.Limit(1).Find(&companyDb, "id = ?", user.Memberships[0].CompanyID)
		if r.RowsAffected == 0 {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Something happened",
			})
		}

		return c.JSON(http.StatusOK, models.UserInfoOut{
			Name:        user.Name,
			CompanyName: companyDb.Name,
			Email:       user.Email,
			Status:      user.Status,
			AvatarUrl:   user.AvatarUrl,
		})
	})

	g.GET("/members", func(c echo.Context) error {
		user := c.Get("currentUser").(models.UserAccount)
		db := c.Get("__db").(*gorm.DB)
		var companyDb models.Company
		r := db.Preload("Members.UserAccount").Limit(1).Find(&companyDb, "id = ?", user.Memberships[0].CompanyID)
		if r.RowsAffected == 0 {
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Something happened",
			})
		}
		// var models
		var members []models.MemberInfoOut
		for _, member := range companyDb.Members {
			memberUser := member.UserAccount
			fmt.Println("Member: ", memberUser.Name, memberUser.Status)
			members = append(members, models.MemberInfoOut{
				Active: member.Active,
				Role:   member.Role,
				UserInfo: models.UserInfoOut{
					Id:          memberUser.ID,
					Name:        memberUser.Name,
					CompanyName: companyDb.Name,
					Email:       memberUser.Email,
					Status:      memberUser.Status,
					AvatarUrl:   memberUser.AvatarUrl,
				},
				InviteCode: member.InviteCode,
			})
		}
		return c.JSON(http.StatusOK, members)
	})

}
