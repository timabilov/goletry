package tasks

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"letryapi/dbhelper"
	"letryapi/models"
	"letryapi/services"
	"letryapi/test"

	"github.com/stretchr/testify/assert"
)

func stringPtr(s string) *string {
	return &s
}

func TestTryOnGeneratingTask(t *testing.T) {
	fmt.Println("Starting TestTryOnGeneratingTask")
	db := dbhelper.SetupTestDB()
	dbhelper.SetupCleaner(db)
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	// asynqClient := asynq.NewClient(asynq.RedisClientOpt{
	// 	Addr: "localhost:6379",
	// })
	// e := controllers.SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, asynqClient, nil)
	user := test.FakeUser(db, nil)

	var clothingTop models.Clothing = models.Clothing{
		Name:         "Test Clothing",
		Description:  stringPtr("This is a test clothing item"),
		ClothingType: "top",
		ImageURL:     stringPtr("test-image.jpg"),
		OwnerID:      user.ID,
		CompanyID:    user.Memberships[0].CompanyID,
	}
	db.Create(&clothingTop)
	var clothingBottom models.Clothing = models.Clothing{
		Name:         "Test Clothing",
		Description:  stringPtr("This is a test clothing item"),
		ClothingType: "bottom",
		ImageURL:     stringPtr("test-image.jpg"),
		OwnerID:      user.ID,
		CompanyID:    user.Memberships[0].CompanyID,
	}
	db.Create(&clothingBottom)

	var tryOn models.ClothingTryonGeneration = models.ClothingTryonGeneration{
		Status:           "pending",
		UserAccountID:    user.ID,
		CompanyID:        user.Memberships[0].CompanyID,
		TopClothingID:    &clothingTop.ID,
		BottomClothingID: &clothingBottom.ID,
		// FileUrl:    stringPtr("examtestimages.zip"),
	}
	db.Create(&tryOn)

	// Create a test note

	mockTopContent, err := os.ReadFile("shirt.jpeg")
	if err != nil {
		t.Fatalf("Failed to open test zip file: %v", err)
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(mockTopContent)
	}))
	defer mockServer.Close()

	fakeTask, err := NewTryOnGenerationTask(user.ID, tryOn.ID)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}
	awsServiceMock := &test.AWSProviderMock{MockUrl: mockServer.URL}

	// err = HandleTranscribeNoteTask(context.Background(), fakeTask, test.MockGoogleTranscriber{}, awsServiceMock)
	err = HandleTryOnGenerationTask(context.Background(), fakeTask, db, &services.GoogleLLMProcessor{}, awsServiceMock)
	assert.NoError(t, err)
	// Assertions

	// // Verify note status in database
	// var updatedNote models.Note
	// err = db.Where("id = ?", note.ID).First(&updatedNote).Error
	// assert.Equal(t, "ready_to_generate", updatedNote.QuizStatus)

	// err = HandleGeneratStudyForNoteTask(context.Background(), fakeTask, db, test.MockGoogleTranscriber{}, awsServiceMock)
	// var questionCount int64
	// err = db.Where("id = ?", note.ID).First(&updatedNote).Error
	// assert.Equal(t, "generated", updatedNote.QuizStatus)
	// assert.Equal(t, uint(1), updatedNote.QuestionGeneratedCount)
	// db.Where("note_id = ?", note.ID).Model(models.Question{}).Count(&questionCount)
	// assert.Equal(t, int64(4), questionCount)
	// assert.NoError(t, err)
}
