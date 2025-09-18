package tasks

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"letryapi/models"
	"letryapi/services"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type TryOnGenerationPayload struct {
	TryOnID uint `json:"try_on_id"`
}
type ClothingGenerationPayload struct {
	ClothingId uint `json:"clothing_id"`
}

// Client initializes an asynq client for enqueuing tasks
func NewClient() (*asynq.Client, error) {
	return asynq.NewClient(asynq.RedisClientOpt{Addr: "your-redis-connection-string"}), nil
}

// EnqueueTranscribeNote enqueues a note for processing
func NewTryOnGenerationTask(tryOnID uint) (*asynq.Task, error) {
	payload, err := json.Marshal(TryOnGenerationPayload{TryOnID: tryOnID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask("generate:tryon", payload), nil

}

func NewClothingProcessingTask(clothingId uint) (*asynq.Task, error) {
	payload, err := json.Marshal(ClothingGenerationPayload{ClothingId: clothingId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask("generate:process_clothing", payload), nil

}

// EnqueueTranscribeNote enqueues a note for processing
func NewExtractYoutubeAudioTask(noteID uint) (*asynq.Task, error) {
	payload, err := json.Marshal(TryOnGenerationPayload{TryOnID: noteID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask("youtube:ytdlp", payload), nil

}

func getFileForNote(awsService services.AWSServiceProvider, note models.Note) ([]byte, string, error) {
	bucketName := os.Getenv("R2_BUCKET_NAME")
	fmt.Printf("[Note: %v] Bucket name: %s\n", note.ID, bucketName)
	fmt.Printf("[Note: %v] Request presigned download url.. ", note.ID)
	if note.FileUrl == nil {
		return nil, "", fmt.Errorf("[Note: %v] File URL is nil", note.ID)
	}
	fileUrl, err := awsService.GetPresignedR2FileReadURL(context.TODO(), bucketName, *note.FileUrl)
	fileName := filepath.Base(*note.FileUrl)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on getting presigned URL for file %s", note.ID, *note.FileUrl))
		return nil, fileName, err
	}
	fmt.Printf("Downloading... %s\n", fileUrl)
	fileBytes, err := services.ReadFileFromUrl(fileUrl)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on downloading file %s: %v", note.ID, *note.FileUrl, err))
		return nil, fileName, err
	}

	return fileBytes, fileName, nil
}

// extractYoutubeID parses YouTube ID from various URL formats
func ExtractYoutubeID(youtubeURL string) (string, error) {
	// Regular expressions for different YouTube URL formats

	matches := services.YoutubeURLRegex.FindStringSubmatch(youtubeURL)
	if len(matches) < 2 {
		return "", fmt.Errorf("no valid YouTube video ID found in URL: %s", youtubeURL)
	}
	return matches[1], nil
}

// rapidAPIRequest handles requests to the RapidAPI YouTube converter
func rapidAPIRequest(youtubeID string) (map[string]interface{}, error) {
	apiKey := os.Getenv("RAPIDAPI_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("RAPIDAPI_KEY environment variable not set")
	}
	apiURL := "https://youtube-mp36.p.rapidapi.com/dl"
	query := url.Values{}
	query.Add("id", youtubeID)

	req, err := http.NewRequest("GET", apiURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-RapidAPI-Key", apiKey)
	req.Header.Set("X-RapidAPI-Host", "youtube-mp36.p.rapidapi.com")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func readYoutubeResultRapidLinkMD5Hash(link, rapidAPIUsername string) (*http.Response, error) {
	// Compute MD5 hash of RapidAPI username
	hash := fmt.Sprintf("%x", md5.Sum([]byte(rapidAPIUsername)))

	// Create a new HTTP request
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	// Add custom X-RUN header with MD5 hash
	req.Header.Set("X-RUN", hash)

	// Create HTTP client and execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		// print the body
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error: youtube result link received status code %d %s", resp.StatusCode, string(body))
	}
	return resp, nil
}

// HandleDownloadYoutubeTask processes YouTube download tasks using RapidAPI
func HandleDownloadYoutubeTask(ctx context.Context, t *asynq.Task, db *gorm.DB, transcriber services.LLMNoteProcessor, awsService services.AWSServiceProvider) error {
	var payload TryOnGenerationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	fmt.Printf("[Note: %v] Youtube Processing\n", payload.TryOnID)

	// Retrieve note from database
	var note models.Note
	res := db.First(&note, payload.TryOnID)
	if res.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Youtube Error on retrieving note for processing %v", payload.TryOnID))
		return res.Error
	}

	if note.YoutubeUrl == nil {
		sentry.CaptureException(fmt.Errorf("[Note: %v] Youtube URL is nil", payload.TryOnID))
		return fmt.Errorf("[Note: %v] Youtube URL is nil, cannot proceed", payload.TryOnID)
	}

	// Extract YouTube ID
	youtubeID, err := ExtractYoutubeID(*note.YoutubeUrl)
	if err != nil {
		saveNoteProcessingFail(db, note, "Invalid YouTube URL format, Please create a new note with a valid YouTube URL", false)
		return nil
	}

	// Initial API request
	response, err := rapidAPIRequest(youtubeID)
	if err != nil {
		sentry.CaptureException(err)
		return err
	}
	deadline := time.Now().Add(120 * time.Second)

	// Poll until completion
	for {
		status, ok := response["status"].(string)
		if !ok {
			fmt.Printf("[Note: %v] Youtube Error on getting status from response: %v\n", payload.TryOnID, response)
			err := fmt.Errorf("[Note: %v] Youtube Error on getting status from response: %v", payload.TryOnID, response)
			sentry.CaptureException(err)
			return err
		}

		if status == "fail" {
			msg, _ := response["msg"].(string)
			if strings.Contains(msg, "Long audio of more than 2 hr") {
				saveNoteProcessingFail(db, note, "Long audio of more than 2 hr duration are not allowed", false)
				return nil
			}
			fmt.Printf("[Note: %v] Youtube Error on getting status from response: %v %s\n", payload.TryOnID, response, msg)
			err := fmt.Errorf("[Note: %v] Youtube Error on getting status from response: %v %s", payload.TryOnID, response, msg)
			sentry.CaptureException(err)
			return err
		}

		if status == "ok" {
			fmt.Printf("[Note: %v] Youtube Download finished successfully %v \n", payload.TryOnID, response)
			break
		}
		// no more than minute
		if time.Now().After(deadline) {
			err := fmt.Errorf("[Note: %v] Youtube polling timeout after 60 seconds, last status: %v", payload.TryOnID, response)
			fmt.Printf("[Note: %v] Youtube polling timeout after 60 seconds: last status: %v \n", payload.TryOnID, response)
			sentry.CaptureException(err)
			return err
		}
		progress, ok := response["progress"].(float64)
		if ok {
			fmt.Printf("[Note: %v] Youtube Download progress: %.2f%%\n", payload.TryOnID, progress)
		}

		// Wait before polling again
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			// Poll again
			response, err = rapidAPIRequest(youtubeID)
			if err != nil {
				sentry.CaptureException(err)
				return err
			}
		}
	}
	duration, ok := response["duration"].(float64)
	if !ok {
		fmt.Printf("[Note: %v] Youtube Error on getting duration from response: %v\n", payload.TryOnID, response)
		err := fmt.Errorf("[Note: %v] Youtube Error on getting duration from response: %v", payload.TryOnID, response)
		sentry.CaptureException(err)
	} else {
		note.TotalDuration = &duration
	}
	title, ok := response["title"].(string)
	if !ok {
		fmt.Printf("[Note: %v] Youtube Error on getting title from response: %v\n", payload.TryOnID, response)
		err := fmt.Errorf("[Note: %v] Youtube Error on getting title from response: %v", payload.TryOnID, response)
		sentry.CaptureException(err)
	} else {

		note.Name = title
	}
	if err := db.Omit("alert_when_processed").Save(&note).Error; err != nil {
		fmt.Printf("[Note: %v] Youtube Error on initial saving note %v\n", payload.TryOnID, err)
		sentry.CaptureException(err)
		return err
	}
	// Download the MP3 file
	link, ok := response["link"].(string)
	if !ok {
		err := fmt.Errorf("invalid link format in response")
		sentry.CaptureException(err)
		return err
	}
	resp, err := readYoutubeResultRapidLinkMD5Hash(link, os.Getenv("RAPIDAPI_USERNAME"))
	if err != nil {
		sentry.CaptureException(err)
		return err
	}
	defer resp.Body.Close()

	fileName := fmt.Sprintf("note-%v.mp3", note.ID)
	file, err := os.Create(fileName)
	if err != nil {
		sentry.CaptureException(err)
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		sentry.CaptureException(err)
		return err
	}

	// Upload to R2
	bucketName := services.GetEnv("R2_BUCKET_NAME", "")
	safeFileName := fmt.Sprintf("notes/%s", fileName)

	uploadUrl, presignErr := awsService.PresignLink(context.Background(), bucketName, safeFileName)
	if presignErr != nil {
		fmt.Printf("[Note: %v] Youtube Unable to create presign link for %s: %v\n", note.ID, note.Name, presignErr)
		sentry.CaptureException(presignErr)
		return presignErr
	}

	fileBytes, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Printf("[Note: %v] Youtube Error on reading file %s: %v\n", payload.TryOnID, fileName, err)
		sentry.CaptureException(err)
		return err
	}

	respBody, statusCode, err := awsService.UploadToPresignedURL(context.Background(), bucketName, uploadUrl, fileBytes)
	fmt.Printf("[Note: %v] Youtube %s R2 Upload file size %v, url %s, response body: %s, status code: %d\n", payload.TryOnID, *note.YoutubeUrl, len(fileBytes), uploadUrl, respBody, statusCode)
	if err != nil || statusCode != 200 {
		fmt.Printf("[Note: %v] Youtube Error on uploading file %s: %v\n", payload.TryOnID, fileName, err)
		sentry.CaptureException(err)
		return err
	}

	// Clean up
	os.Remove(fileName)
	note.FileUrl = &safeFileName
	note.YoutubeId = &youtubeID

	if err := db.Omit("alert_when_processed").Save(&note).Error; err != nil {
		fmt.Printf("[Note: %v] Youtube Error on saving note %v\n", payload.TryOnID, err)
		sentry.CaptureException(err)
		return err
	}

	fmt.Printf("[Note: %v] Youtube Download finished successfully\n", payload.TryOnID)

	// Enqueue transcription task
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: os.Getenv("ASYNC_BROKER_ADDRESS")})
	if asynqClient == nil {
		err := fmt.Errorf("failed to create asynq client")
		fmt.Printf("[Note: %v] Youtube Error on creating asynq client %v\n", payload.TryOnID, err)
		sentry.CaptureException(err)
		return err
	}
	defer asynqClient.Close()

	noteTranscribeTask, err := NewTranscribeNoteTask(note.ID)
	if err != nil {
		fmt.Printf("[Note: %v] Youtube error on creating transcribe task %v\n", payload.TryOnID, err)
		sentry.CaptureException(err)
		return err
	}

	taskInfo, err := asynqClient.Enqueue(noteTranscribeTask, asynq.MaxRetry(3), asynq.ProcessIn(1*time.Second), asynq.Queue("transcribe"))
	if err != nil {
		fmt.Printf("[Note: %v] Youtube error on enqueuing transcribe task %v\n", payload.TryOnID, err)
		sentry.CaptureException(err)
		return err
	}
	fmt.Printf("[Note: %v] Youtube transcribe task enqueued: %s\n", payload.TryOnID, taskInfo.ID)

	return nil
}

