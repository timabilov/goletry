package controllers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lessnoteapi/models"
	"lessnoteapi/services"
	"lessnoteapi/tasks"

	firebase "firebase.google.com/go/v4"
	"github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// NoteRoutes struct to hold dependencies
// UpdateNoteRequest defines the request body for updating quiz alerts
type UpdateNoteRequest struct {
	// QuizAlertsEnabled  *bool `json:"quiz_alerts_enabled" validate:"required"`
	AlertWhenProcessed *bool `json:"alert_when_processed"`
}

type NoteRoutes struct {
	AWSService  services.AWSServiceProvider
	FirebaseApp *firebase.App
}

// Request structs for validation
type CreateNoteRequest struct {
	NoteType     string  `json:"note_type" validate:"required,oneof=audio youtube image pdf text test multi"`
	Name         string  `json:"name" validate:"required,max=100"`
	FileName     *string `json:"file_name" validate:"required,max=1000"`
	Transcript   *string `json:"transcript" validate:"omitempty"`
	LanguageCode *string `json:"language_code" validate:"omitempty,oneof=auto zh hi en id ur ha pt bn ru es ja am ph ar vi sw tr fa de th fr it zu ko my uk ki az kk ms si km lo ne pl ps ku ta ak ro hu cs el he mn uz tg ka hy da fi nl no sv"`
	FolderID     *uint   `json:"folder_id" validate:"omitempty"`
	Language     string  `json:"language" validate:"omitempty,oneof=zh hi en id ur ha pt bn ru es ja am ph ar vi sw tr fa de th fr it zu ko my uk ki az kk ms si km lo ne pl ps ku ta ak ro hu cs el he mn uz tg ka hy da fi nl no sv"`
	YoutubeURL   *string `json:"youtube_url" validate:"omitempty"`
}

type NoteUploadFileRequest struct {
	FileName *string `json:"file_name" validate:"required,max=1000"`
}

type CreateFolderRequest struct {
	Name string `json:"name" validate:"required,max=100"`
}

type GenericResponse struct {
	message string
}

// Response structs
type NoteResponse struct {
	ID                 uint              `json:"id"`
	LanguageCode       *string           `json:"language_code"`
	MDSummaryAI        *string           `json:"md_summary_ai"`
	NoteType           string            `json:"note_type"`
	FileUrl            *string           `json:"file_url"`
	FileName           *string           `json:"file_name"`
	OwnerID            uint              `json:"owner_id"`
	Name               string            `json:"name"`
	Transcript         string            `json:"transcript"`
	FolderID           *uint             `json:"folder_id"`
	Language           string            `json:"language"`
	CreatedAt          string            `json:"created_at"`
	Status             string            `json:"status"`
	FlashCardsJson     *string           `json:"flashcards_json"`
	QuizJson           *string           `json:"quiz_json"`
	QuizAlertsEnabled  bool              `json:"quiz_alerts_enabled"`
	QuizStatus         string            `json:"quiz_status"`
	YoutubeURL         *string           `json:"youtube_url"`
	YoutubeId          *string           `json:"youtube_id"`
	InputTokenCount    *int32            `json:"prompt_token_count"`
	ThoughtsTokenCount *int32            `json:"thoughts_token_count"`
	OuputTokenCount    *int32            `json:"output_token_count"`
	TotalTokenCount    *int32            `json:"total_token_count"`
	Questions          []models.Question `json:"questions"`

	// on note 'list' only. fraction
	NoteProgress               float32  `json:"note_progress"`
	ProcessingErrorMessage     *string  `json:"processing_error_message"`
	ProcessingQuizErrorMessage *string  `json:"processing_quiz_error_message"`
	AlertWhenProcessed         bool     `json:"alert_when_processed"`
	TotalDuration              *float64 `json:"total_duration"`
}

type NoteCreatedResponse struct {
	NoteResponse
	FileUploadUrl string `json:"file_upload_presign_url"`
	// FileUploadUrl string `json:"file_upload_presign_url"`
}

type FolderResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type NotesResponse struct {
	Notes []NoteResponse `json:"notes"`
}

func (controller *NoteRoutes) NoteRoutes(g *echo.Group) {
	g.POST("/create", controller.CreateNote)
	g.POST("/folder/create", controller.CreateFolder)
	g.GET("/all", controller.GetNotes)
	g.POST("/request-batch-file-upload", controller.batchUploadFiles)
	g.GET("/:noteId", controller.GetNote)
	g.GET("/:noteId/documents-url", controller.GetNoteDocumentsUrl)
	g.GET("/:noteId/questions", controller.GetNoteQuestions)
	g.POST("/:noteId/questions/:questionId/answer", controller.AnswerNoteQuestion)
	g.PUT("/:noteId/setAsUploaded", controller.SetAsUploaded)
	g.PUT("/:noteId/generateFileUploadLink", controller.GenerateNoteUploadLink)
	g.PUT("/:noteId/toggleQuizAlerts", controller.toggleQuizAlerts)
	g.PUT("/:noteId/generate-for-study", controller.GenerateStudyMaterial)
	g.PATCH("/:noteId/update", controller.UpdateNoteState)
}

func (controller *NoteRoutes) CreateNote(c echo.Context) error {
	var req CreateNoteRequest
	if err := c.Bind(&req); err != nil {
		fmt.Println(err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate request
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Validate YouTube URL if provided
	var videoId *string
	if req.YoutubeURL != nil && *req.YoutubeURL != "" {
		videoIdString, youtubeIdErr := tasks.ExtractYoutubeID(*req.YoutubeURL)
		videoId = &videoIdString
		if youtubeIdErr != nil {
			fmt.Println("Parse youtube error", youtubeIdErr)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid YouTube URL format, example: https://www.youtube.com/watch?v=XXXXXXX"})
		}
	}

	// Get user and db from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection error"})
	}
	company := user.Memberships[0].Company
	if string(company.Subscription) == "free" {
		var totalNoteCount int64
		// if currentCompany.EnforcedDailyNoteLimit == nil {
		if err := db.Model(&models.Note{}).Where("company_id = ?", company.ID).Count(&totalNoteCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get note data"})
		}
		fmt.Printf("[User %v] Free plan, note count: %v", user.ID, totalNoteCount)
		if totalNoteCount >= 2 {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "You have reached the free limit of total 2 notes, please subscribe"})
		}
	}

	if company.EnforcedDailyNoteLimit != nil {
		// get daily note count of user
		var dailyNoteCount int64
		today := time.Now().UTC().Format("2006-01-02")
		if err := db.Model(&models.Note{}).Where("company_id = ? AND DATE(created_at) = ?", company.ID, today).Count(&dailyNoteCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get note data"})
		}
		fmt.Printf("[User %v] Enforced daily limit, note count: %v", user.ID, dailyNoteCount)
		if dailyNoteCount >= int64(*company.EnforcedDailyNoteLimit) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": fmt.Sprintf("You have reached the limit of %v daily notes. Please wait for the next day.", dailyNoteCount)})
		}
	}
	note := models.Note{
		Name:       req.Name,
		Transcript: req.Transcript,
		NoteType:   &req.NoteType,
		OwnerID:    user.ID,
		FolderID:   req.FolderID,
		Language:   req.Language,
		Status:     "draft",
		CompanyID:  user.Memberships[0].CompanyID,
		YoutubeUrl: req.YoutubeURL,
		YoutubeId:  videoId,
		// Company:    user.Memberships[0].Company,
	}
	if req.NoteType == "text" {

		note.Transcript = req.Transcript

	}
	if req.LanguageCode != nil {
		note.LanguageCode = req.LanguageCode
	}
	// If folder ID is provided, verify it exists and belongs to user
	if req.FolderID != nil {
		var folder models.Folder
		if err := db.Where("id = ? AND owner_id = ?", *req.FolderID, user.ID).First(&folder).Error; err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid folder"})
		}
	}

	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	var uploadUrl string
	var presignErr error
	if req.FileName != nil && *req.FileName != "" {
		// todo clean and map the same file name as in FE UI otherwise **FAIL**
		safeFileName := fmt.Sprintf("notes/%s", *req.FileName)

		// uploadUrl, presignErr = controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
		// fileName := strings.ReplaceAll(*req.FileName, " ", "")
		// fileName = strings.ReplaceAll(*req.FileName, "-", "")
		// safeFileName := fmt.Sprintf("notes/%s", fileName)
		uploadUrl, presignErr = controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
		note.FileUrl = &safeFileName
		if presignErr != nil {
			log.Printf("Unable to presign generate for %s!, %s", note.Name, presignErr)
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Error while creating note with attachment",
			})
		}
	}
	// Save to database
	if err := db.Create(&note).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create note"})
	}

	// Prepare response
	response := NoteCreatedResponse{
		NoteResponse: NoteResponse{
			ID:           note.ID,
			MDSummaryAI:  note.ConspectAIMD5,
			Name:         note.Name,
			Transcript:   *note.Transcript,
			NoteType:     *note.NoteType,
			FolderID:     note.FolderID,
			Language:     note.Language,
			CreatedAt:    note.CreatedAt.Format("2006-01-02T15:04:05Z"),
			Status:       note.Status,
			LanguageCode: note.LanguageCode,
		},
		FileUploadUrl: uploadUrl,
	}

	return c.JSON(http.StatusCreated, response)
}

