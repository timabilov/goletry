package controllers

import (
	"fmt"
	"letryapi/dbhelper"
	"letryapi/models"
	"letryapi/test"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebhookBody(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	// user3 := test.FakeUser(db, nil)
	user := test.FakeUser(db, nil)

	invitedUser := models.UserAccount{
		Name:   "Invited",
		Status: "INVITATION_PENDING",
		Email:  "invitedme@fastposapp.com",
	}
	db.Save(&invitedUser)
	newMembership := models.UserCompanyRole{
		UserAccountID: invitedUser.ID,
		CompanyID:     user.Memberships[0].CompanyID,
		Active:        false,
		Role:          models.SALES,
	}
	db.Save(&newMembership)
	data := map[string]interface{}{
		"event": map[string]interface{}{
			"app_id":                      "app70fd013e95",
			"app_user_id":                 fmt.Sprint(user.ID),
			"commission_percentage":       nil,
			"country_code":                "US",
			"currency":                    nil,
			"entitlement_id":              nil,
			"entitlement_ids":             nil,
			"environment":                 "SANDBOX",
			"event_timestamp_ms":          1715405366686,
			"expiration_at_ms":            1715412566686,
			"id":                          "791C890E-B8AD-46C9-8290-13EAF5F14C9F",
			"is_family_share":             nil,
			"offer_code":                  nil,
			"original_app_user_id":        "7f680253-003b-4073-b4f3-5d1df7cd9a67",
			"original_transaction_id":     nil,
			"period_type":                 "NORMAL",
			"presented_offering_id":       nil,
			"price":                       nil,
			"price_in_purchased_currency": nil,
			"product_id":                  "test_product",
			"purchased_at_ms":             1715405366686,
			"store":                       "PLAY_STORE",
			"takehome_percentage":         nil,
			"tax_percentage":              nil,
			"transaction_id":              nil,
			"type":                        "INITIAL_PURCHASE",
			// "type":                        "INITIAL_PURCHASE",
			// "type":                        "EXPIRATION",
			// "type":          "CANCELLATION",
			// "cancel_reason": "PRICE_INCREASE",
		},
	}
	req := test.NewJSONAuthRequestCustomAuth("POST", "/webhooks/rc-subscription-webhooks", fmt.Sprintf("Bearer %s", "fake"), data)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	var updatedCompany models.Company
	db.First(&updatedCompany, user.Memberships[0].CompanyID)

	assert.Equal(t, models.ProPlus, updatedCompany.Subscription)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

}