// func HandleDownloadYoutubeTask(ctx context.Context, t *asynq.Task, db *gorm.DB, transcriber services.LLMNoteProcessor, awsService services.AWSServiceProvider) error {
// 	installState, err := ytdlp.Install(context.TODO(), nil)
// 	fmt.Println("Ytdlp exec:", installState.Executable, " cache:", installState.FromCache, " Version:", installState.Version)
// 	if err != nil {
// 		sentry.CaptureException(fmt.Errorf("[QUEUE] Youtube Error on installing ytdlp: %v", err))
// 	}

// 	var payload NoteProcessingPayload
// 	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
// 		return err
// 	}
// 	fmt.Printf("[Note: %v] Youtube Processing\n", payload.NoteID)
// 	var note models.Note
// 	res := db.First(&note, payload.NoteID)
// 	if res.Error != nil {
// 		sentry.CaptureException(fmt.Errorf("[QUEUE] Youtube Error on retrieving note for processing %v", payload.NoteID))
// 		return res.Error
// 	}
// 	if note.YoutubeUrl == nil {
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Youtube URL is nil", payload.NoteID))
// 		return fmt.Errorf("[Note: %v] Youtube URL is nil, cannot proceed", payload.NoteID)
// 	}

// 	youtubeUrl := *note.YoutubeUrl
// 	fileName := fmt.Sprintf("note-%v.m4a", note.ID)
// 	fmt.Printf("[Note: %v] Youtube URL: %s to %s\n", payload.NoteID, youtubeUrl, fileName)
// 	os.Remove(fileName)