func (controller *NoteRoutes) GenerateNoteUploadLink(c echo.Context) error {
	// Get user from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	var req NoteUploadFileRequest
	if err := c.Bind(&req); err != nil {
		fmt.Println(err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate request
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Get database from context
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Our service is not available, please try again a bit later"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("id = ? AND owner_id = ?", noteId, user.ID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}

	// Update note status
	if note.Status != "draft" {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Note is already created"})
	}
	// Return success response
	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	if req.FileName != nil && *req.FileName != "" {
		// todo clean and map the same file name as in FE UI otherwise **FAIL**
		safeFileName := fmt.Sprintf("notes/%s", *req.FileName)

		// uploadUrl, presignErr = controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
		// fileName := strings.ReplaceAll(*req.FileName, " ", "")
		// fileName = strings.ReplaceAll(*req.FileName, "-", "")
		// safeFileName := fmt.Sprintf("notes/%s", fileName)
		uploadUrl, presignErr := controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
		if presignErr != nil {
			log.Printf("Unable to presign generate for %s!, %s", note.Name, presignErr)
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Error while creating note with attachment",
			})
		}
		note.FileUrl = &safeFileName
		if err := db.Save(&note).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update note status"})
		}
		return c.JSON(http.StatusOK, map[string]string{"message": "Note is updated successfully", "upload_url": uploadUrl, "file_name": *req.FileName})
	} else {
		fmt.Printf("[Note %v] File name is empty but it is expected from FE! ", note.ID)
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Please try again."})
	}
}

