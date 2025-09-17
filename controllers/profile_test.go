package controllers

import (
	"encoding/json"
	"lessnoteapi/dbhelper"
	"lessnoteapi/test"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProfileOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	req := test.NewJSONAuthRequest("GET", "/shop/profile/me", strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	payload := map[string]interface{}{}

	err := json.Unmarshal([]byte(rec.Body.String()), &payload)
	if err != nil {
		log.Fatal(err)
	}
	assert.Equal(t, user.Name, payload["name"])
	assert.Equal(t, user.Email, payload["email"])
	assert.Equal(t, "My Company", payload["company_name"])

}

func TestGetMembersOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	// user3 := test.FakeUser(db, nil)
	user := test.FakeUser(db, nil)

	req := test.NewJSONAuthRequest("GET", "/shop/profile/members", strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// payload := []map[string]interface{}{}

	// err := json.Unmarshal(rec.Body.Bytes(), &payload)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// userData := payload[0]["user"].(map[string]string)
	// assert.Equal(t, user.Name, userData["name"])
	// assert.Equal(t, user.Email, userData["email"])
	// assert.Equal(t, "My Company", userData["company_name"])

}