// 	dl := ytdlp.New().
// 		// PrintJSON().
// 		NoProgress().
// 		SleepInterval(3).
// 		FormatSort("res,ext:mp4:m4a").
// 		ExtractAudio().
// 		AudioQuality("0").
// 		NoPlaylist().
// 		NoOverwrites().
// 		Continue().
// 		ProgressFunc(1000*time.Millisecond, func(prog ytdlp.ProgressUpdate) {
// 			fmt.Printf( //nolint:forbidigo
// 				"%s @ %s [eta: %s] :: %s\n",
// 				prog.Status,
// 				prog.PercentString(),
// 				prog.ETA(),
// 				prog.Filename,
// 			)
// 		}).
// 		Output(fmt.Sprintf("note-%v.%%(ext)s", note.ID))

// 	r, err := dl.Run(context.TODO(), youtubeUrl)
// 	if err != nil {
// 		fmt.Println(r.Stderr)

// 		sentry.CaptureException(err)
// 		// panic(err)
// 	}

// 	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
// 	// todo clean and map the same file name as in FE UI otherwise **FAIL**
// 	safeFileName := fmt.Sprintf("notes/%s", fileName)

// 	// uploadUrl, presignErr = controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
// 	// fileName := strings.ReplaceAll(*req.FileName, " ", "")
// 	// fileName = strings.ReplaceAll(*req.FileName, "-", "")
// 	// safeFileName := fmt.Sprintf("notes/%s", fileName)
// 	uploadUrl, presignErr := awsService.PresignLink(context.Background(), bucketName, safeFileName)
// 	if presignErr != nil {
// 		fmt.Printf("[Note: %v] Youtube Unable to create presign link from youtube for %s!, %s\n", note.ID, note.Name, presignErr)
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Unable to create presign link from youtube for %s!, %s", note.ID, note.Name, presignErr))
// 		return presignErr
// 	}
// 	// parse file from Output of ytdlp file path in fmt.Sprintf("note-%v.%%(ext)s", note.ID)
// 	fileContent := fileName
// 	fileBytes, err := os.ReadFile(fileContent)
// 	if err != nil {
// 		fmt.Printf("[Note: %v] Youtube Error on reading file %s: %v\n", payload.NoteID, fileContent, err)
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on reading file %s: %v", payload.NoteID, fileContent, err))
// 		return err
// 	}
// 	respBody, statusCode, err := awsService.UploadToPresignedURL(context.Background(), bucketName, uploadUrl, fileBytes)
// 	fmt.Printf("[Note: %v] Youtube %s R2 Upload response body: %s, status code: %d\n", payload.NoteID, youtubeUrl, respBody, statusCode)
// 	if err != nil || statusCode != 204 {
// 		fmt.Printf("[Note: %v] Youtube Error on uploading file %s: %v\n", payload.NoteID, fileContent, err)
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on uploading file %s: %v", payload.NoteID, fileContent, err))
// 		return err
// 	}
// 	fmt.Printf("[Note: %v] Success.. Removing local one\n", payload.NoteID)
// 	os.Remove(fileName)
// 	note.FileUrl = &safeFileName

// 	if err := db.Omit("alert_when_processed").Save(&note).Error; err != nil {
// 		fmt.Printf("[Note: %v] Youtube Error on saving note %v", payload.NoteID, err)
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on saving note %v", payload.NoteID, err))
// 	}
// 	fmt.Printf("[Note: %v] Youtube Download finished succesfully..\n", payload.NoteID)