func (controller *NoteRoutes) SetAsUploaded(c echo.Context) error {
	// Get user from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	// Get database from context
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Our service is not available, please try again a bit later"})
	}
	asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("id = ? AND owner_id = ?", noteId, user.ID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}

	// Update note status
	if note.Status == "draft" || note.Status == "uploaded" {
		note.Status = "uploaded"
		if err := db.Save(&note).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update note status"})
		}
	} else {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Note is already created"})
	}
	noteType := strings.ToLower(*note.NoteType)
	if noteType != "youtube" {

		task, err := tasks.NewTranscribeNoteTask(note.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create note, please try again later"})
		}
		info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Queue("transcribe"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create note, please try again later"})
		}
		fmt.Println("[Queue] Transcribe task submitted, Note ID: ", note.ID, " Task ID: ", info.ID)
	} else if noteType == "youtube" {
		youtubeUrl := note.YoutubeUrl
		if youtubeUrl == nil || *youtubeUrl == "" {
			fmt.Printf("[Note %v] Youtube URL is empty, but note type is youtube", note.ID)
			sentry.CaptureException(fmt.Errorf("[Note %v] Youtube URL is empty, but note type is  youtube", note.ID))
			return c.JSON(http.StatusBadRequest, map[string]string{"message": "Something went wrong, please try again by creating new note"})
		}

		// youtubeLink := *youtubeUrl

		// if !youtubeVideoRegex.MatchString(youtubeLink) {
		// 	return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid YouTube URL format, example: https://www.youtube.com/watch?v=XXXXXXX"})
		// }
		task, err := tasks.NewExtractYoutubeAudioTask(note.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create note, please try again later"})
		}
		info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Queue("youtube"))
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create note, please try again later"})
		}
		fmt.Println("[Queue] Youtube Transcribe task submitted, Note ID: ", note.ID, " Task ID: ", info.ID)
	}
	// Return success response
	return c.JSON(http.StatusOK, map[string]string{"message": "Note is updated successfully"})
}

type GenericToggleSettingsIn struct {
	Enabled bool `json:"enabled"`
}

func (controller *NoteRoutes) toggleQuizAlerts(c echo.Context) error {
	// Get user from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	// Get database from context
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Our service is not available, please try again a bit later"})
	}
	var req GenericToggleSettingsIn
	if err := c.Bind(&req); err != nil {
		fmt.Println(err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate request
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	// Find note and verify ownership
	var note models.Note
	if err := db.Where("id = ? AND owner_id = ?", noteId, user.ID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}

	note.QuizAlertsEnabled = req.Enabled
	if err := db.Save(&note).Error; err != nil {
		sentry.CaptureException(fmt.Errorf("[Note %v] Error on updating quiz alert status", note.ID))
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update note quiz alert status"})
	}
	fmt.Printf("[Note %v] Quiz toggle new status: %v \n", note.ID, note.QuizAlertsEnabled)
	// fbApp := controller.FirebaseApp
	time.Sleep(time.Second * 2)
	// services.SendNotification(fbApp, db, user.ID, "Quiz Alerts", fmt.Sprintf("Quiz alerts are %s", map[bool]string{true: "enabled", false: "disabled"}[note.QuizAlertsEnabled]))

	if note.QuizStatus == "generated" || note.QuizStatus == "in_progress" {
		fmt.Println("[Note ", note.ID, "] Alerts: Quiz is already generated or in progress, skip generating new quiz")
	} else {
		asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
		if !ok {
			fmt.Println("Service is not available, please try again a bit later")
			sentry.CaptureException(fmt.Errorf("[Note %v] Alert Error on getting asynq client from context", note.ID))
		} else {

			task, err := tasks.NewStudyGenerationeNoteTask(note.ID)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create note, please try again later"})
			}
			info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Queue("generatestudy"))
			if err != nil {
				fmt.Println(err)
				sentry.CaptureException(fmt.Errorf("[Note %v] Error on enqueue study generation task", note.ID))
			}
			note.QuizStatus = "in_progress"
			if err := db.Save(&note).Error; err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update note status"})
			}
			fmt.Println("[Queue] Study generation task submitted, Note ID: ", note.ID, " Task ID: ", info.ID)
		}
	}
	// Return success response
	return c.JSON(http.StatusOK, map[string]string{"message": "Note is updated successfully"})
}

func (controller *NoteRoutes) CreateFolder(c echo.Context) error {
	var req CreateFolderRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate request
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Get user and db from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection error"})
	}

	// Create folder
	folder := models.Folder{
		Name:    req.Name,
		OwnerID: user.ID,
	}

	// Save to database
	if err := db.Create(&folder).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to create folder"})
	}

	// Prepare response
	response := FolderResponse{
		ID:        folder.ID,
		Name:      folder.Name,
		CreatedAt: folder.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return c.JSON(http.StatusCreated, response)
}

