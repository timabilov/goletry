package controllers

// import (
// 	"encoding/json"
// 	"fmt"
// 	"lessnoteapi/dbhelper"
// 	"lessnoteapi/models"
// 	"lessnoteapi/test"
// 	"log"
// 	"net/http"
// 	"net/http/httptest"
// 	"strconv"
// 	"testing"

// 	"github.com/stretchr/testify/assert"
// )

// func TestGetCompanyMembersOk(t *testing.T) {
// 	db := SetupTestDB()
// 	cleaner := dbhelper.SetupCleaner(db)
// 	defer cleaner()
// 	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil)
// 	// user3 := test.FakeUser(db, nil)
// 	user := test.FakeUser(db, nil)
// 	currentCompanyMember := test.FakeUserV2(db, &user.Memberships[0].Company, "Current", "current@fastposapp.com")

// 	company := &models.Company{
// 		Name:    "Donot need it now",
// 		OwnerID: user.ID,
// 	}
// 	db.Create(&company)
// 	db.Save(&models.UserCompanyRole{
// 		CompanyID:     company.ID,
// 		UserAccountID: user.ID,
// 		Active:        true,
// 		Role:          "OWNER",
// 	})
// 	test.FakeUserV2(db, company, "Another one in different company", "another@fastposapp.com")

// 	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/members", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), "")
// 	rec := httptest.NewRecorder()

// 	e.ServeHTTP(rec, req)

// 	assert.Equal(t, http.StatusOK, rec.Code)
// 	var payload []models.MemberInfoOut
// 	err := json.Unmarshal(rec.Body.Bytes(), &payload)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	for _, mm := range payload {
// 		fmt.Println(mm.UserInfo.Name)
// 	}
// 	assert.Equal(t, 2, len(payload))
// 	userData := payload[0].UserInfo
// 	assert.Equal(t, user.Name, userData.Name)
// 	assert.Equal(t, user.Email, userData.Email)

// 	otherUserData := payload[1].UserInfo
// 	assert.Equal(t, currentCompanyMember.Name, otherUserData.Name)
// 	assert.Equal(t, currentCompanyMember.Email, otherUserData.Email)
// 	// assert.Equal(t, "My Company", userData["company_name"])

// }

// func TestAddCompanyNewMemberOk(t *testing.T) {
// 	db := SetupTestDB()
// 	cleaner := dbhelper.SetupCleaner(db)
// 	defer cleaner()
// 	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil)
// 	// user3 := test.FakeUser(db, nil)
// 	user := test.FakeUser(db, nil)

// 	data := map[string]string{
// 		"email": "myemail@fastposapp.com",
// 		"role":  string(models.ADMIN),
// 	}
// 	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/members", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), data)
// 	rec := httptest.NewRecorder()

// 	e.ServeHTTP(rec, req)

// 	assert.Equal(t, http.StatusOK, rec.Code)
// 	var payload map[string]string
// 	err := json.Unmarshal(rec.Body.Bytes(), &payload)

// 	if err != nil {
// 		fmt.Println(err)
// 	}

// 	inviteCode := payload["invite_code"]
// 	assert.NotNil(t, inviteCode)

// 	var createdMembership *models.UserCompanyRole

// 	db.Where("invite_code = ? ", inviteCode).Joins("UserAccount").First(&createdMembership)

// 	assert.NotNil(t, createdMembership)

// 	assert.Equal(t, "myemail@fastposapp.com", createdMembership.UserAccount.Email)
// 	assert.Equal(t, "INVITATION_PENDING", createdMembership.UserAccount.Status)

// }

// func TestAddCompanyExistingUserMemberOk(t *testing.T) {
// 	db := SetupTestDB()
// 	cleaner := dbhelper.SetupCleaner(db)
// 	defer cleaner()
// 	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil)
// 	// user3 := test.FakeUser(db, nil)
// 	user := test.FakeUser(db, nil)
// 	otherCompanyUser := test.FakeUserV2(db, nil, "Someuser", "newuser@fastposapp.com")