// 	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: os.Getenv("ASYNC_BROKER_ADDRESS")})
// 	if asynqClient == nil {
// 		fmt.Printf("[Note: %v] Youtube Error on creating asynq client %v\n", payload.NoteID, err)
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on creating asynq client %v", payload.NoteID, err))
// 		return fmt.Errorf("[Note: %v] Error on creating asynq client %v", payload.NoteID, err)
// 	}
// 	noteTranscribeTask, err := NewTranscribeNoteTask(note.ID)
// 	if err != nil {
// 		fmt.Printf("[Note: %v] Youtube error on creating transcribe task %v\n", payload.NoteID, err)
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on creating transcribe task %v", payload.NoteID, err))
// 		return err
// 	}
// 	taskInfo, err := asynqClient.Enqueue(noteTranscribeTask, asynq.MaxRetry(3), asynq.ProcessIn(5*time.Second))
// 	if err != nil {
// 		fmt.Printf("[Note: %v] Youtube error on enqueuing transcribe task %v\n", payload.NoteID, err)
// 		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on enqueuing transcribe task %v", payload.NoteID, err))
// 		return err
// 	}
// 	fmt.Printf("[Note: %v] Youtube transcribe task enqueued: %s\n", payload.NoteID, taskInfo.ID)
// 	return nil
// }

func cleanAIResponseText(text string) string {
	// Remove any leading or trailing whitespace
	cleanContent := strings.ReplaceAll(text, "```json", "")
	cleanContent = strings.TrimSuffix(cleanContent, "```")
	// cleanContent = strings.ReplaceAll(text, "\\n", "\n")

	// Replace multiple spaces with a single space

	// Remove any newlines or carriage returns
	// Remove any leading or trailing whitespace again after replacements
	return cleanContent
}
func cleanAIResponseSeparateFieldsText(text string) string {
	// Remove any leading or trailing whitespace
	// cleanContent := strings.ReplaceAll(text, "```json", "")
	// cleanContent = strings.TrimSuffix(text, "```")
	cleanText := strings.ReplaceAll(text, "\\n", "\n")

	// Replace multiple spaces with a single space

	// Remove any newlines or carriage returns
	// Remove any leading or trailing whitespace again after replacements
	return cleanText
}