func (controller *NoteRoutes) GetNotes(c echo.Context) error {
	// Get user and db from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection error"})
	}

	// Get folder ID from query params (optional)
	folderIDStr := c.QueryParam("folder_id")
	var folderID *uint
	if folderIDStr != "" {
		fID, err := strconv.ParseUint(folderIDStr, 10, 32)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid folder ID"})
		}
		fID32 := uint(fID)
		folderID = &fID32

		// Verify folder exists and belongs to user
		var folder models.Folder
		if err := db.Where("id = ? AND owner_id = ?", *folderID, user.ID).First(&folder).Error; err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid folder"})
		}
	}

	// Query notes
	var notes []models.Note
	query := db.Where("owner_id = ? AND deleted = ?", user.ID, false)
	if folderID != nil {
		query = query.Where("folder_id = ?", *folderID)
	} else {
		query = query.Where("folder_id IS NULL OR folder_id != 0")
	}
	if err := query.Order("created_at DESC").Preload("Questions").Find(&notes).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve notes"})
	}

	// Prepare response
	response := NotesResponse{
		Notes: make([]NoteResponse, len(notes)),
	}
	for i, note := range notes {
		totalQuestions := len(note.Questions)
		correctAnswers := 0
		for _, question := range note.Questions {
			if question.UserAnswer != "" && question.UserAnswer == question.Answer {
				correctAnswers++
			}
		}

		noteProgress := 0.0
		if totalQuestions > 0 {
			noteProgress = float64(correctAnswers) / float64(totalQuestions)
		}

		response.Notes[i] = NoteResponse{
			ID:       note.ID,
			NoteType: *note.NoteType,
			Status:   note.Status,
			// Deleted:  note.Deleted,
			FileUrl:           note.FileUrl,
			FileName:          note.FileName,
			OwnerID:           note.OwnerID,
			QuizAlertsEnabled: note.QuizAlertsEnabled,
			Name:              note.Name,
			// Transcript:        transcript,
			FolderID:  note.FolderID,
			Language:  note.Language,
			CreatedAt: note.CreatedAt.Format("2006-01-02T15:04:05Z"),
			//FlashCardsJson:    note.FlashcardsJSON,
			//QuizJson:          note.QuizJSON,
			NoteProgress:           float32(noteProgress),
			ProcessingErrorMessage: note.ProcessingErrorMessage,
		}
	}

	return c.JSON(http.StatusOK, response)
}

func (controller *NoteRoutes) GetNote(c echo.Context) error {
	// Get user from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	// Get database from context
	db, ok := c.Get("__db").(*gorm.DB)
	// asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("notes.id = ? AND owner_id = ?", noteId, user.ID).Preload("Questions").First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}
	// BECAUSE STATUS PING - WE ABUSE R2 storage bill!
	// var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	// var fileUrl string = ""
	// if note.FileUrl != nil && *note.FileUrl != "" {
	// 	fileUrl, err = controller.AWSService.GetPresignedR2FileReadURL(context.TODO(), bucketName, *note.FileUrl)
	// 	if err != nil {
	// 		log.Printf("Unable to presign generate for %s!, %s", note.Name, err)
	// 		sentry.CaptureException(errors.New(fmt.Sprintf("[Get Note] Error on retrieving note FILE for returning %v", note.ID)))
	// 		return c.JSON(http.StatusInternalServerError, echo.Map{
	// 			"message": "Error while creating note with attachment",
	// 		})
	// 	}
	// }

	// Prepare response
	// isAdmin := user.Email == "tamerlan.abilov"
	response := NoteResponse{
		ID:                     note.ID,
		NoteType:               *note.NoteType,
		MDSummaryAI:            note.ConspectAIMD5,
		FlashCardsJson:         note.FlashcardsJSON,
		QuizJson:               note.QuizJSON,
		FileUrl:                nil,
		FileName:               note.FileName,
		OwnerID:                note.OwnerID,
		Name:                   note.Name,
		Transcript:             *note.Transcript,
		FolderID:               note.FolderID,
		Language:               note.Language,
		Status:                 note.Status,
		CreatedAt:              note.CreatedAt.Format("2006-01-02T15:04:05Z"),
		InputTokenCount:        note.InputTokenCount,
		TotalTokenCount:        note.TotalTokenCount,
		ThoughtsTokenCount:     note.ThoughtsTokenCount,
		OuputTokenCount:        note.OuputTokenCount,
		Questions:              note.Questions,
		QuizStatus:             note.QuizStatus,
		QuizAlertsEnabled:      note.QuizAlertsEnabled,
		YoutubeURL:             note.YoutubeUrl,
		YoutubeId:              note.YoutubeId,
		ProcessingErrorMessage: note.ProcessingErrorMessage,
		AlertWhenProcessed:     note.AlertWhenProcessed,
		TotalDuration:          note.TotalDuration,
	}
	return c.JSON(http.StatusOK, response)

}

