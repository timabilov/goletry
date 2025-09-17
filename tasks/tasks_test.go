package tasks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"lessnoteapi/dbhelper"
	"lessnoteapi/models"
	"lessnoteapi/services"
	"lessnoteapi/test"

	"github.com/stretchr/testify/assert"
)

func stringPtr(s string) *string {
	return &s
}

func TestHandleTranscribeNoteTask(t *testing.T) {

	db := dbhelper.SetupTestDB()
	dbhelper.SetupCleaner(db)
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	// asynqClient := asynq.NewClient(asynq.RedisClientOpt{
	// 	Addr: "localhost:6379",
	// })
	// e := controllers.SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, asynqClient, nil)
	user := test.FakeUser(db, nil)
	var note models.Note = models.Note{
		Name:       "Test Note",
		Status:     "draft",
		Transcript: nil,
		NoteType:   stringPtr("youtube"),
		OwnerID:    user.ID,
		CompanyID:  user.Memberships[0].CompanyID,
		FileUrl:    stringPtr("notes/test-file-key.m4a"),
	}
	db.Create(&note)

	// Create a test note

	mockFileContent, err := os.ReadFile("samplerecord.zip")
	if err != nil {
		t.Fatalf("Failed to open test zip file: %v", err)
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(mockFileContent)
	}))
	defer mockServer.Close()

	fakeTask, err := NewTranscribeNoteTask(note.ID)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}
	awsServiceMock := &test.AWSProviderMock{MockUrl: mockServer.URL}

	err = HandleInitialProcessNoteTask(context.Background(), fakeTask, db, services.GoogleLLMNoteProcessor{}, awsServiceMock)
	assert.NoError(t, err)
	// Assertions

	// Verify note status in database
	var updatedNote models.Note
	err = db.Where("id = ?", note.ID).First(&updatedNote).Error
	assert.Equal(t, "transcribed", updatedNote.Status)
	assert.Equal(t, stringPtr("Transcript audio here"), updatedNote.Transcript)
	assert.Equal(t, "ru", updatedNote.Language)
}

func TestHandleGenerateStudyMaterialForNoteTask(t *testing.T) {

	db := dbhelper.SetupTestDB()
	dbhelper.SetupCleaner(db)
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	// asynqClient := asynq.NewClient(asynq.RedisClientOpt{
	// 	Addr: "localhost:6379",
	// })
	// e := controllers.SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, asynqClient, nil)
	user := test.FakeUser(db, nil)
	var note models.Note = models.Note{
		Name:       "Test Note",
		Status:     "draft",
		Transcript: stringPtr("Some math problems"),
		NoteType:   stringPtr("test"),
		OwnerID:    user.ID,
		CompanyID:  user.Memberships[0].CompanyID,
		FileUrl:    stringPtr("examtestimages.zip"),
	}
	db.Create(&note)

	// Create a test note

	mockFileContent, err := os.ReadFile("examtestimages.zip")
	if err != nil {
		t.Fatalf("Failed to open test zip file: %v", err)
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(mockFileContent)
	}))
	defer mockServer.Close()

	fakeTask, err := NewTranscribeNoteTask(note.ID)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}
	awsServiceMock := &test.AWSProviderMock{MockUrl: mockServer.URL}

	// err = HandleTranscribeNoteTask(context.Background(), fakeTask, test.MockGoogleTranscriber{}, awsServiceMock)
	err = HandleInitialProcessNoteTask(context.Background(), fakeTask, db, test.MockGoogleTranscriber{}, awsServiceMock)
	assert.NoError(t, err)
	// Assertions

	// Verify note status in database
	var updatedNote models.Note
	err = db.Where("id = ?", note.ID).First(&updatedNote).Error
	assert.Equal(t, "ready_to_generate", updatedNote.QuizStatus)

	err = HandleGeneratStudyForNoteTask(context.Background(), fakeTask, db, test.MockGoogleTranscriber{}, awsServiceMock)
	var questionCount int64
	err = db.Where("id = ?", note.ID).First(&updatedNote).Error
	assert.Equal(t, "generated", updatedNote.QuizStatus)
	assert.Equal(t, uint(1), updatedNote.QuestionGeneratedCount)
	db.Where("note_id = ?", note.ID).Model(models.Question{}).Count(&questionCount)
	assert.Equal(t, int64(4), questionCount)
	assert.NoError(t, err)
}

func TestHandleDownloadYoutubeTask(t *testing.T) {

	db := dbhelper.SetupTestDB()
	dbhelper.SetupCleaner(db)
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	// asynqClient := asynq.NewClient(asynq.RedisClientOpt{
	// 	Addr: "localhost:6379",
	// })
	// e := controllers.SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, asynqClient, nil)
	user := test.FakeUser(db, nil)
	var note models.Note = models.Note{
		Name:       "Test Note",
		Status:     "draft",
		Transcript: stringPtr("Some math problems"),
		NoteType:   stringPtr("test"),
		OwnerID:    user.ID,
		CompanyID:  user.Memberships[0].CompanyID,
		FileUrl:    stringPtr("examtestimages.zip"),
		YoutubeUrl: stringPtr("https://www.youtube.com/watch?v=7HZsPCQaNZI"),
	}
	db.Create(&note)

	// Create a test note

	mockFileContent, err := os.ReadFile("examtestimages.zip")
	if err != nil {
		t.Fatalf("Failed to open test zip file: %v", err)
	}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(mockFileContent)
	}))
	defer mockServer.Close()

	fakeTask, err := NewTranscribeNoteTask(note.ID)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}
	awsServiceMock := &test.AWSProviderMock{MockUrl: mockServer.URL}
	// awsServiceMock := services.AWSService{}
	// awsServiceMock.InitPresignClient(context.Background())

	// err = HandleTranscribeNoteTask(context.Background(), fakeTask, test.MockGoogleTranscriber{}, awsServiceMock)
	err = HandleDownloadYoutubeTask(context.Background(), fakeTask, db, test.MockGoogleTranscriber{}, awsServiceMock)
	assert.NoError(t, err)
	// Assertions

}