// InitialProcessNote handles the task of processing a note with the LLM
func HandleInitialProcessNoteTask(
	ctx context.Context, t *asynq.Task, db *gorm.DB, transcriber services.LLMNoteProcessor,
	awsService services.AWSServiceProvider, fbApp *firebase.App) error {
	google_key := os.Getenv("GOOGLE_API_KEY")
	if google_key == "" {
		sentry.CaptureException(fmt.Errorf("[QUEUE] %s Google API key is not set", string(t.Payload())))
		return fmt.Errorf("[QUEUE] %s Google API key is not set", string(t.Payload()))
	}
	var payload TryOnGenerationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	fmt.Printf("[Note: %v] Start Processing\n", payload.TryOnID)
	var note models.Note
	res := db.Joins("Company").First(&note, payload.TryOnID)
	if res.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Error on retrieving note for processing %v", payload.TryOnID))
		return res.Error
	}
	if note.NoteType == nil {
		saveNoteProcessingFail(db, note, "Failed to identify note type, please try to create new note", false)
		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on getting note type", payload.TryOnID))
		return fmt.Errorf("[Note: %v] Error on getting note type", payload.TryOnID)
	}
	fileBytes, fileName, err := getFileForNote(awsService, note)
	if *note.NoteType != "youtube" && note.FileUrl != nil && err != nil {
		saveNoteProcessingFail(db, note, "Failed to read note file, please try to create new note", false)
		sentry.CaptureException(fmt.Errorf("[Note: %v] File path exists, but error on getting file %s: %v", payload.TryOnID, *note.FileUrl, err))
		return err
	}
	fmt.Printf("[Note: %v] Downloaded file size: %d bytes\n", payload.TryOnID, len(fileBytes))
	var noteLLMResponseText string
	var noteLLMResponse *services.LLMResponse

	fmt.Printf("[Note: %v] Type: %s\n", note.ID, *note.NoteType)

	fmt.Printf("[Note: %v] Transcribing..\n", payload.TryOnID)
	model := services.Flash25
	modelString := model.String()
	if note.Company.EnforcedLLMModel != nil {
		model = services.LLMModelName(*note.Company.EnforcedLLMModel)
		modelString = model.String()
		fmt.Printf("[Note: %v] [ENFORCE MODEL] Using enforced model: %s\n", payload.TryOnID, model.String())
	}

	fmt.Printf("[Note: %v] Model: %s\n", payload.TryOnID, modelString)
	// }

	if note.NoteType == nil {
		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on getting note type", payload.TryOnID))
		return fmt.Errorf("[Note: %v] Error on getting note type", payload.TryOnID)
	} else if *note.NoteType == "text" {
		if note.Transcript == nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] Text note Error on getting note transcript to analyze note text", payload.TryOnID))
			return fmt.Errorf("[Note: %v] Text note Error on getting note transcript to analyze note text", payload.TryOnID)
		}
		noteLLMResponse, err = transcriber.TextParse(*note.Transcript, model)
		if err != nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] Error on transcribing audio %s: %v", payload.TryOnID, *note.FileUrl, err))
			return err
		}
		if noteLLMResponse == nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing audio %s: %v", payload.TryOnID, *note.FileUrl, err))
			return fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing %s: %v", payload.TryOnID, *note.FileUrl, err)
		}
		noteLLMResponseText = noteLLMResponse.Response
	} else if *note.NoteType == "test" {
		if note.Transcript == nil || fileBytes == nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] [Test] Note doesn't have neither transcript nor image to analyze", payload.TryOnID))
			return fmt.Errorf("[Note: %v] [Test] Note doesn't have neither transcript nor image to analyze", payload.TryOnID)
		}
		tempImagePaths := []string{}
		if fileBytes != nil {

			tempImagePaths, err = services.ExtractZipImages(fileBytes, fileName, payload.TryOnID)
			if err != nil {
				sentry.CaptureException(fmt.Errorf("[Note: %v] Error extracting images from zip %s: %v", payload.TryOnID, *note.FileUrl, err))
				return err
			}
			fmt.Printf("[Note: %v] Extracted zip image paths %v:", payload.TryOnID, tempImagePaths)
		}
		defer func() {
			for _, path := range tempImagePaths {
				os.Remove(path)
			}
		}()
		// Process first image (or modify to handle multiple images)
		noteLLMResponse, err = transcriber.ExamParse(note.Transcript, tempImagePaths, model)
		noteLLMResponseText = noteLLMResponse.Response

	} else if *note.NoteType == "youtube" {
		// create temporary file with filename and given file bytes then return file path
		filePath, err := services.CreateTempFile(fileBytes, fileName)
		if err != nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] Error on creating temp file %s: %v", payload.TryOnID, fileName, err))
			return err
		}
		defer os.Remove(filePath)
		language := "auto"
		if note.LanguageCode != nil {
			language = *note.LanguageCode
		}
		noteLLMResponse, err = transcriber.Transcribe([]string{filePath}, model, language)

		if err != nil {
			if strings.Contains(err.Error(), "content violation") {
				saveNoteProcessingFail(db, note, "Sorry, it seems that this Youtube audio contains violated content that we cannot process.", false)
				sentry.CaptureException(fmt.Errorf("[Note: %v] Content violation on transcribing youtube audio %s: %v", payload.TryOnID, *note.FileUrl, err))
				return nil
			}
			fmt.Printf("[Note: %v] Error on transcribing youtube audio %v: %v\n", payload.TryOnID, *note.FileUrl, err)
			saveNoteProcessingFail(db, note, "Sorry, we failed to analyze this Youtube audio, please try to create new note or contact support", true)
			sentry.CaptureException(fmt.Errorf("[Note: %v] Error on transcribing youtube audio  %s: %v", payload.TryOnID, *note.FileUrl, err))
			return err
		}
		if noteLLMResponse == nil {
			fmt.Printf("[Note: %v] Error on transcribing youtube audio %v: %v\n", payload.TryOnID, *note.FileUrl, err)
			saveNoteProcessingFail(db, note, "Sorry, we failed to analyze this Youtube audio, please try to create new note or contact support", true)
			sentry.CaptureException(fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing  %s: %v", payload.TryOnID, *note.FileUrl, err))
			return fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing %s: %v", payload.TryOnID, *note.FileUrl, err)
		}
		if noteLLMResponse == nil {
			sentry.CaptureException(fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing audio %s: %v", payload.TryOnID, *note.FileUrl, err))
			return fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing %s: %v", payload.TryOnID, *note.FileUrl, err)
		}
		noteLLMResponseText = noteLLMResponse.Response
	} else {
		fmt.Printf("[Note: %v] Note type %s\n", payload.TryOnID, *note.NoteType)
		tempDocumentPaths, userTranscript, err := services.ExtractZipMaterialFiles(fileBytes, fileName, payload.TryOnID)
		for _, path := range tempDocumentPaths {
			// clean defer file after processing
			defer func(path string) {
				if err := os.Remove(path); err != nil {
					fmt.Printf("[Note: %v] Error removing temporary file %s: %v\n", payload.TryOnID, path, err)
				} else {
					fmt.Printf("[Note: %v] Successfully removed temporary file %s\n", payload.TryOnID, path)
				}
			}(path)

		}
		if err != nil {
			saveNoteProcessingFail(db, note, "Failed to read your note files, please try to create new note", true)
			sentry.CaptureException(fmt.Errorf("[Note: %v] Error extracting documents from zip %s: %v", payload.TryOnID, *note.FileUrl, err))
			return err
		}
		fmt.Printf("[Note: %v] Extracted zip document paths %v:", payload.TryOnID, tempDocumentPaths)
		note.NoteType = services.StrPointer(services.DetermineNoteType(tempDocumentPaths))
		if db.Omit("alert_when_processed").Save(&note).Error != nil {
			fmt.Printf("[Note: %v] Error on saving note mid type detect %v", payload.TryOnID, err)
			saveNoteProcessingFail(db, note, "Failed to determine note type, please try to create new note", true)
			sentry.CaptureException(fmt.Errorf("[Note: %v] Error on saving note mid type detect %v", payload.TryOnID, err))
		}
		defer func() {
			for _, path := range tempDocumentPaths {
				os.Remove(path)
			}
		}()

		if len(tempDocumentPaths) > 0 || userTranscript != "" {
			language := "auto"
			if note.LanguageCode != nil {
				language = *note.LanguageCode
			}
			noteLLMResponse, err = transcriber.DocumentsParse(tempDocumentPaths, userTranscript, services.Flash25, language)
			if err != nil {
				fmt.Printf("[Note: %v] Error on transcribing documents %v: %v\n", payload.TryOnID, tempDocumentPaths, err)
				if strings.Contains(err.Error(), "content violation") {
					saveNoteProcessingFail(db, note, "Sorry, it seems that this note contains violated content that we cannot process.", false)
					sentry.CaptureException(fmt.Errorf("[Note: %v] Content violation on transcribing note %s: %v", payload.TryOnID, *note.FileUrl, err))
					return nil
				}
				saveNoteProcessingFail(db, note, "Failed to transribe your note, please try to create new note", true)
				sentry.CaptureException(fmt.Errorf("[Note: %v] Error on transcribing documents %s: %v", payload.TryOnID, *note.FileUrl, err))
				return err
			}
			if noteLLMResponse == nil {
				fmt.Printf("[Note: %v] Response is nil but no error provided on transcribing %v: %v\n", payload.TryOnID, tempDocumentPaths, err)
				saveNoteProcessingFail(db, note, "Failed to transribe your note, please try to create new note", true)
				sentry.CaptureException(fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing documents %v: %v", payload.TryOnID, tempDocumentPaths, err))
				return fmt.Errorf("[Note: %v] Response is nil but no error provided on transcribing documents %s: %v", payload.TryOnID, *note.FileUrl, err)
			}
			noteLLMResponseText = noteLLMResponse.Response

		} else {
			saveNoteProcessingFail(db, note, "It seems that you uploaded empty note, please try to create new note", false)
			return nil
		}
	}
	cleanNoteContent := cleanAIResponseText(noteLLMResponseText)
	// content is json read attributes
	fmt.Printf("[Note: %v] LLM Processed: %q, IT: %d, OT: %d, TT: %d, TOT: %d, Thoughts: %s..\n", payload.TryOnID, cleanNoteContent, noteLLMResponse.InputTokenCount, noteLLMResponse.OutputTokenCount, noteLLMResponse.ThoughtsTokenCount, noteLLMResponse.TotalTokenCount, noteLLMResponse.Thoughts)
	var parsedNoteData services.NoteTranscribeResponse
	if err := json.Unmarshal([]byte(cleanNoteContent), &parsedNoteData); err != nil {
		fmt.Println(err)
		fmt.Printf("[Note: %v] Error on parsing Gemini %s AI json %s", payload.TryOnID, model.String(), noteLLMResponseText)
		note.FailedToProcessLLMNoteResponse = cleanNoteContent
		saveNoteProcessingFail(db, note, "Failed to transcribe note content, please try again later", true)
		sentry.CaptureException(fmt.Errorf("[Note: %v] Error on parsing Gemini %s AI json %s", payload.TryOnID, services.Pro25, noteLLMResponseText))
		return err
	}
	parsedNoteData.Transcription = cleanAIResponseSeparateFieldsText(parsedNoteData.Transcription)
	parsedNoteData.MD_Summary = cleanAIResponseSeparateFieldsText(parsedNoteData.MD_Summary)

	// convert quiz to json string
	// quizJSONBytes, err := json.Marshal(parsedNoteData.QuizJSON)
	// quizJSONString :=
	note.Transcript = &parsedNoteData.Transcription
	note.ConspectAIMD5 = &parsedNoteData.MD_Summary
	// note.QuizJSON = &quizJSONString
	note.Name = parsedNoteData.Name
	note.Language = parsedNoteData.Language
	note.Status = "transcribed"
	note.QuizStatus = "ready_to_generate"
	note.TotalTokenCount = &noteLLMResponse.TotalTokenCount
	note.InputTokenCount = &noteLLMResponse.InputTokenCount
	note.ThoughtsTokenCount = &noteLLMResponse.ThoughtsTokenCount
	note.OuputTokenCount = &noteLLMResponse.OutputTokenCount
	note.Thoughts = &noteLLMResponse.Thoughts
	note.LLMModel = &modelString
	note.ProcessingErrorMessage = nil
	tx := db.Omit("alert_when_processed").Save(&note)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Error on saving note %v", payload.TryOnID))
		return tx.Error
	}
	fmt.Printf("[Note: %v] Transcribing finished succesfully..", payload.TryOnID)
	if note.AlertWhenProcessed {
		fmt.Printf("[Note: %v] Sending notification to user %v\n", payload.TryOnID, note.OwnerID)
		services.SendNotification(fbApp, db, note.OwnerID, "Note Transcription Completed", fmt.Sprintf("Your note %s has been transcribed", note.Name), map[string]string{"note_id": fmt.Sprintf("%d", note.ID), "type": "note_transcribed"})
	} else {
		fmt.Printf("[Note: %v] AlertWhenProcessed is false, not sending notification to user %v\n", payload.TryOnID, note.OwnerID)
	}
	// Save result back to database
	return nil
}