func (controller *NoteRoutes) GetNoteDocumentsUrl(c echo.Context) error {
	// Get user from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	// Get database from context
	db, ok := c.Get("__db").(*gorm.DB)
	// asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("notes.id = ? AND owner_id = ?", noteId, user.ID).Preload("Questions").First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}
	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	var fileUrl string = ""
	if note.FileUrl != nil && *note.FileUrl != "" {
		fileUrl, err = controller.AWSService.GetPresignedR2FileReadURL(context.TODO(), bucketName, *note.FileUrl)
		if err != nil {
			log.Printf("Unable to presign generate for %s!, %s", note.Name, err)
			sentry.CaptureException(errors.New(fmt.Sprintf("[Get Note] Error on retrieving note FILE for returning %v", note.ID)))
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Error while creating note with attachment",
			})
		}
	}
	// Prepare response
	// isAdmin := user.Email == "tamerlan.abilov"
	response := NoteResponse{
		ID:      note.ID,
		Name:    note.Name,
		FileUrl: stringPtr(fileUrl),
	}
	return c.JSON(http.StatusOK, response)

}

func (controller *NoteRoutes) GetNoteQuestions(c echo.Context) error {
	// Get user from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	// Get database from context
	db, ok := c.Get("__db").(*gorm.DB)
	// asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("notes.id = ? AND owner_id = ?", noteId, user.ID).Preload("Questions").First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}

	// Prepare response
	// isAdmin := user.Email == "tamerlan.abilov"
	response := NoteResponse{
		ID:                         note.ID,
		Name:                       note.Name,
		Questions:                  note.Questions,
		QuizStatus:                 note.QuizStatus,
		QuizAlertsEnabled:          note.QuizAlertsEnabled,
		FlashCardsJson:             note.FlashcardsJSON,
		ProcessingErrorMessage:     note.ProcessingErrorMessage,
		ProcessingQuizErrorMessage: note.ProcessingQuizErrorMessage,
	}
	return c.JSON(http.StatusOK, response)

}

type AnswerNoteQuestionRequest struct {
	Answer string `json:"answer" validate:"required"`
}

func (controller *NoteRoutes) AnswerNoteQuestion(c echo.Context) error {
	// Get user from context
	var req AnswerNoteQuestionRequest
	if err := c.Bind(&req); err != nil {
		fmt.Println(err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Validate request
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}
	questionIdStr := c.Param("questionId")
	questionId, err := strconv.Atoi(questionIdStr)
	if err != nil || questionId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid question ID"})
	}

	// Get database from context
	db, ok := c.Get("__db").(*gorm.DB)
	// asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("notes.id = ? AND owner_id = ?", noteId, user.ID).Preload("Questions").First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}

	var question models.Question

	if err := db.Where("id = ? AND note_id = ?", questionId, note.ID).First(&question).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Question not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch question"})
	}

	question.UserAnswer = req.Answer
	now := time.Now()
	question.UserAnsweredDate = &now
	if err := db.Save(&question).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to save answer"})
	}
	// Prepare response
	// isAdmin := user.Email == "tamerlan.abilov"
	response := GenericResponse{
		message: "Answer saved successfully",
	}
	return c.JSON(http.StatusOK, response)

}

