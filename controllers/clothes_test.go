package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"letryapi/dbhelper"
	"letryapi/models"
	"letryapi/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateClothingOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil, &test.URLCacheMock{})
	user := test.FakeUser(db, nil)

	// Prepare request payload
	reqBody := CreateClothingIn{
		Name:         "Test Clothing",
		Description:  stringPtr("This is a test clothing item"),
		ClothingType: "top",
		FileName:     stringPtr("test-image.jpg"),
		AddToCloset:  BoolPointer(false),
	}

	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/clothes/tryon", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "Expected status code 201 Created, got %d", rec.Code)

	var response ClothingCreatedResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Equal(t, reqBody.Name, response.ClothingResponse.Name)
	require.Equal(t, reqBody.Description, response.ClothingResponse.Description)
	require.Equal(t, reqBody.ClothingType, response.ClothingResponse.ClothingType)
}

func TestCreateClothingInvalidInput(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil, &test.URLCacheMock{})
	user := test.FakeUser(db, nil)

	// Prepare invalid request payload (missing required fields)
	reqBody := CreateClothingIn{
		Name: "Test Clothing",
		// ClothingType missing
		FileName:    stringPtr("test.jpg"),
		AddToCloset: BoolPointer(false),
	}

	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/clothes/tryon", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"], "clothing_type")
}

func TestCreateClothingUnauthorized(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil, &test.URLCacheMock{})
	user := test.FakeUser(db, nil)

	// Prepare request payload
	reqBody := CreateClothingIn{
		Name:         "Test Clothing",
		Description:  stringPtr("This is a test clothing item"),
		ClothingType: "top",
		FileName:     stringPtr("test.jpg"),
		AddToCloset:  BoolPointer(false),
	}
	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/clothes/tryon", user.Memberships[0].CompanyID), "", reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListClothesOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil, &test.URLCacheMock{})
	user := test.FakeUser(db, nil)

	// Create test clothing items
	clothing1 := models.Clothing{
		Name:         "Test Top",
		Description:  stringPtr("A test top"),
		ClothingType: "top",
		OwnerID:      user.ID,
		CompanyID:    user.Memberships[0].CompanyID,
		Status:       "in_closet",
	}
	clothing2 := models.Clothing{
		Name:         "Test Bottom",
		Description:  stringPtr("A test bottom"),
		ClothingType: "bottom",
		OwnerID:      user.ID,
		CompanyID:    user.Memberships[0].CompanyID,
		Status:       "in_closet",
	}

	require.NoError(t, db.Create(&clothing1).Error)
	require.NoError(t, db.Create(&clothing2).Error)

	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/clothes/list", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "Expected status code 200 OK, got %d: %s", rec.Code, rec.Body.String())

	var response ClothesListResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response.Tops, 1)
	require.Len(t, response.Bottoms, 1)
	require.Equal(t, clothing1.Name, response.Tops[0].Name)
	require.Equal(t, clothing2.Name, response.Bottoms[0].Name)
}

func TestListClothesEmpty(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil, &test.URLCacheMock{})
	user := test.FakeUser(db, nil)

	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/clothes/list", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var response ClothesListResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response.Tops, 0)
	require.Len(t, response.Bottoms, 0)
	require.Len(t, response.Shoes, 0)
	require.Len(t, response.Accessories, 0)
}

func TestListClothesUnauthorized(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil, &test.URLCacheMock{})

	req := test.NewJSONAuthRequest("GET", "/company/1/clothes/list", "", "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Unauthorized", response["error"])
}

func TestListClothesWithMultipleTypes(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil, &test.URLCacheMock{})
	user := test.FakeUser(db, nil)

	// Create test clothing items of different types
	top := models.Clothing{
		Name:         "Test Top",
		Description:  stringPtr("A test top"),
		ClothingType: "top",
		OwnerID:      user.ID,
		CompanyID:    user.Memberships[0].CompanyID,
		Status:       "in_closet",
	}
	shoes := models.Clothing{
		Name:         "Test Shoes",
		Description:  stringPtr("A test shoes"),
		ClothingType: "shoes",
		OwnerID:      user.ID,
		CompanyID:    user.Memberships[0].CompanyID,
		Status:       "in_closet",
	}
	accessory := models.Clothing{
		Name:         "Test Accessory",
		Description:  stringPtr("A test accessory"),
		ClothingType: "accessory",
		OwnerID:      user.ID,
		CompanyID:    user.Memberships[0].CompanyID,
		Status:       "in_closet",
	}

	require.NoError(t, db.Create(&top).Error)
	require.NoError(t, db.Create(&shoes).Error)
	require.NoError(t, db.Create(&accessory).Error)

	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/clothes/list", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "Expected status code 200 OK, got %d: %s", rec.Code, rec.Body.String())

	var response ClothesListResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response.Tops, 1)
	require.Len(t, response.Bottoms, 0)
	require.Len(t, response.Shoes, 1)
	require.Len(t, response.Accessories, 1)
	assert.Equal(t, top.Name, response.Tops[0].Name)
	assert.Equal(t, shoes.Name, response.Shoes[0].Name)
	assert.Equal(t, accessory.Name, response.Accessories[0].Name)
}

func stringPtr(s string) *string {
	return &s
}