// Generates study material
func HandleGeneratStudyForNoteTask(ctx context.Context, t *asynq.Task, db *gorm.DB, transcriber services.LLMNoteProcessor, awsService services.AWSServiceProvider) error {
	var payload TryOnGenerationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	fmt.Printf("[Material Note: %v] Start Processing\n", payload.TryOnID)
	// Fetch note from database

	var note models.Note
	res := db.First(&note, payload.TryOnID)
	if res.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Error on retrieving note for processing %v", payload.TryOnID))
		return res.Error
	}
	if note.QuizStatus == "generated" {
		fmt.Printf("[Material Note: %v] Quiz already generated\n", payload.TryOnID)
		return nil
	}
	if note.QuizStatus != "in_progress" && note.QuizStatus != "failed" {
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Quiz is not ready for generation: %s %s", payload.TryOnID, note.Status, *note.NoteType))
		return fmt.Errorf("[Material Note: %v] Quiz is not ready for generation: %s %s", payload.TryOnID, note.Status, *note.NoteType)
	}

	model := services.Flash25
	if *note.NoteType == "test" {
		model = services.Pro25
	}
	modelString := model.String()
	fmt.Printf("[Material Note: %v] Model: %s\n", payload.TryOnID, modelString)
	noteLLMResponse, err := transcriber.GenerateQuizAndFlashCards(*note.Transcript, *note.NoteType == "test", model, note.Language)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Error on generating study material %s: %v", payload.TryOnID, *note.FileUrl, err))
		saveFailQuizNote(db, note, "Failed to generate study material, please try again", true)
		return err
	}
	noteLLMResponseText := noteLLMResponse.Response
	if noteLLMResponseText == "" {
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Response is nil but no error provided on generating study material %s: %v", payload.TryOnID, *note.FileUrl, err))
		saveFailQuizNote(db, note, "Failed to generate study material, please try again", true)
		return fmt.Errorf("[Material Note: %v] Response is nil but no error provided on generating study material %s: %v", payload.TryOnID, *note.FileUrl, err)
	}

	cleanNoteContent := cleanAIResponseText(noteLLMResponseText)
	// content is json read attributes
	fmt.Printf("[Material Note: %v] LLM Processed: %s, IT: %d, OT: %d, TT: %d, TOT: %d Thoughts: %s ..\n", payload.TryOnID, cleanNoteContent, noteLLMResponse.InputTokenCount, noteLLMResponse.OutputTokenCount, noteLLMResponse.ThoughtsTokenCount, noteLLMResponse.TotalTokenCount, noteLLMResponse.Thoughts)
	var parsedNoteData services.NoteQuizResponse
	if err := json.Unmarshal([]byte(cleanNoteContent), &parsedNoteData); err != nil {
		fmt.Printf("[Material Note: %v] Error on parsing Gemini %s AI json %s", payload.TryOnID, modelString, noteLLMResponseText)
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Error on parsing Gemini %s AI json %s", payload.TryOnID, modelString, noteLLMResponseText))
		note.FailedToProcessLLMNoteResponse = cleanNoteContent
		saveFailQuizNote(db, note, "Failed to generate study material, please try again", true)
		return err
	}
	easy_len := len(parsedNoteData.EasyQuestions)
	hard_len := len(parsedNoteData.HardQuestions)
	bonus_len := len(parsedNoteData.HardQuestions)
	flashcards_len := len(parsedNoteData.FlashCards)
	if easy_len == 0 || hard_len == 0 || flashcards_len == 0 {
		fmt.Printf("[Material Note: %v] Easy: %v, Hard:%v, Flashcards: %v, Some material is empty for model %s AI json %s", payload.TryOnID, easy_len, hard_len, flashcards_len, modelString, noteLLMResponseText)
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Easy: %v, Hard:%v, Flashcards: %v, Some material is empty for model %s AI json %s", payload.TryOnID, easy_len, hard_len, flashcards_len, modelString, noteLLMResponseText))
		saveFailQuizNote(db, note, "No questions or flashcards generated, are you sure that your note has enough content?", true)
		return err
	}
	if easy_len < 10 || hard_len < 10 || bonus_len < 10 || flashcards_len < 10 {
		fmt.Printf("[Material Note: %v] Less material detected! Easy: %v, Hard:%v, Bonus: %v, Flashcards: %v, for %s AI json %s", payload.TryOnID, easy_len, hard_len, bonus_len, flashcards_len, modelString, noteLLMResponseText)
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Less material detected! Easy: %v, Hard:%v, Flashcards: %v, for %s AI json %s", payload.TryOnID, easy_len, hard_len, flashcards_len, modelString, noteLLMResponseText))
		// saveFailQuizNote(db, note)

	}
	// convert quiz to json string
	quizJSONBytes, err := json.Marshal(parsedNoteData)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Error on dumping quiz as string from Gemini %s AI json %s", payload.TryOnID, modelString, noteLLMResponseText))
		saveFailQuizNote(db, note, "Could not save quiz, please try to create new note", true)
		return err
	}
	flashcardsJSONBytes, flashCardErr := json.Marshal(parsedNoteData.FlashCards)
	if flashCardErr != nil {
		sentry.CaptureException(fmt.Errorf("[Material Note: %v] Error on dumping flashcards as string from Gemini %s AI json %s", payload.TryOnID, modelString, noteLLMResponseText))
		saveFailQuizNote(db, note, "Could not save flashcards, please try to create new note", true)
		return flashCardErr
	}
	quizJSONString := string(quizJSONBytes)

	note.QuizJSON = &quizJSONString
	flashcardsJSONString := string(flashcardsJSONBytes)
	note.FlashcardsJSON = &flashcardsJSONString
	note.QuizStatus = "generated"
	note.QuestionGeneratedCount = note.QuestionGeneratedCount + 1

	note.QuizTotalTokenCount = &noteLLMResponse.TotalTokenCount
	note.QuizInputTokenCount = &noteLLMResponse.InputTokenCount
	note.QuizThoughtsTokenCount = &noteLLMResponse.ThoughtsTokenCount
	note.QuizOuputTokenCount = &noteLLMResponse.OutputTokenCount
	note.Thoughts = &noteLLMResponse.Thoughts
	note.QuizLLMModel = &modelString

	// save question from llm
	var questions []models.Question
	for _, question := range parsedNoteData.EasyQuestions {
		question.Question = strings.TrimSpace(question.Question)
		var options []string
		for _, option := range question.Options {
			options = append(options, strings.TrimSpace(option))

		}
		cleanQuestion := cleanAIResponseSeparateFieldsText(question.Question)
		cleanExplanation := cleanAIResponseSeparateFieldsText(question.Explanation)
		questions = append(questions, models.Question{
			NoteID:          note.ID,
			Type:            "single_choice",
			ComplexityLevel: "easy",
			Explanation:     cleanExplanation,
			QuestionText:    cleanQuestion,
			Answer:          fmt.Sprint(question.Answer),
			Options:         options,
		})
	}

	for _, question := range parsedNoteData.HardQuestions {
		question.Question = strings.TrimSpace(question.Question)
		var options []string
		for _, option := range question.Options {
			options = append(options, strings.TrimSpace(option))

		}
		questions = append(questions, models.Question{
			NoteID:          note.ID,
			Type:            "single_choice",
			ComplexityLevel: "hard",
			Explanation:     question.Explanation,
			QuestionText:    question.Question,
			Answer:          fmt.Sprint(question.Answer),
			Options:         pq.StringArray(options),
		})
	}

	for _, question := range parsedNoteData.BonusQuestions {
		question.Question = strings.TrimSpace(question.Question)
		var options []string
		for _, option := range question.Options {
			options = append(options, strings.TrimSpace(option))

		}
		questions = append(questions, models.Question{
			NoteID:          note.ID,
			Type:            "single_choice",
			ComplexityLevel: "bonus",
			Explanation:     question.Explanation,
			QuestionText:    question.Question,
			Answer:          fmt.Sprint(question.Answer),
			Options:         pq.StringArray(options),
		})
	}

	db.CreateInBatches(questions, 1000)
	tx := db.Omit("alert_when_processed").Save(&note)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Material Note %v] Error on saving note at the end", payload.TryOnID))
		return tx.Error
	}
	fmt.Printf("[Material Note: %v] Transcribing finished succesfully..", payload.TryOnID)

	// Save result back to database
	return nil
}