func (controller *NoteRoutes) GenerateStudyMaterial(c echo.Context) error {
	// Get user from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}

	// Parse and validate noteId
	noteIdStr := c.Param("noteId")
	noteId, err := strconv.Atoi(noteIdStr)
	if err != nil || noteId <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Our service is not available, please try again a bit later"})
	}
	asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("id = ? AND owner_id = ?", noteId, user.ID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}

	if note.QuizStatus == "generated" {
		return c.JSON(http.StatusOK, map[string]string{"message": "Quiz & Flashcards are already generated"})
	}
	if note.QuizStatus == "in_progress" {
		return c.JSON(http.StatusOK, map[string]string{"message": "Quiz & Flashcards are already in progress"})
	}
	task, err := tasks.NewStudyGenerationeNoteTask(note.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create note, please try again later"})
	}
	info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Queue("generatestudy"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to create note, please try again later"})
	}
	note.QuizStatus = "in_progress"
	if err := db.Save(&note).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update note status"})
	}
	fmt.Println("[Queue] Generate study task submitted, Note ID: ", note.ID, " Task ID: ", info.ID)
	response := GenericResponse{
		message: "Generation is in progress, please check back later",
	}
	return c.JSON(http.StatusOK, response)

}

func (controller *NoteRoutes) batchUploadFiles(c echo.Context) error {
	db := c.Get("__db").(*gorm.DB)
	user := c.Get("currentUser").(models.UserAccount)
	var request = new(models.NoteFilesUploadRequestIn)
	var noteUrls []models.NoteFileUploadRequestOut
	var noteIds []uint
	noteToImageName := map[uint]string{}
	if err := c.Bind(request); err != nil {
		fmt.Println("Error bind data", err)
		return err
	}
	for _, noteRequest := range request.Notes {
		noteIds = append(noteIds, noteRequest.NoteId)
		noteToImageName[noteRequest.NoteId] = noteRequest.FileName
	}
	log.Println("Request note urls with size: ", len(noteIds), " for user: ", user.ID)

	var myCompanyIds []uint
	for _, membership := range user.Memberships {
		if membership.Active && string(membership.Company.Subscription) != "free" {

			myCompanyIds = append(myCompanyIds, membership.CompanyID)
		} else {
			log.Println("non active membership for m id ", membership.ID, "name ", user.Name, "sub status", string(membership.Company.Subscription))
		}
	}
	var results []models.Note
	result := db.Where("ID in (?) and company_id in (?)", noteIds, myCompanyIds).Order("id asc").Find(&results)
	if result.Error != nil {
		log.Println("Error fetching notes for file upload ", result.Error)
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message": "Error while uploading files.",
		})
	}

	if len(results) == 0 {
		log.Println("No active memberships found for user, ignore file updates..")
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "No notes found to upload files",
		})
	}
	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	for _, note := range results {
		// Operations on each record in the batch

		localImageName := noteToImageName[note.ID]
		log.Println("Image url provided, generate presign link for ", note.ID, " local image name provided: ", localImageName)
		// prefixPath, fileName, found := strings.Split(localImageName, "/")
		nameChunks := strings.Split(localImageName, "/")
		fileName := nameChunks[0] // worst bad case
		if len(nameChunks) > 1 {
			// ideally we need that!
			fileName = nameChunks[len(nameChunks)-1]
		}
		fileName = strings.ReplaceAll(fileName, "-", "")

		dbfileName := fmt.Sprintf("%v-%v-%s", note.ID, time.Now().UnixMilli(), fileName)
		safeFileName := fmt.Sprintf("notes/%s", dbfileName)
		note.FileUrl = &safeFileName
		note.FileName = &fileName
		err := db.Select("file_url").Updates(&note).Error
		if err != nil {
			log.Printf("Error saving note link!, %v, %s", note.ID, err)
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Error while uploading images. Please try again later",
			})
		}
		url, err := controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
		if err != nil {
			log.Printf("Unable to presign generate for %s!, %s", note.Name, err)
			return c.JSON(http.StatusInternalServerError, echo.Map{
				"message": "Error while uploading images.",
			})
		}

		noteUrls = append(noteUrls, models.NoteFileUploadRequestOut{
			NoteId:    note.ID,
			UploadUrl: url,
			FileName:  localImageName,
		})
	}
	return c.JSON(http.StatusOK, models.NoteFilesUploadRequestOut{
		Notes: noteUrls,
	})
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

