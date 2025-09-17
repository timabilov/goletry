package controllers

import (
	"encoding/json"
	"fmt"
	"lessnoteapi/dbhelper"
	"lessnoteapi/models"
	"lessnoteapi/test"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestAuthGoogle(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, test.AWSProviderMock{}, nil, nil, nil)

	// dUUID := uuid.NewString()

	param := models.GoogleAuthSignIn{
		IdToken:  "eyJhbGciOiJSUzI1NiIsImtpZCI6IjJkOWE1ZWY1YjEyNjIzYzkxNjcxYTcwOTNjYjMyMzMzM2NkMDdkMDkiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJhenAiOiIxMjM0MDkyNzU0NzItcm1qNGxpZjM4cThvZjk4dmZiaDg3cWhmZzIxMGhwYjUuYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJhdWQiOiIxMjM0MDkyNzU0NzItcm1qNGxpZjM4cThvZjk4dmZiaDg3cWhmZzIxMGhwYjUuYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJzdWIiOiIxMDEyMzc1MTYwMTEwMjg2ODQ2NDUiLCJlbWFpbCI6InRpbWFiaWxvdjMzQGdtYWlsLmNvbSIsImVtYWlsX3ZlcmlmaWVkIjp0cnVlLCJhdF9oYXNoIjoiRk5zV0x4WHp2bFNOb3U5Rk40ZHZ1ZyIsIm5vbmNlIjoiRU9oUG5YOU9ZRVlvaDh5TVJkZ1pVWjgtSWdEekJrQkk0VWdQdlpHcl9PMCIsIm5hbWUiOiJUYW1lcmxhbiBBYmlsb3YiLCJwaWN0dXJlIjoiaHR0cHM6Ly9saDMuZ29vZ2xldXNlcmNvbnRlbnQuY29tL2EvQUdObXl4YU9HOFFqVE9hQ3h0b3daSVlUcDZVZUNJV0lXVk1VaEFNeHprTWg9czk2LWMiLCJnaXZlbl9uYW1lIjoiVGFtZXJsYW4iLCJmYW1pbHlfbmFtZSI6IkFiaWxvdiIsImxvY2FsZSI6ImVuIiwiaWF0IjoxNjg0NzgzNDYyLCJleHAiOjE2ODQ3ODcwNjJ9.ZAU26BJJcrWrbJBtqpSTlJjkPa1MkjEJRFyJq2laBcIMg9BFO-9whiC1aR8nW4JL49ASTakZCZzr_y5LNCVXDkKYDTkRrnzbugHvLgsZ6smuQDuoy5MkQ_9LvvP0TdpqYDP3s3heMKp_8PvrfGoF7RUy9tnFZfRVYfGmlS4UsBnioo8qK-DYSDaIuBAB7PPCcEBWjMxfl0_4d_h-ZqVc8hqUUE13VKm5DBFneqhNkClgxfTHG7Xkvi66BfjV7emGU9pk0vU-vRY9cuhoQ8KaQ6xKtXL2-A8qKSQ9hKrJ1Hh7-ct-O6JmFPn3e3lnWBtRMA3Dl9Kxpj9ujlUXusKgOA",
		Platform: "ios",
	}
	req := test.NewJSONRequest("POST", "/auth/google/v2?verify=true", param)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp models.GoogleSignInOut
	json.Unmarshal(rec.Body.Bytes(), &resp)

	assert.Equal(t, "fake@example.com", resp.Email, resp)
	assert.Equal(t, true, resp.New, resp)
	assert.Equal(t, "fake@example.com", resp.Email, resp)
	assert.Equal(t, "pictureurl", resp.Avatar, resp)
	assert.NotEmpty(t, resp.AccessToken, resp)

	// assert.JSONEq(t, `{"message": "Söz 'cookie' üçün sorğu göndərildi! 🕵️"}`, rec.Body.String())

	var user models.UserAccount

	db.First(&user, "email = ?", "fake@example.com")

	assert.Equal(t, "fake@example.com", user.Email)
	assert.Equal(t, "STARTED_AUTH", user.Status)
	assert.Equal(t, models.PlatformIOS, user.Platform)
	// assert.Equal(t, models.AZ, word.Language)
	// assert.Equal(t, false, word.Validated)

	param_2 := models.SignUpIn{
		IdToken:  "eyJhbGciOiJSUzI1NiIsImtpZCI6IjJkOWE1ZWY1YjEyNjIzYzkxNjcxYTcwOTNjYjMyMzMzM2NkMDdkMDkiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJhenAiOiIxMjM0MDkyNzU0NzItcm1qNGxpZjM4cThvZjk4dmZiaDg3cWhmZzIxMGhwYjUuYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJhdWQiOiIxMjM0MDkyNzU0NzItcm1qNGxpZjM4cThvZjk4dmZiaDg3cWhmZzIxMGhwYjUuYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJzdWIiOiIxMDEyMzc1MTYwMTEwMjg2ODQ2NDUiLCJlbWFpbCI6InRpbWFiaWxvdjMzQGdtYWlsLmNvbSIsImVtYWlsX3ZlcmlmaWVkIjp0cnVlLCJhdF9oYXNoIjoiRk5zV0x4WHp2bFNOb3U5Rk40ZHZ1ZyIsIm5vbmNlIjoiRU9oUG5YOU9ZRVlvaDh5TVJkZ1pVWjgtSWdEekJrQkk0VWdQdlpHcl9PMCIsIm5hbWUiOiJUYW1lcmxhbiBBYmlsb3YiLCJwaWN0dXJlIjoiaHR0cHM6Ly9saDMuZ29vZ2xldXNlcmNvbnRlbnQuY29tL2EvQUdObXl4YU9HOFFqVE9hQ3h0b3daSVlUcDZVZUNJV0lXVk1VaEFNeHprTWg9czk2LWMiLCJnaXZlbl9uYW1lIjoiVGFtZXJsYW4iLCJmYW1pbHlfbmFtZSI6IkFiaWxvdiIsImxvY2FsZSI6ImVuIiwiaWF0IjoxNjg0NzgzNDYyLCJleHAiOjE2ODQ3ODcwNjJ9.ZAU26BJJcrWrbJBtqpSTlJjkPa1MkjEJRFyJq2laBcIMg9BFO-9whiC1aR8nW4JL49ASTakZCZzr_y5LNCVXDkKYDTkRrnzbugHvLgsZ6smuQDuoy5MkQ_9LvvP0TdpqYDP3s3heMKp_8PvrfGoF7RUy9tnFZfRVYfGmlS4UsBnioo8qK-DYSDaIuBAB7PPCcEBWjMxfl0_4d_h-ZqVc8hqUUE13VKm5DBFneqhNkClgxfTHG7Xkvi66BfjV7emGU9pk0vU-vRY9cuhoQ8KaQ6xKtXL2-A8qKSQ9hKrJ1Hh7-ct-O6JmFPn3e3lnWBtRMA3Dl9Kxpj9ujlUXusKgOA",
		Platform: "ios",
		ProfileIn: models.ProfileIn{
			Name:    "My Name",
			Company: "LLC",
		},
	}
	req_2 := test.NewJSONRequest("POST", "/auth/google/v2", param_2)
	rec_2 := httptest.NewRecorder()

	e.ServeHTTP(rec_2, req_2)

	var resp2 echo.Map
	json.Unmarshal(rec_2.Body.Bytes(), &resp2)

	db.First(&user, "email = ?", "fake@example.com")

	assert.Equal(t, "fake@example.com", user.Email)
	assert.Equal(t, "FINISHED_AUTH", user.Status)
	assert.Equal(t, models.PlatformIOS, user.Platform)
	assert.Equal(t, "My Name", user.Name)

	var company models.Company

	db.First(&company)
	assert.Equal(t, "LLC", company.Name)

	var membership models.UserCompanyRole

	db.First(&membership)
	assert.Equal(t, user.ID, membership.UserAccountID)
	assert.Equal(t, true, membership.Active)
	assert.Equal(t, company.ID, membership.CompanyID)

	param_3 := models.SignUpIn{
		IdToken:  "eyJhbGciOiJSUzI1NiIsImtpZCI6IjJkOWE1ZWY1YjEyNjIzYzkxNjcxYTcwOTNjYjMyMzMzM2NkMDdkMDkiLCJ0eXAiOiJKV1QifQ.eyJpc3MiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20iLCJhenAiOiIxMjM0MDkyNzU0NzItcm1qNGxpZjM4cThvZjk4dmZiaDg3cWhmZzIxMGhwYjUuYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJhdWQiOiIxMjM0MDkyNzU0NzItcm1qNGxpZjM4cThvZjk4dmZiaDg3cWhmZzIxMGhwYjUuYXBwcy5nb29nbGV1c2VyY29udGVudC5jb20iLCJzdWIiOiIxMDEyMzc1MTYwMTEwMjg2ODQ2NDUiLCJlbWFpbCI6InRpbWFiaWxvdjMzQGdtYWlsLmNvbSIsImVtYWlsX3ZlcmlmaWVkIjp0cnVlLCJhdF9oYXNoIjoiRk5zV0x4WHp2bFNOb3U5Rk40ZHZ1ZyIsIm5vbmNlIjoiRU9oUG5YOU9ZRVlvaDh5TVJkZ1pVWjgtSWdEekJrQkk0VWdQdlpHcl9PMCIsIm5hbWUiOiJUYW1lcmxhbiBBYmlsb3YiLCJwaWN0dXJlIjoiaHR0cHM6Ly9saDMuZ29vZ2xldXNlcmNvbnRlbnQuY29tL2EvQUdObXl4YU9HOFFqVE9hQ3h0b3daSVlUcDZVZUNJV0lXVk1VaEFNeHprTWg9czk2LWMiLCJnaXZlbl9uYW1lIjoiVGFtZXJsYW4iLCJmYW1pbHlfbmFtZSI6IkFiaWxvdiIsImxvY2FsZSI6ImVuIiwiaWF0IjoxNjg0NzgzNDYyLCJleHAiOjE2ODQ3ODcwNjJ9.ZAU26BJJcrWrbJBtqpSTlJjkPa1MkjEJRFyJq2laBcIMg9BFO-9whiC1aR8nW4JL49ASTakZCZzr_y5LNCVXDkKYDTkRrnzbugHvLgsZ6smuQDuoy5MkQ_9LvvP0TdpqYDP3s3heMKp_8PvrfGoF7RUy9tnFZfRVYfGmlS4UsBnioo8qK-DYSDaIuBAB7PPCcEBWjMxfl0_4d_h-ZqVc8hqUUE13VKm5DBFneqhNkClgxfTHG7Xkvi66BfjV7emGU9pk0vU-vRY9cuhoQ8KaQ6xKtXL2-A8qKSQ9hKrJ1Hh7-ct-O6JmFPn3e3lnWBtRMA3Dl9Kxpj9ujlUXusKgOA",
		Platform: "ios",
		ProfileIn: models.ProfileIn{
			Name:    "My Name",
			Company: "LLC",
		},
	}
	req_3 := test.NewJSONRequest("POST", "/auth/google/v2?verify=true", param_3)
	rec_3 := httptest.NewRecorder()

	e.ServeHTTP(rec_3, req_3)

	var resp3 echo.Map
	json.Unmarshal(rec_3.Body.Bytes(), &resp3)

	assert.Equal(t, fmt.Sprint(resp3["id"]), fmt.Sprint(user.ID), rec_3.Body.String())
	assert.NotEmpty(t, fmt.Sprint(resp3["company_id"]), fmt.Sprint(company.ID), rec_3.Body.String())
}

func TestRefreshToken(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, test.AWSProviderMock{}, nil, nil, nil)

	// dUUID := uuid.NewString()
	userDb := test.FakeUserV2(db, nil, "name", "refresh@fastposapp.com")
	refreshToken, err := GenerateRefreshToken(fmt.Sprint(userDb.ID))
	if err != nil {
		fmt.Println("Error generating refesh", err)
	}
	param := echo.Map{
		"refresh_token": refreshToken,
	}
	req := test.NewJSONRequest("POST", "/auth/refresh-token", param)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

}

func TestAcceptInvitation(t *testing.T) {
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
	nowMs := time.Now().UnixMilli()
	data := map[string]string{
		"email": "newuser@fastposapp.com",
		"role":  string(models.ADMIN),
	}
	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/auth/accept-invitation/%v", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(invitedUser.ID), 10), data)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	db.Where("company_id = ? and user_account_id = ?", user.Memberships[0].CompanyID, invitedUser.ID).First(&newMembership)
	// db.Save(&newMembership)
	assert.Greater(t, *newMembership.InviteAcceptedAt, nowMs)
	assert.Equal(t, true, newMembership.Active)

}