// 	data := map[string]string{
// 		"email": "newuser@fastposapp.com",
// 		"role":  string(models.ADMIN),
// 	}
// 	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/members", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), data)
// 	rec := httptest.NewRecorder()

// 	e.ServeHTTP(rec, req)

// 	assert.Equal(t, http.StatusOK, rec.Code)
// 	var payload map[string]string
// 	err := json.Unmarshal(rec.Body.Bytes(), &payload)

// 	if err != nil {
// 		fmt.Println(err)
// 	}

// 	inviteCode := payload["invite_code"]
// 	assert.NotNil(t, inviteCode)

// 	var createdMembership *models.UserCompanyRole

// 	db.Where("invite_code = ? ", inviteCode).Joins("UserAccount").First(&createdMembership)

// 	assert.NotNil(t, createdMembership)

// 	assert.Equal(t, otherCompanyUser.ID, createdMembership.UserAccount.ID)
// 	// assert.Equal(t, "INVITATION_PENDING", createdMembership.UserAccount.Status)

// }

// func TestAddCompanyMemberAlreadyInvited(t *testing.T) {
// 	db := SetupTestDB()
// 	cleaner := dbhelper.SetupCleaner(db)
// 	defer cleaner()
// 	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil)
// 	// user3 := test.FakeUser(db, nil)
// 	user := test.FakeUser(db, nil)
// 	test.FakeUserV2(db, &user.Memberships[0].Company, "Someuser", "newuser@fastposapp.com")

// 	data := map[string]string{
// 		"email": "newuser@fastposapp.com",
// 		"role":  string(models.ADMIN),
// 	}
// 	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/members", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), data)
// 	rec := httptest.NewRecorder()

// 	e.ServeHTTP(rec, req)

// 	assert.Equal(t, http.StatusBadRequest, rec.Code)
// 	// assert.Equal(t, "INVITATION_PENDING", createdMembership.UserAccount.Status)

// }

// // func TestUserDataRequestBatchImageUpload(t *testing.T) {
// // 	db := SetupTestDB()
// // 	cleaner := dbhelper.SetupCleaner(db)
// // 	defer cleaner()
// // 	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil)
// // 	// pUUID := uuid.NewString()

// // 	user := test.FakeUser(db, nil)
// // 	companyId := user.Memberships[0].CompanyID
// // 	defaultCreated := time.Now().UnixMilli()
// // 	fmt.Println("Test created ms: ", defaultCreated)
// // 	db.Save(&[]models.Product{
// // 		{
// // 			WatermelonJsonModel: models.WatermelonJsonModel{

// // 				ID:                 "28a7daaa-e195-4a29-b054-46b7594d688c",
// // 				CreatedAt:          defaultCreated,
// // 				UpdatedAt:          defaultCreated,
// // 				ServerCreatedAt:    defaultCreated,
// // 				ServerLastModified: defaultCreated,
// // 				DeletedAt:          nil,
// // 			},
// // 			Name:            "product",
// // 			UniqueId:        "uniqueId33",
// // 			UnitType:        StrPointer("pcs"),
// // 			RelatedId:       nil,
// // 			ShopId:          nil,
// // 			ImageUri:        "",
// // 			ImageUrl:        nil,
// // 			Barcode:         "123456789",
// // 			Sku:             nil,
// // 			UnlimitedStock:  false,
// // 			Bookmarked:      false,
// // 			DiscountPercent: nil,
// // 			SalesPrice:      123.99,
// // 			CostPrice:       103.99,
// // 			CreatedUserId:   StrPointer(fmt.Sprintf("%v", user.ID)),
// // 			CreatedById:     UIntPointer(user.ID),
// // 			Weight:          Float32Pointer(2.34),
// // 			Notes:           StrPointer("My notes"),
// // 			CompanyID:       companyId,
// // 			StockCount:      12,
// // 			Color:           StrPointer("#aaaaaa"),
// // 		},
// // 		{
// // 			WatermelonJsonModel: models.WatermelonJsonModel{