func saveFailQuizNote(db *gorm.DB, note models.Note, message string, shouldRetry bool) error {
	// note.QuizStatus = "failed"
	note.QuizProcessRetryTimes = note.QuizProcessRetryTimes + 1
	note.ProcessingQuizErrorMessage = services.StrPointer(message)
	if !shouldRetry || note.QuizProcessRetryTimes >= 3 {

		note.QuizStatus = "failed"
	}

	tx := db.Omit("alert_when_processed").Save(&note)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Fail Note %v] Error on saving note quiz for failed status", note.ID))
		return tx.Error
	}
	return nil
}

func saveNoteProcessingFail(db *gorm.DB, note models.Note, msg string, shouldRetry bool) error {
	note.ProcessRetryTimes = note.ProcessRetryTimes + 1
	note.ProcessingErrorMessage = &msg
	if !shouldRetry || note.ProcessRetryTimes >= 3 {

		note.Status = "failed"
	}
	tx := db.Omit("alert_when_processed").Save(&note)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Fail Note %v] Error on saving note for failed status", note.ID))
		return tx.Error
	}
	return nil
}

// ScheduledQuizAlertTask sends a random quiz question to users via notification
func ScheduledQuizAlertTask(ctx context.Context, t *asynq.Task, db *gorm.DB, fbApp *firebase.App) error {

	fmt.Printf("[Quiz Alert] Processing for all users\n")

	// Get all active users who have quiz alerts enabled
	var users []models.UserAccount
	result := db.Where("banned = ?", false).Find(&users)
	if result.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Quiz Alert] Error fetching users: %v", result.Error))
		return result.Error
	}

	fmt.Printf("[Quiz Alert] Found %d users to send notifications\n", len(users))

	// Process each user
	for _, user := range users {
		err := sendQuizAlertToUser(ctx, db, fbApp, user.ID)
		if err != nil {
			fmt.Printf("[Quiz Alert] Failed to send to user %d: %v\n", user.ID, err)
			sentry.CaptureException(fmt.Errorf("[Quiz Alert] Failed to send to user %d: %v", user.ID, err))
			continue
		}
		fmt.Printf("[Quiz Alert] Successfully sent quiz alert to user %d\n", user.ID)
		time.Sleep(1 * time.Second) // To avoid hitting rate limits
	}

	return nil
}

