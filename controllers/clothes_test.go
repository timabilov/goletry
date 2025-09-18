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

	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNoteOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Prepare request payload
	reqBody := CreateNoteRequest{
		Name:       "Test Note",
		Transcript: stringPtr("This is a test note content"),
		FolderID:   nil,
		NoteType:   "multi",
		FileName:   stringPtr(""),
		Language:   "en",
	}

	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/notes/create", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, "Expected status code 201 Created, got %d", rec.Body.String())

	var response ClothingResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Equal(t, reqBody.Name, response.Name)
	require.Equal(t, reqBody.Transcript, response.Transcript)
	require.Equal(t, reqBody.Language, response.Language)
}

func TestCreateNoteInvalidInput(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Prepare invalid request payload (missing required fields)
	reqBody := CreateNoteRequest{
		Name: "Test Note",
		// Transcript missing
		Language: "en",
	}

	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/notes/create", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"], "Transcript")
}

func TestCreateNoteUnauthorized(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Prepare request payload
	reqBody := CreateNoteRequest{
		Name:       "Test Note",
		Transcript: stringPtr("This is a test note content"),
		Language:   "en",
	}
	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/notes/create", user.Memberships[0].CompanyID), "", reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCreateFolderOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Prepare request payload
	reqBody := CreateFolderRequest{
		Name: "Test Folder",
	}

	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/notes/folder/create", user.Memberships[0].CompanyID), "", reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var response FolderResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, reqBody.Name, response.Name)
}

func TestCreateFolderInvalidInput(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Prepare invalid request payload (missing name)
	reqBody := CreateFolderRequest{
		Name: "", // Empty name
	}

	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/notes/folder/create", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), reqBody)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Body.String())
	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Contains(t, response["error"], "name")
}

func TestCreateFolderUnauthorized(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)
	// Prepare request payload
	reqBody := CreateFolderRequest{
		Name: "Test Folder",
	}
	body, _ := json.Marshal(reqBody)

	req := test.NewJSONAuthRequest("POST", fmt.Sprintf("/company/%v/notes/folder/create", user.Memberships[0].CompanyID), "", string(body))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Unauthorized", response["error"])
}

func TestGetNotesOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Create some test notes
	note1 := models.Note{
		Name:       "Note 1",
		Transcript: stringPtr("Content 1"),
		NoteType:   stringPtr("text"),
		OwnerID:    user.ID,
		CompanyID:  user.Memberships[0].CompanyID,
		Language:   "en",
		Status:     "active",
	}
	note2 := models.Note{
		Name:       "Note 2",
		Transcript: stringPtr("Content 2"),
		NoteType:   stringPtr("text"),
		OwnerID:    user.ID,
		CompanyID:  user.Memberships[0].CompanyID,
		Language:   "en",
		Status:     "active",
	}

	require.NoError(t, db.Create(&note1).Error)
	require.NoError(t, db.Create(&note2).Error)
	questions := []models.Question{
		{
			QuestionText: "What is the content of Note 1?",
			Answer:       "Option 1",
			UserAnswer:   "Option 1",
			NoteID:       note1.ID,
			Options:      pq.StringArray{"Option 1", "Option 2", "Option 3"},
		},
		{
			QuestionText: "What is the wrong answer of Note 1?",
			Answer:       "Option 3",
			UserAnswer:   "Option 1",
			NoteID:       note1.ID,
			Options:      pq.StringArray{"Option 1", "Option 2", "Option 3"},
		},
		{
			QuestionText: "What is the content of Note 2?",
			Answer:       "Option 2",
			UserAnswer:   "Option 2",
			NoteID:       note2.ID,
			Options:      pq.StringArray{"Option 1", "Option 2", "Option 3"},
		},
	}
	// Create questions in the database
	require.NoError(t, db.Create(&questions).Error)

	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/notes/all", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "Expected status code 200 OK, got %d: %s", rec.Code, rec.Body.String())

	var response ClothingListResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response.Notes, 2)
	assert.Equal(t, note1.Name, response.Notes[1].Name)
	assert.Equal(t, float32(0.5), response.Notes[1].NoteProgress)
	assert.Equal(t, *note1.Transcript, response.Notes[1].Transcript)
	assert.Equal(t, note2.Name, response.Notes[0].Name)
	assert.Equal(t, *note2.Transcript, response.Notes[0].Transcript)
}

func TestGetNoteSingleOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Create some test notes
	note1 := models.Note{
		Name:       "Note 1",
		Transcript: stringPtr("Content 1"),
		NoteType:   stringPtr("text"),
		CompanyID:  user.Memberships[0].CompanyID,
		OwnerID:    user.ID,
		Language:   "en",
		Status:     "active",
	}
	note2 := models.Note{
		Name:       "Note 2",
		Transcript: stringPtr("Content 2"),
		NoteType:   stringPtr("text"),
		CompanyID:  user.Memberships[0].CompanyID,
		OwnerID:    user.ID,
		Language:   "en",
		Status:     "active",
	}
	require.NoError(t, db.Create(&note1).Error)
	require.NoError(t, db.Create(&note2).Error)

	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/notes/%v", user.Memberships[0].CompanyID, note1.ID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "Expected status code 200 OK, got %d: %s", rec.Code, rec.Body.String())

	var response ClothingResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Equal(t, response.ID, note1.ID)
	require.Len(t, response.Questions, 0)
}

func TestGetNoteSingleWithQuestionsOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Create some test notes
	note1 := models.Note{
		Name:       "Note 1",
		Transcript: stringPtr("Content 1"),
		NoteType:   stringPtr("text"),
		CompanyID:  user.Memberships[0].CompanyID,
		OwnerID:    user.ID,
		Language:   "en",
		Status:     "active",
	}
	note2 := models.Note{
		Name:       "Note 2",
		Transcript: stringPtr("Content 2"),
		NoteType:   stringPtr("text"),
		CompanyID:  user.Memberships[0].CompanyID,
		OwnerID:    user.ID,
		Language:   "en",
		Status:     "active",
	}

	require.NoError(t, db.Create(&note1).Error)
	require.NoError(t, db.Create(&note2).Error)

	q1 := models.Question{
		QuestionText: "What is the content of Note 1?",
		Answer:       "Option 1",
		NoteID:       note1.ID,
		Options:      pq.StringArray{"Option 1", "Option 2", "Option 3"},
	}
	require.NoError(t, db.Create(&q1).Error)
	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/notes/%v", user.Memberships[0].CompanyID, note1.ID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "Expected status code 200 OK, got %d: %s", rec.Code, rec.Body.String())

	var response ClothingResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Equal(t, response.ID, note1.ID)
	require.Len(t, response.Questions, 1)
}

func TestGetNotesWithFolderOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Create a test folder
	folder := models.Folder{
		Name:    "Test Folder",
		OwnerID: user.ID,
	}
	require.NoError(t, db.Create(&folder).Error)

	// Create test notes, one in folder, one without
	noteInFolder := models.Note{
		Name:       "Note in Folder",
		Transcript: stringPtr("Folder Content"),
		NoteType:   stringPtr("text"),
		OwnerID:    user.ID,
		FolderID:   &folder.ID,
		Language:   "en",
		Status:     "active",
	}
	noteOutside := models.Note{
		Name:       "Note Outside",
		Transcript: stringPtr("Outside Content"),
		NoteType:   stringPtr("text"),
		OwnerID:    user.ID,
		Language:   "en",
		Status:     "active",
	}
	require.NoError(t, db.Create(&noteInFolder).Error)
	require.NoError(t, db.Create(&noteOutside).Error)

	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/notes/all?folder_id=%v", user.Memberships[0].CompanyID, folder.ID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "Expected status code 200 OK, got %d: %s", rec.Code, rec.Body.String())

	var response ClothingListResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	require.Len(t, response.Notes, 1)
	assert.Equal(t, noteInFolder.Name, response.Notes[0].Name)
	assert.Equal(t, *noteInFolder.Transcript, response.Notes[0].Transcript)
	assert.Equal(t, folder.ID, *response.Notes[0].FolderID)
}

func TestGetNotesUnauthorized(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)

	req := test.NewJSONAuthRequest("GET", "/company/1/notes/all", "", "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code, "Expected status code 401 Unauthorized, got %d", rec.Code)
}

func TestGetNotesInvalidFolder(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Use a non-existent folder ID
	invalidFolderID := uint(999)

	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/notes/all?folder_id=%v", user.Memberships[0].CompanyID, invalidFolderID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "Expected status code 400 Bad Request, got %d", rec.Code)
	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid folder", response["error"])
}

func TestGetNotesInvalidFolderIDFormat(t *testing.T) {
	db := dbhelper.SetupTestDB()
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, nil, nil)
	user := test.FakeUser(db, nil)

	// Use an invalid folder ID format
	req := test.NewJSONAuthRequest("GET", fmt.Sprintf("/company/%v/notes/all?folder_id=invalid", user.Memberships[0].CompanyID), strconv.FormatUint(uint64(user.ID), 10), "")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "Expected status code 400 Bad Request, got %d", rec.Code)
	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "Invalid folder ID", response["error"])
}

func TestSetAsUploadedOk(t *testing.T) {
	db := dbhelper.SetupTestDB()
	dbhelper.SetupCleaner(db)
	cleaner := dbhelper.SetupCleaner(db)
	defer cleaner()
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{
		Addr: "localhost:6379",
	})
	e := SetupServer(db, test.GoogleServiceMock{}, &test.AWSProviderMock{}, nil, asynqClient, nil)
	user := test.FakeUser(db, nil)
	var note models.Note = models.Note{
		Name:       "Test Note",
		Status:     "draft",
		Transcript: nil,
		NoteType:   stringPtr("audio"),
		OwnerID:    user.ID,
		CompanyID:  user.Memberships[0].CompanyID,
	}
	db.Create(&note)

	// Create a test note

	// Prepare request
	req := test.NewJSONAuthRequest(
		"PUT",
		fmt.Sprintf("/company/%v/notes/%v/setAsUploaded", user.Memberships[0].CompanyID, note.ID),
		strconv.FormatUint(uint64(user.ID), 10),
		nil,
	)
	rec := httptest.NewRecorder()

	// Execute request
	e.ServeHTTP(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code, "Expected status code 200 OK, got %d", rec.Code)

	var response map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Note is updated successfully", response["message"])

	// Verify note status in database
	var updatedNote models.Note
	err = db.Where("id = ?", note.ID).First(&updatedNote).Error
	assert.NoError(t, err)
	assert.Equal(t, "uploaded", updatedNote.Status)
}