// // 				ID:                 "18a7daaa-e195-4a29-b054-46b7594d688c",
// // 				CreatedAt:          defaultCreated - 5,
// // 				UpdatedAt:          defaultCreated - 5,
// // 				ServerCreatedAt:    defaultCreated - 5,
// // 				ServerLastModified: defaultCreated - 5,
// // 				DeletedAt:          nil,
// // 			},
// // 			Name:            "In DB",
// // 			UniqueId:        "uniqueId",
// // 			UnitType:        StrPointer("pcs"),
// // 			RelatedId:       nil,
// // 			ShopId:          nil,
// // 			ImageUri:        "",
// // 			ImageUrl:        nil,
// // 			Barcode:         "123456789",
// // 			Sku:             nil,
// // 			UnlimitedStock:  false,
// // 			Bookmarked:      false,
// // 			DiscountPercent: nil,
// // 			SalesPrice:      120.99,
// // 			CostPrice:       100.99,
// // 			CreatedUserId:   StrPointer(fmt.Sprintf("%v", user.ID)),
// // 			CreatedById:     UIntPointer(user.ID),
// // 			Weight:          Float32Pointer(2.34),
// // 			Notes:           StrPointer("My notes"),
// // 			CompanyID:       companyId,
// // 			StockCount:      12,
// // 			Color:           StrPointer("#aaaaaa"),
// // 		},
// // 	})

// // 	data := map[string]interface{}{
// // 		"products": []map[string]string{
// // 			{
// // 				"product_id": "28a7daaa-e195-4a29-b054-46b7594d688c",
// // 				"image_name": "name1.jpg",
// // 			},
// // 			{
// // 				"product_id": "18a7daaa-e195-4a29-b054-46b7594d688c",
// // 				"image_name": "name2.jpg",
// // 			},
// // 		},
// // 	}
// // 	jsonData, err := json.Marshal(data)
// // 	fmt.Println(string(jsonData))
// // 	if err != nil {
// // 		fmt.Println("data to json error ", err)
// // 	}
// // 	req := test.NewJSONAuthRequestRaw("POST", "/shop/userdata/request-batch-image-upload-v2", strconv.FormatUint(uint64(user.ID), 10), string(jsonData))
// // 	rec := httptest.NewRecorder()

// // 	e.ServeHTTP(rec, req)

// // 	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
// // 	// var tag models.Tag

// // 	// db.First(&product, "unique_id = ?", pUUID)

// // 	// ---- PRODUCTS
// // 	payload := models.ProductImagesUploadRequestOut{}
// // 	// fmt.Println(rec.Body.String())
// // 	err = json.Unmarshal([]byte(rec.Body.Bytes()), &payload)
// // 	if err != nil {
// // 		log.Fatal(err)
// // 	}

// // 	assert.Equal(t, len(payload.Products), 2, "Only two products should be returned")

// // 	firstProduct := payload.Products[0]
// // 	assert.Equal(t, firstProduct.ProductId, "18a7daaa-e195-4a29-b054-46b7594d688c")
// // 	assert.NotNil(t, firstProduct.UploadUrl, "Product upload url cannot be null")

// // 	secondProduct := payload.Products[1]
// // 	assert.Equal(t, secondProduct.ProductId, "28a7daaa-e195-4a29-b054-46b7594d688c")
// // 	assert.NotNil(t, secondProduct.UploadUrl, "Product upload url cannot be null")

// // 	var product models.Product
// // 	r := db.First(&product, "id =? ", "28a7daaa-e195-4a29-b054-46b7594d688c")
// // 	if r.Error != nil {
// // 		fmt.Println("Error fetching product", err)
// // 	}

// // 	assert.NotNil(t, product.ImageUrl, "Image url cannot be null")

// // }