func (controller *NoteRoutes) UpdateNoteState(c echo.Context) error {
	// Parse and validate noteId
	noteID, err := strconv.Atoi(c.Param("noteId"))
	if err != nil || noteID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid note ID"})
	}

	// Bind and validate request
	var req UpdateNoteRequest
	// print request body as string now
	requestBody := new(bytes.Buffer)
	if _, err := requestBody.ReadFrom(c.Request().Body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
	}
	fmt.Println("Update note request body: ", requestBody.String())
	c.Request().Body = io.NopCloser(requestBody) // Reset the body so it can be read again
	// Bind again
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Invalid request body"})
	}
	if err := c.Validate(req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": err.Error()})
	}

	// Get user and db from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
	}
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Database connection error"})
	}

	// Find note and verify ownership
	var note models.Note
	if err := db.Where("id = ? AND owner_id = ?", noteID, user.ID).First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(http.StatusNotFound, map[string]string{"message": "Note not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to fetch note"})
	}

	// Update QuizAlertsEnabled
	// if req.QuizAlertsEnabled != nil {

	// 	note.QuizAlertsEnabled = *req.QuizAlertsEnabled
	// }
	if req.AlertWhenProcessed != nil {
		fmt.Println("[Note ", note.ID, "] Update request received: ", "Alert when processed - ", *req.AlertWhenProcessed)

	} else {
		fmt.Println("[Note ", note.ID, "] Update request received: ", "Alert when processed is null - ", req.AlertWhenProcessed)
	}
	updates := make(map[string]interface{})
	if req.AlertWhenProcessed != nil {
		updates["alert_when_processed"] = *req.AlertWhenProcessed
		note.AlertWhenProcessed = *req.AlertWhenProcessed
	}
	if err := db.Debug().Model(&note).Updates(updates).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update note"})
	}
	// Prepare response
	response := NoteResponse{
		ID:                 note.ID,
		NoteType:           *note.NoteType,
		MDSummaryAI:        note.ConspectAIMD5,
		FlashCardsJson:     note.FlashcardsJSON,
		QuizJson:           note.QuizJSON,
		FileUrl:            note.FileUrl,
		FileName:           note.FileName,
		OwnerID:            note.OwnerID,
		Name:               note.Name,
		Transcript:         *note.Transcript,
		FolderID:           note.FolderID,
		Language:           note.Language,
		Status:             note.Status,
		CreatedAt:          note.CreatedAt.Format("2006-01-02T15:04:05Z"),
		InputTokenCount:    note.InputTokenCount,
		TotalTokenCount:    note.TotalTokenCount,
		ThoughtsTokenCount: note.ThoughtsTokenCount,
		OuputTokenCount:    note.OuputTokenCount,
		Questions:          note.Questions,
		QuizStatus:         note.QuizStatus,
		QuizAlertsEnabled:  note.QuizAlertsEnabled,
		AlertWhenProcessed: note.AlertWhenProcessed,
	}

	return c.JSON(http.StatusOK, response)
}
