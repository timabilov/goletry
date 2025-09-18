package test

import (
	"context"
	"encoding/json"
	"fmt"
	"letryapi/models"
	"letryapi/services"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"google.golang.org/api/idtoken"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func JsonString(model interface{}) string {
	bytes, _ := json.Marshal(model)
	return string(bytes)
}

func NewJSONRequest(method string, target string, param interface{}) *http.Request {

	req := httptest.NewRequest(method, target, strings.NewReader(JsonString(param)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	return req
}

func GenerateUserToken(userPk string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   userPk,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour * 72)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	t, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		log.Fatalf("Error when signing user token for %s. Error %s ", userPk, err)
		// sentry.CaptureMessage("Error when signing user token for %s. Error %s ", userPk, err)
	}
	return t
}

func NewJSONAuthRequest(method string, target string, userPk string, param interface{}) *http.Request {
	log.Println(JsonString(param))
	req := httptest.NewRequest(method, target, strings.NewReader(JsonString(param)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	token := GenerateUserToken(userPk)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return req
}

func NewJSONAuthRequestCustomAuth(method string, target string, authorizationString string, param interface{}) *http.Request {
	log.Println(JsonString(param))
	req := httptest.NewRequest(method, target, strings.NewReader(JsonString(param)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", authorizationString)
	return req
}

func NewJSONAuthRequestRaw(method string, target string, userPk string, json string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(json))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	token := GenerateUserToken(userPk)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	return req
}

func Int64Pointer(i int64) *int64 {
	return &i
}

func FakeUser(db *gorm.DB, company *models.Company) *models.UserAccount {
	user := &models.UserAccount{
		Name:      "OurName",
		Email:     "email@example.com",
		GoogleID:  "12232",
		Platform:  models.PlatformIOS,
		LastIp:    "123.122.122.122",
		Status:    "FINISHED_AUTH",
		AvatarUrl: "pictureurl",
	}
	db.Create(&user)

	// fmt.Printf("Test.. Fake user id %s \n", strconv.FormatUint(uint64(user.ID), 10))
	if company == nil {

		company = &models.Company{
			Name:         "My Company",
			OwnerID:      user.ID,
			Subscription: "free",
			// TrialStartedDate: Int64Pointer(time.Now().UnixMilli()),
		}
		db.Create(&company)
	}
	var user_membership = &models.UserCompanyRole{
		CompanyID:        company.ID,
		UserAccountID:    user.ID,
		Active:           true,
		InviteAcceptedAt: Int64Pointer(time.Now().UnixMilli()),
		Role:             "OWNER",
	}
	db.Save(&user)
	tokenDb := models.UserPushToken{
		UserAccountID: user.ID,
		Platform:      "android",
		Token:         "cX-UZ3zwQEiPt-2GJkG2gA:APA91bGqRflaGrJrnynhRwZ442HdgUjVcO7mWMFnx6IwAdJ9RRKopvSP4QU7hbvTmk1XAp8XGvtHZLvo5JmOPTVKBbGqqvhfbZWKlXA9csEjx1hgpNvrWepU-rqG1sxS8_WCF5cGZchf",
		Active:        true,
	}
	db.Save(&tokenDb)
	db.Save(&user_membership)
	// db.Commit()
	db.Preload("Memberships.Company").First(&user, user.ID)

	return user
}

func FakeUserV2(db *gorm.DB, company *models.Company, userName string, email string) *models.UserAccount {

	if email == "" {
		email = "email@example.com"
	}
	user := &models.UserAccount{
		Name:      userName,
		Email:     email,
		GoogleID:  "12232",
		Platform:  models.PlatformIOS,
		LastIp:    "123.122.122.122",
		Status:    "FINISHED_AUTH",
		AvatarUrl: "pictureurl",
	}
	db.Create(&user)
	// fmt.Printf("Test.. Fake user id %s \n", strconv.FormatUint(uint64(user.ID), 10))
	if company == nil {

		company = &models.Company{
			Name:    "My Company",
			OwnerID: user.ID,
		}
		db.Create(&company)
	}
	var user_membership = &models.UserCompanyRole{
		CompanyID:     company.ID,
		UserAccountID: user.ID,
		Active:        true,
		Role:          "OWNER",
	}
	db.Save(&user)
	tokenDb := models.UserPushToken{
		UserAccountID: user.ID,
		Platform:      "android",
		Token:         "cX-UZ3zwQEiPt-2GJkG2gA:APA91bGqRflaGrJrnynhRwZ442HdgUjVcO7mWMFnx6IwAdJ9RRKopvSP4QU7hbvTmk1XAp8XGvtHZLvo5JmOPTVKBbGqqvhfbZWKlXA9csEjx1hgpNvrWepU-rqG1sxS8_WCF5cGZchf",
		Active:        true,
	}
	db.Save(&tokenDb)
	db.Save(&user_membership)
	db.Preload(clause.Associations).First(&user, user.ID)
	return user
}

func NewJSONRootRequest(method string, target string, param interface{}, password string) *http.Request {

	req := httptest.NewRequest(method, target, strings.NewReader(JsonString(param)))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", password)
	return req
}

func Contains(items []string, lookFor string) bool {

	for i := 0; i < len(items); i++ {

		if items[i] == lookFor {
			return true
		}
	}
	return false
}

func NewRefString(data string) *string {
	return &data
}

func InternalRequestMessage(e *echo.Echo, method string, url string, param interface{}, password string) string {
	var req *http.Request
	if password != "" {

		req = NewJSONRootRequest(method, url, param, os.Getenv("ROOT_PASSWORD"))
	} else {
		req = NewJSONRequest(method, url, param)
	}
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	var r map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code > 300 {

		log.Printf("%s", rec.Body.String())
	}
	if val, ok := r["message"]; ok {
		return val.(string)
	}

	return "internal error"

}

func InternalRequestJSON(e *echo.Echo, method string, url string, param interface{}, password string) []byte {
	var req *http.Request
	if password != "" {

		req = NewJSONRootRequest(method, url, param, os.Getenv("ROOT_PASSWORD"))
	} else {
		req = NewJSONRequest(method, url, param)
	}
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)
	if rec.Code > 300 {

		log.Println(rec.Body.String())
		log.Printf("%s", rec.Body.String())
	}
	return rec.Body.Bytes()

}

type GoogleServiceMock struct{}

func (gsm GoogleServiceMock) ValidateIdToken(ctx context.Context, idToken string, audience string) (*idtoken.Payload, error) {

	return &idtoken.Payload{Issuer: "Issue", Audience: "AAA", Expires: 119919191919, IssuedAt: 12312321321, Subject: "fake@example.com", Claims: map[string]interface{}{
		"email":   "fake@example.com",
		"picture": "pictureurl",
		"sub":     "123googleid",
	}}, nil

}

func (gsm GoogleServiceMock) GetUserSubscriptionStatus(ctx context.Context, appUserId string) ([]byte, error) {
	data := `
	{
		"request_date": "2024-05-11T06:50:56Z",
		"request_date_ms": 1715410256322,
		"subscriber": {
		  "entitlements": {
			"Pro": {
			  "expires_date": "2024-05-11T06:51:15Z",
			  "grace_period_expires_date": null,
			  "product_identifier": "prostandard",
			  "product_plan_identifier": "monthly-autorenew",
			  "purchase_date": "2024-05-11T06:49:05Z"
			},
			"Pro Plus": {
			  "expires_date": "2029-05-12T22:28:12Z",
			  "grace_period_expires_date": null,
			  "product_identifier": "vunpos_pro_plus",
			  "product_plan_identifier": "pro-plus-monthly",
			  "purchase_date": "2024-05-10T22:23:12Z"
			}
		  },
		  "first_seen": "2024-05-07T12:41:57Z",
		  "last_seen": "2024-05-10T20:43:21Z",
		  "management_url": "https://play.google.com/store/account/subscriptions",
		  "non_subscriptions": {},
		  "original_app_user_id": "$RCAnonymousID:60ad7a0c84694890b4b272b5654efa1f",
		  "original_application_version": null,
		  "original_purchase_date": null,
		  "other_purchases": {},
		  "subscriptions": {
			"prostandard": {
			  "auto_resume_date": null,
			  "billing_issues_detected_at": null,
			  "expires_date": "2024-05-11T06:51:15Z",
			  "grace_period_expires_date": null,
			  "is_sandbox": true,
			  "original_purchase_date": "2024-05-11T06:49:05Z",
			  "period_type": "normal",
			  "product_plan_identifier": "monthly-autorenew",
			  "purchase_date": "2024-05-11T06:49:05Z",
			  "refunded_at": null,
			  "store": "play_store",
			  "store_transaction_id": "GPA.3308-7668-0800-70257",
			  "unsubscribe_detected_at": null
			},
			"vunpos_pro_plus": {
			  "auto_resume_date": null,
			  "billing_issues_detected_at": null,
			  "expires_date": "2024-05-12T22:28:12Z",
			  "grace_period_expires_date": null,
			  "is_sandbox": true,
			  "original_purchase_date": "2024-05-10T21:56:21Z",
			  "period_type": "normal",
			  "product_plan_identifier": "pro-plus-monthly",
			  "purchase_date": "2024-05-10T22:23:12Z",
			  "refunded_at": null,
			  "store": "play_store",
			  "store_transaction_id": "GPA.3311-8032-8178-10570..5",
			  "unsubscribe_detected_at": "2024-05-10T22:28:15Z"
			}
		  }
		}
	  }
	  `

	return []byte(data), nil
}

type AWSProviderMock struct {
	MockUrl string
}

func (awsService AWSProviderMock) InitPresignClient(ctx context.Context) error {
	return nil
}

func (awsService AWSProviderMock) PresignLink(ctx context.Context, bucketName string, fileName string) (string, error) {

	return fmt.Sprintf("https://fakebucketurl.com/%s", fileName), nil
}

func (awsService AWSProviderMock) GetPresignedR2FileReadURL(ctx context.Context, bucketName, fileKey string) (string, error) {
	return awsService.MockUrl, nil
}

func (awsService AWSProviderMock) UploadToPresignedURL(ctx context.Context, bucketName, url string, fileContent []byte) (string, int, error) {
	// Simulate a successful upload
	// In a real implementation, you would use the AWS SDK to upload the file to S3
	// and return the URL of the uploaded file.
	return url, 204, nil
}

type MockGoogleTranscriber struct{}

func (m MockGoogleTranscriber) Transcribe(filePaths []string, modelName services.LLMModelName) (*services.LLMResponse, error) {
	return &services.LLMResponse{Response: `{
		"md_summary": "Audio summary here.",
		"name": "Audio name",
		"quiz_json": [
			{
			"answer": "Список команд",
			"options": [
				"Набор файлов",
				"Список команд",
				"Графическое изображение",
				"Системная настройка"
			],
			"question": "Что понимается под меню в операционной системе Windows согласно тексту?"
			}
		],
		"transcription": "Transcript audio here",
		"language": "ru"
		}`,

		InputTokenCount:    10,
		TotalTokenCount:    11,
		ThoughtsTokenCount: 12,
		OutputTokenCount:   13,
	}, nil
}

func (m MockGoogleTranscriber) ImageOrPdfParse(filePaths []string, modelName services.LLMModelName) (*services.LLMResponse, error) {
	return &services.LLMResponse{Response: `{
		"md_summary": "Image summary here.",
		"name": "Image name here",
		"quiz_json": [
			{
			"answer": "Список команд",
			"options": [
				"Набор файлов",
				"Список команд",
				"Графическое изображение",
				"Системная настройка"
			],
			"question": "Что понимается под меню в операционной системе Windows согласно тексту?"
			}
		],
		"transcription": "Hello there",
		"language": "ru"
		}`,
		InputTokenCount:    10,
		TotalTokenCount:    11,
		ThoughtsTokenCount: 12,
		OutputTokenCount:   13,
	}, nil
}

func (m MockGoogleTranscriber) ExamParse(text *string, filePaths []string, modelName services.LLMModelName) (*services.LLMResponse, error) {
	return &services.LLMResponse{Response: `{
		"md_summary": "Exam summary here.",
		"name": "Exam name here",
		"transcription": "Hello there",
		"language": "ru"
		}`,
		InputTokenCount:    10,
		TotalTokenCount:    11,
		ThoughtsTokenCount: 12,
		OutputTokenCount:   13,
	}, nil
}

func (m MockGoogleTranscriber) TextParse(text string, modelName services.LLMModelName) (*services.LLMResponse, error) {
	return &services.LLMResponse{Response: `{
		"md_summary": "Exam summary here.",
		"name": "Exam name here",
		"transcription": "Hello there",
		"language": "ru"
		}`,
		InputTokenCount:    10,
		TotalTokenCount:    11,
		ThoughtsTokenCount: 12,
		OutputTokenCount:   13,
	}, nil
}

func (m MockGoogleTranscriber) GenerateQuizAndFlashCards(content string, isSourceTest bool, modelName services.LLMModelName, languageCode string) (*services.LLMResponse, error) {
	return &services.LLMResponse{Response: `{
  "easy_questions": [
    {
      "answer": "–12,9",
      "options": ["–2,5", "2,5", "12,9", "–12,9"],
      "question": "–5,2 – 7,7 ifadəsinin qiymətini tapın."
    },
    {
      "answer": "0,1",
      "options": ["0,00001", "0,0001", "0,001", "0,1"],
      "question": "Aşağıdakı ədədlərdən hansı böyükdür?"
    }
  ],
  "flashcards": [
    {
      "answer": "Rasional və irrasional ədədlər çoxluğunun birləşməsi.",
      "question": "Həqiqi ədədlər nədir?"
    },
    {
      "answer": "Sıfırdan böyük ədədlər.",
      "question": "Müsbət ədədlər necə ifadə olunur?"
    }
  ],
  "hard_questions": [
    {
      "answer": "228",
      "options": ["80", "301", "228", "220"],
      "question": "147 ədədinin müxtəlif bölənlərinin cəmini tapın."
    },
    {
      "answer": "234",
      "options": ["90", "233", "235", "234"],
      "question": "153 ədədinin müxtəlif bölənlərinin cəmini tapın."
    }
  ]
}`,
		InputTokenCount:    10,
		TotalTokenCount:    11,
		ThoughtsTokenCount: 12,
		OutputTokenCount:   13,
	}, nil
}

func (m MockGoogleTranscriber) DocumentsParse(filePaths []string, userTranscript string, modelName services.LLMModelName) (*services.LLMResponse, error) {
	return &services.LLMResponse{Response: `{
		"md_summary": "Document summary here.",
		"name": "Document name here",
		"quiz_json": [
			{
			"answer": "Список команд",
			"options": [
				"Набор файлов",
				"Список команд",
				"Графическое изображение",
				"Системная настройка"
			],
			"question": "Что понимается под меню в операционной системе Windows согласно тексту?"
			}
		],
		"transcription": "Hello there",
		"language": "ru"
		}`,
		InputTokenCount:    10,
		TotalTokenCount:    11,
		ThoughtsTokenCount: 12,
		OutputTokenCount:   13,
	}, nil
}