func sendQuizAlertToUser(ctx context.Context, db *gorm.DB, fbApp *firebase.App, userID uint) error {
	// Get user's notes with quiz alerts enabled and generated quizzes
	var notes []models.Note
	result := db.Where("owner_id = ? AND quiz_alerts_enabled = ? AND quiz_status = ? AND deleted = ?",
		userID, true, "generated", false).Find(&notes)

	if result.Error != nil {
		return fmt.Errorf("error fetching user notes: %v", result.Error)
	}

	if len(notes) == 0 {
		fmt.Printf("[Quiz Alert] No eligible notes found for user %d\n", userID)
		return nil
	}

	// Pick a random note
	randomNote := notes[time.Now().Unix()%int64(len(notes))]

	// Get a random question from the note
	var questions []models.Question
	questionResult := db.Where("note_id = ?", randomNote.ID).Find(&questions)
	if questionResult.Error != nil {
		fmt.Println("Error fetching questions from DB ", questionResult.Error)
		return fmt.Errorf("error fetching questions: %v", questionResult.Error)
	}

	if len(questions) == 0 {
		fmt.Printf("[Quiz Alert] No questions found for note %d\n", randomNote.ID)
		return nil
	}

	// Pick a random question
	randomQuestion := questions[time.Now().Unix()%int64(len(questions))]

	// Create notification message
	title := "📚 Quiz Time!"
	message := fmt.Sprintf("%s: %s", randomNote.Name, randomQuestion.QuestionText)

	// Limit message length for notifications
	if len(message) > 100 {
		message = message[:97] + "..."
	}

	// Send notification
	fmt.Println("[Quiz Alert] Sending notification to user", userID, "for note", randomNote.ID, "question", randomQuestion.ID)
	services.SendNotification(fbApp, db, userID, title, message, map[string]string{"note_id": fmt.Sprintf("%d", randomNote.ID), "type": "quiz_alert", "question_id": fmt.Sprintf("%d", randomQuestion.ID)})

	return nil
}
