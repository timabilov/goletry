package tasks

import (
	"context"
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
	"gorm.io/gorm"
)

type TryOnGenerationPayload struct {
	UserID  uint `json:"user_id"`
	TryOnID uint `json:"try_on_id"`
}
type ClothingGenerationPayload struct {
	ClothingId uint `json:"clothing_id"`
}

type UserAvatarGeneratePayload struct {
	UserID uint `json:"user_id"`
}

// Client initializes an asynq client for enqueuing tasks
func NewClient() (*asynq.Client, error) {
	return asynq.NewClient(asynq.RedisClientOpt{Addr: "your-redis-connection-string"}), nil
}

// EnqueueTranscribeNote enqueues a clothing for processing
func NewTryOnGenerationTask(userID uint, tryOnID uint) (*asynq.Task, error) {
	payload, err := json.Marshal(TryOnGenerationPayload{TryOnID: tryOnID, UserID: userID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask("generate:tryon", payload), nil

}

// EnqueueTranscribeNote enqueues a clothing for processing
func NewFullBodyAvatarGenerateTask(userID uint) (*asynq.Task, error) {
	payload, err := json.Marshal(UserAvatarGeneratePayload{UserID: userID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask("generate:avatar", payload), nil

}

func NewClothingProcessingTask(clothingId uint) (*asynq.Task, error) {
	payload, err := json.Marshal(ClothingGenerationPayload{ClothingId: clothingId})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask("generate:process_clothing", payload), nil

}

func fetchR2File(awsService services.AWSServiceProvider, r2FilePath *string, entityLog string) ([]byte, string, error) {
	bucketName := os.Getenv("R2_BUCKET_NAME")
	fmt.Printf("[R2: %v] Bucket name: %s\n", entityLog, bucketName)
	fmt.Printf("[R2: %v] Request presigned download url.. ", entityLog)
	if r2FilePath == nil {
		return nil, "", fmt.Errorf("[Clothing: %v] File URL is nil", entityLog)
	}
	fileUrl, err := awsService.GetPresignedR2FileReadURL(context.TODO(), bucketName, *r2FilePath)
	fileName := filepath.Base(*r2FilePath)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error on getting presigned URL for file %s", entityLog, *r2FilePath))
		return nil, fileName, err
	}
	fmt.Printf("Downloading... %s\n", fileUrl)
	fileBytes, err := services.ReadFileFromUrl(fileUrl)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error on downloading file %s: %v", entityLog, *r2FilePath, err))
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

func ProcessAvatarTask(
	ctx context.Context, t *asynq.Task, db *gorm.DB, transcriber services.LLMProcessor,
	awsService services.AWSServiceProvider, fbApp *firebase.App) error {
	google_key := os.Getenv("GOOGLE_API_KEY")
	if google_key == "" {
		sentry.CaptureException(fmt.Errorf("[QUEUE] %s Google API key is not set", string(t.Payload())))
		return fmt.Errorf("[QUEUE] %s Google API key is not set", string(t.Payload()))
	}
	var payload UserAvatarGeneratePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	fmt.Printf("[Avatar: %v] Start Processing\n", payload.UserID)
	var user models.UserAccount
	res := db.First(&user, payload.UserID)
	if res.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Avatar: error on retrieving user for processing %v", payload.UserID))
		return res.Error
	}
	if user.UserFullBodyImageURL == nil {
		saveUserAvatarProcessingFail(db, user, "Failed to identify your avatar image, please try to upload new avatar", false)
		sentry.CaptureException(fmt.Errorf("[Avatar: %v] Error on getting user image for processing", payload.UserID))
		return fmt.Errorf("[Avatar: %v] Error on getting user image", payload.UserID)
	}
	time.Sleep(2 * time.Second) // wait for r2 to be ready
	fileBytes, fileName, err := fetchR2File(awsService, user.UserFullBodyImageURL, "User ID "+fmt.Sprint(payload.UserID))
	if err != nil {
		fmt.Printf("[Avatar: %v] Error on getting file from R2 %s: %v\n", payload.UserID, *user.UserFullBodyImageURL, err)
		saveUserAvatarProcessingFail(db, user, "Failed to read clothing image, please try to create new clothing", false)
		sentry.CaptureException(fmt.Errorf("[Avatar: %v] File path exists, but error on getting file %s: %v", payload.UserID, *user.UserFullBodyImageURL, err))
		return err
	}
	fmt.Printf("[Avatar: %v] Downloaded file size: %d bytes\n", payload.UserID, len(fileBytes))
	imgPath, err := services.CreateTempFile(fileBytes, fileName)
	// clean defer file after processing
	defer func(path string) {
		if err := os.Remove(path); err != nil {
			fmt.Printf("[Avatar: %v] Error removing temporary file %s: %v\n", payload.UserID, path, err)
		} else {
			fmt.Printf("[Avatar: %v] Successfully removed temporary file %s\n", payload.UserID, path)
		}
	}(imgPath)

	if err != nil {
		saveUserAvatarProcessingFail(db, user, "Failed to read your clothing files, please try to create new clothing", true)
		sentry.CaptureException(fmt.Errorf("[Avatar: %v] Error extracting documents from zip %s: %v", payload.UserID, *user.UserFullBodyImageURL, err))
		return err
	}

	var clothingLLMResponseText string
	var clothingLLMResponse *services.LLMResponse

	fmt.Printf("[Avatar: %v] Transform to e-commerce style avatar..\n", payload.UserID)
	model := services.Flash25Image
	modelString := model.String()
	// if user.EnforcedLLMModel != nil {
	// 	model = services.LLMModelName(*user.Company.EnforcedLLMModel)
	// 	modelString = model.String()
	// 	fmt.Printf("[Avatar: %v] [ENFORCE MODEL] Using enforced model: %s\n", payload.UserID, model.String())
	// }

	fmt.Printf("[Avatar: %v] Model: %s\n", payload.UserID, modelString)
	// }

	fmt.Printf("[Avatar: %v] Avatar url %s\n", payload.UserID, *user.UserFullBodyImageURL)
	fmt.Printf("[Avatar: %v] Downloaded avatar: %v:", payload.UserID, imgPath)
	// if db.Save(&user).Error != nil {
	// 	fmt.Printf("[Avatar: %v] Error on saving clothing type detect %v", payload.UserID, err)
	// 	saveUserAvatarProcessingFail(db, user, "Failed to determine clothing type, please try to create new clothing", true)
	// 	sentry.CaptureException(fmt.Errorf("[Avatar: %v] Error on saving clothing mid type detect %v", payload.UserID, err))
	// }

	clothingLLMResponse, err = transcriber.ProcessAvatarTask(imgPath, services.Flash25Image)
	fmt.Printf("[Avatar: %v] Images length: %d", payload.UserID, len(clothingLLMResponse.Images))
	fmt.Println("Images length:", len(clothingLLMResponse.Images))
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Avatar: %v] Error on generating study material %s: %v", payload.UserID, "", err))
		saveUserAvatarProcessingFail(db, user, "Failed to generate avatar, please try again", true)
		return err
	}
	clothingLLMResponseText = clothingLLMResponse.Response
	if strings.Contains(clothingLLMResponseText, "NO_PERSON") {
		saveUserAvatarProcessingFail(db, user, "No person detected in the image, please try to upload new avatar", false)
		sentry.CaptureException(fmt.Errorf("[Avatar: %v] No person detected in the image on generating tryon %s: %v", payload.UserID, "", err))
		return fmt.Errorf("[Avatar: %v] No person detected in the image on generating tryon %s: %v", payload.UserID, "", err)
	}
	if clothingLLMResponseText != "" {
		fmt.Printf("[Avatar: %v] Response is nil but no error provided on generating study material %s: %s", payload.UserID, "", clothingLLMResponseText)
	}
	if len(clothingLLMResponse.Images) == 0 {
		sentry.CaptureException(fmt.Errorf("[Avatar: %v] Response image is nil or empty on generating try on %s: %v", payload.UserID, "", err))
		saveUserAvatarProcessingFail(db, user, "Failed to generate generating preview, please try again", true)
		return fmt.Errorf("[Avatar: %v] Response image is nil or empty on generating try on %s: %v", payload.UserID, "", err)
	}
	if len(clothingLLMResponse.Images) > 1 {
		fmt.Printf("[Avatar: %v] Warning: More than 1 image returned, using the first one\n", payload.UserID)
	}
	generatedImageBytes := clothingLLMResponse.Images[0]
	// err = os.WriteFile("nanobanana.png", generatedImageBytes, 0644)
	// if err != nil {
	// 	log.Fatalf("failed to write file: %s", err)
	// }

	fmt.Println("Successfully wrote data to file1.txt")
	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	// todo clean and map the same file name as in FE UI otherwise **FAIL**
	safeFileName := fmt.Sprintf("/user/%v/generation/%s", user.ID, "generation.png")

	uploadUrl, presignErr := awsService.PresignLink(context.Background(), bucketName, safeFileName)
	if presignErr != nil {
		saveUserAvatarProcessingFail(db, user, "Failed to upload generated avatar, please try again", true)
		fmt.Printf("[Avatar: %v]  Unable to create presign link for tryon %s!\n", user.ID, presignErr)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Unable to create presign for tryon %s", payload.UserID, presignErr))
		return presignErr
	}
	// parse file from Output of ytdlp file path in fmt.Sprintf("clothing-%v.%%(ext)s", clothing.ID)
	respBody, statusCode, err := awsService.UploadToPresignedURL(context.Background(), bucketName, uploadUrl, generatedImageBytes)
	fmt.Printf("[Avatar: %v] R2 Upload response body: %s, status code: %v\n", payload.UserID, respBody, statusCode)
	if err != nil || statusCode > 299 {
		saveUserAvatarProcessingFail(db, user, "Failed to upload generated avatar, please try again", true)
		fmt.Printf("[Avatar: %v] Error on uploading file not success code or err %s: %v\n", payload.UserID, safeFileName, err)
		sentry.CaptureException(fmt.Errorf("[Avatar: %v] Error on uploading file %s: %v", payload.UserID, safeFileName, err))
		return err
	}
	fmt.Printf("[Avatar: %v] Success.. Removing local one\n", payload.UserID)
	user.UserFullBodyImageURL = &safeFileName
	user.FullBodyAvatarStatus = "completed"

	user.LLMTotalTokenCount = &clothingLLMResponse.TotalTokenCount
	user.LLMInputTokenCount = &clothingLLMResponse.InputTokenCount
	user.LLMThoughtsTokenCount = &clothingLLMResponse.ThoughtsTokenCount
	user.LLMOutputTokenCount = &clothingLLMResponse.OutputTokenCount
	user.LLMThoughts = &clothingLLMResponse.Thoughts
	user.LLMModel = &modelString

	// save question from llm

	tx := db.Save(&user)
	if tx.Error != nil {
		saveUserAvatarProcessingFail(db, user, "Failed to save generated avatar, please try again", true)
		sentry.CaptureException(fmt.Errorf("[Avatar %v] Error on saving avatar at the end", payload.UserID))
		return tx.Error
	}
	fmt.Printf("[Avatar: %v] Generation finished succesfully..", payload.UserID)

	// Save result back to database
	return nil
}

// InitialProcessNote handles the task of processing a clothing with the LLM
func ProcessClothingTask(
	ctx context.Context, t *asynq.Task, db *gorm.DB, transcriber services.LLMProcessor,
	awsService services.AWSServiceProvider, fbApp *firebase.App) error {
	google_key := os.Getenv("GOOGLE_API_KEY")
	if google_key == "" {
		sentry.CaptureException(fmt.Errorf("[QUEUE] %s Google API key is not set", string(t.Payload())))
		return fmt.Errorf("[QUEUE] %s Google API key is not set", string(t.Payload()))
	}
	var payload ClothingGenerationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	fmt.Printf("[Clothing: %v] Start Processing\n", payload.ClothingId)
	var clothing models.Clothing
	res := db.Joins("Company").First(&clothing, payload.ClothingId)
	if res.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Error on retrieving clothing for processing %v", payload.ClothingId))
		return res.Error
	}
	if clothing.ClothingType == "" {
		saveClothingProcessingFail(db, clothing, "Failed to identify clothing type, please try to create new clothing", false)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error on getting clothing type", payload.ClothingId))
		return fmt.Errorf("[Clothing: %v] Error on getting clothing type", payload.ClothingId)
	}
	fileBytes, fileName, err := fetchR2File(awsService, clothing.ImageURL, "Clothing ID "+fmt.Sprint(payload.ClothingId))
	if err != nil {
		saveClothingProcessingFail(db, clothing, "Failed to read clothing image, please try to create new clothing", false)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] File path exists, but error on getting file %s: %v", payload.ClothingId, *clothing.ImageURL, err))
		return err
	}
	fmt.Printf("[Clothing: %v] Downloaded file size: %d bytes\n", payload.ClothingId, len(fileBytes))
	imgPath, err := services.CreateTempFile(fileBytes, fileName)
	// clean defer file after processing
	defer func(path string) {
		if err := os.Remove(path); err != nil {
			fmt.Printf("[Clothing: %v] Error removing temporary file %s: %v\n", payload.ClothingId, path, err)
		} else {
			fmt.Printf("[Clothing: %v] Successfully removed temporary file %s\n", payload.ClothingId, path)
		}
	}(imgPath)

	if err != nil {
		saveClothingProcessingFail(db, clothing, "Failed to read your clothing files, please try to create new clothing", true)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error extracting documents from zip %s: %v", payload.ClothingId, *clothing.ImageURL, err))
		return err
	}

	var clothingLLMResponseText string
	var clothingLLMResponse *services.LLMResponse

	fmt.Printf("[Clothing: %v] Type: %s\n", clothing.ID, clothing.ClothingType)

	fmt.Printf("[Clothing: %v] Transform to e-commerce style white image..\n", payload.ClothingId)
	model := services.Flash25
	modelString := model.String()
	if clothing.Company.EnforcedLLMModel != nil {
		model = services.LLMModelName(*clothing.Company.EnforcedLLMModel)
		modelString = model.String()
		fmt.Printf("[Clothing: %v] [ENFORCE MODEL] Using enforced model: %s\n", payload.ClothingId, model.String())
	}

	fmt.Printf("[Clothing: %v] Model: %s\n", payload.ClothingId, modelString)
	// }

	if clothing.ClothingType == "" {
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error on getting clothing type", payload.ClothingId))
		return fmt.Errorf("[Clothing: %v] Error on getting clothing type", payload.ClothingId)
	}
	fmt.Printf("[Clothing: %v] Note type %s\n", payload.ClothingId, clothing.ClothingType)
	fmt.Printf("[Clothing: %v] Extracted zip document paths %v:", payload.ClothingId, imgPath)
	if db.Save(&clothing).Error != nil {
		fmt.Printf("[Clothing: %v] Error on saving clothing mid type detect %v", payload.ClothingId, err)
		saveClothingProcessingFail(db, clothing, "Failed to determine clothing type, please try to create new clothing", true)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error on saving clothing mid type detect %v", payload.ClothingId, err))
	}

	clothingLLMResponse, err = transcriber.ProcessClothing(imgPath, services.Flash25Image)
	if err != nil {
		fmt.Printf("[Clothing: %v] Error on transcribing documents %v: %v\n", payload.ClothingId, imgPath, err)
		if strings.Contains(err.Error(), "content violation") {
			saveClothingProcessingFail(db, clothing, "Sorry, it seems that this clothing contains violated content that we cannot process.", false)
			sentry.CaptureException(fmt.Errorf("[Clothing: %v] Content violation on transcribing clothing %s: %v", payload.ClothingId, *clothing.ImageURL, err))
			return nil
		}
		saveClothingProcessingFail(db, clothing, "Failed to transribe your clothing, please try to create new clothing", true)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error on transcribing documents %s: %v", payload.ClothingId, *clothing.ImageURL, err))
		return err
	}
	if clothingLLMResponse == nil {
		fmt.Printf("[Clothing: %v] Response is nil but no error provided on transcribing %v: %v\n", payload.ClothingId, imgPath, err)
		saveClothingProcessingFail(db, clothing, "Failed to transribe your clothing, please try to create new clothing", true)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Response is nil but no error provided on transcribing documents %v: %v", payload.ClothingId, imgPath, err))
		return fmt.Errorf("[Clothing: %v] Response is nil but no error provided on transcribing documents %s: %v", payload.ClothingId, *clothing.ImageURL, err)
	}
	clothingLLMResponseText = clothingLLMResponse.Response

	cleanNoteContent := cleanAIResponseText(clothingLLMResponseText)
	fmt.Println(cleanNoteContent)
	// content is json read attributes
	// fmt.Printf("[Clothing: %v] LLM Processed: %q, IT: %d, OT: %d, TT: %d, TOT: %d, Thoughts: %s..\n", payload.ClothingId, cleanNoteContent, clothingLLMResponse.InputTokenCount, clothingLLMResponse.OutputTokenCount, clothingLLMResponse.ThoughtsTokenCount, clothingLLMResponse.TotalTokenCount, clothingLLMResponse.Thoughts)
	// var parsedNoteData services.ClothingTranscribeResponse
	// if err := json.Unmarshal([]byte(cleanNoteContent), &parsedNoteData); err != nil {
	// 	fmt.Println(err)
	// 	fmt.Printf("[Clothing: %v] Error on parsing Gemini %s AI json %s", payload.ClothingId, model.String(), clothingLLMResponseText)
	// 	clothing.FailedToProcessLLMNoteResponse = cleanNoteContent
	// 	saveClothingProcessingFail(db, clothing, "Failed to transcribe clothing content, please try again later", true)
	// 	sentry.CaptureException(fmt.Errorf("[Clothing: %v] Error on parsing Gemini %s AI json %s", payload.ClothingId, services.Pro25, clothingLLMResponseText))
	// 	return err
	// }
	// parsedNoteData.Transcription = cleanAIResponseSeparateFieldsText(parsedNoteData.Transcription)
	// parsedNoteData.MD_Summary = cleanAIResponseSeparateFieldsText(parsedNoteData.MD_Summary)

	// convert quiz to json string
	// quizJSONBytes, err := json.Marshal(parsedNoteData.QuizJSON)
	// quizJSONString :=
	clothing.Status = "in_closet"
	clothing.ProcessingStatus = "completed"
	// clothing.Transcript = &parsedNoteData.Transcription
	// clothing.ConspectAIMD5 = &parsedNoteData.MD_Summary
	// // clothing.QuizJSON = &quizJSONString
	// clothing.Name = parsedNoteData.Name
	// clothing.Language = parsedNoteData.Language
	// clothing.Status = "transcribed"
	// clothing.QuizStatus = "ready_to_generate"
	// clothing.TotalTokenCount = &clothingLLMResponse.TotalTokenCount
	// clothing.InputTokenCount = &clothingLLMResponse.InputTokenCount
	// clothing.ThoughtsTokenCount = &clothingLLMResponse.ThoughtsTokenCount
	// clothing.OuputTokenCount = &clothingLLMResponse.OutputTokenCount
	// clothing.Thoughts = &clothingLLMResponse.Thoughts
	// clothing.LLMModel = &modelString
	// clothing.ProcessingErrorMessage = nil
	tx := db.Save(&clothing)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Error on saving clothing %v", payload.ClothingId))
		return tx.Error
	}
	fmt.Printf("[Clothing: %v] Processing finished succesfully..", payload.ClothingId)
	// Save result back to database
	return nil
}

// Generates study material
func HandleTryOnGenerationTask(ctx context.Context, t *asynq.Task, db *gorm.DB, llmProcessor services.LLMProcessor, awsService services.AWSServiceProvider) error {
	var payload TryOnGenerationPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return err
	}
	fmt.Printf("[Try on Gen: %v] Start Processing\n", payload.TryOnID)
	// Fetch clothing from database

	var tryOnGeneration models.ClothingTryonGeneration
	res := db.Joins("TopClothing").Joins("BottomClothing").Joins("ShoesClothing").Joins("Accessory").First(&tryOnGeneration, payload.TryOnID)
	if res.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Error on retrieving clothing for generation %v", payload.TryOnID))
		return res.Error
	}
	if tryOnGeneration.Status == "completed" {
		fmt.Printf("[Try on Gen: %v] Try on generation already generated\n", payload.TryOnID)
		return nil
	}

	var user models.UserAccount
	resUser := db.First(&user, payload.UserID)
	if resUser.Error != nil {
		sentry.CaptureException(fmt.Errorf("[QUEUE] Error on retrieving user for try on generation %v", payload.UserID))
		return resUser.Error
	}
	if user.UserFullBodyImageURL == nil || *user.UserFullBodyImageURL == "" {
		saveTryOnGenerationFail(db, tryOnGeneration, "Please set your avatar first", false)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] User full body image is missing, please upload a full body image to use try on generation", payload.TryOnID))
		return fmt.Errorf("[Try on Gen: %v] User full body image is missing, please upload a full body image to use try on generation", payload.TryOnID)
	}

	model := services.Flash25Image
	modelString := model.String()
	fmt.Printf("[Try on Gen: %v] Model: %s\n", payload.TryOnID, modelString)
	var topImgPath, bottomImgPath, shoesImgPath, accessoryImgPath string
	if tryOnGeneration.TopClothing != nil && tryOnGeneration.TopClothing.ImageURL == nil {
		saveTryOnGenerationFail(db, tryOnGeneration, "Top clothing image is missing, please select a valid top clothing", false)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] Top clothing image is missing, please select a valid top clothing", payload.TryOnID))
		return nil
	}

	if tryOnGeneration.BottomClothing != nil && tryOnGeneration.TopClothing.ImageURL == nil {
		saveTryOnGenerationFail(db, tryOnGeneration, "Bottom clothing image is missing, please select a valid top clothing", false)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] Bottom clothing image is missing, please select a valid top clothing", payload.TryOnID))
		return nil
	}

	if tryOnGeneration.ShoesClothing != nil && tryOnGeneration.ShoesClothing.ImageURL == nil {
		saveTryOnGenerationFail(db, tryOnGeneration, "Shoes clothing image is missing, please select a valid top clothing", false)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] Shoes clothing image is missing, please select a valid top clothing", payload.TryOnID))
		return nil
	}

	if tryOnGeneration.Accessory != nil && tryOnGeneration.Accessory.ImageURL == nil {
		saveTryOnGenerationFail(db, tryOnGeneration, "Accessory clothing image is missing, please select a valid top clothing", false)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] Accessory clothing image is missing, please select a valid top clothing", payload.TryOnID))
		return nil
	}
	fmt.Println("Fetching clothing files...", tryOnGeneration.TopClothing, *tryOnGeneration.TopClothingID, "hello there")
	time.Sleep(2 * time.Second) // wait for r2 to be ready
	if tryOnGeneration.TopClothing != nil {
		fmt.Printf("[Try on Gen: %v] Adding top clothing ID: %v\n", payload.TryOnID, tryOnGeneration.TopClothing.ID)
		topFileBytes, topFileName, err := fetchR2File(awsService, tryOnGeneration.TopClothing.ImageURL, fmt.Sprintf("TryOnGen-%v-TopClothing-%v", payload.TryOnID, tryOnGeneration.TopClothing.ID))
		if err != nil {
			saveTryOnGenerationFail(db, tryOnGeneration, "Failed to fetch top clothing image, please try again", true)
			sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] R2 Fetch top file error, but error on getting file %s: %v", payload.TryOnID, *tryOnGeneration.TopClothing.ImageURL, err))
			return err
		}
		topImgPath, err = services.CreateTempFile(topFileBytes, topFileName)
		if err != nil {
			saveTryOnGenerationFail(db, tryOnGeneration, "Failed to read top clothing image, please try again", true)
			sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] File path exists, but error on getting file %s: %v", payload.TryOnID, *tryOnGeneration.TopClothing.ImageURL, err))
			return err
		}

		// clean defer file after processing
		defer func(path string) {
			if err := os.Remove(path); err != nil {
				fmt.Printf("[Clothing: %v] Error removing temporary file %s: %v\n", payload.TryOnID, path, err)
			} else {
				fmt.Printf("[Clothing: %v] Successfully removed temporary file %s\n", payload.TryOnID, path)
			}
		}(topImgPath)
	}

	if tryOnGeneration.BottomClothing != nil {
		fmt.Printf("[Try on Gen: %v] Adding bottom clothing ID: %v\n", payload.TryOnID, tryOnGeneration.BottomClothing.ID)
		bottomFileBytes, bottomFileName, err := fetchR2File(awsService, tryOnGeneration.BottomClothing.ImageURL, fmt.Sprintf("TryOnGen-%v-BottomClothing-%v", payload.TryOnID, tryOnGeneration.BottomClothing.ID))
		if err != nil {
			saveTryOnGenerationFail(db, tryOnGeneration, "Failed to fetch bottom clothing image, please try again", true)
			sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] R2 Fetch bottom file error, but error on getting file %s: %v", payload.TryOnID, *tryOnGeneration.TopClothing.ImageURL, err))
			return err
		}
		if err != nil {
			saveTryOnGenerationFail(db, tryOnGeneration, "Failed to fetch bottom clothing image, please try again", true)
			sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] File path exists, but error on getting file %s: %v", payload.TryOnID, *tryOnGeneration.TopClothing.ImageURL, err))
			return err
		}
		bottomImgPath, err = services.CreateTempFile(bottomFileBytes, bottomFileName)
		if err != nil {
			saveTryOnGenerationFail(db, tryOnGeneration, "Failed to read bottom clothing image, please try again", true)
			sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] File path exists, but error on getting file %s: %v", payload.TryOnID, *tryOnGeneration.BottomClothing.ImageURL, err))
			return err
		}
		defer func(path string) {
			if err := os.Remove(path); err != nil {
				fmt.Printf("[Clothing: %v] Error removing temporary file %s: %v\n", payload.TryOnID, path, err)
			} else {
				fmt.Printf("[Clothing: %v] Successfully removed temporary file %s\n", payload.TryOnID, path)
			}
		}(bottomImgPath)

	}
	if tryOnGeneration.ShoesClothing != nil {
		fmt.Printf("[Try on Gen: %v] Adding shoes clothing ID: %v\n", payload.TryOnID, tryOnGeneration.ShoesClothing.ID)
		shoesFileBytes, shoesFileName, err := fetchR2File(awsService, tryOnGeneration.ShoesClothing.ImageURL, fmt.Sprintf("TryOnGen-%v-ShoesClothing-%v", payload.TryOnID, tryOnGeneration.ShoesClothing.ID))
		shoesImgPath, err = services.CreateTempFile(shoesFileBytes, shoesFileName)
		if err != nil {
			saveTryOnGenerationFail(db, tryOnGeneration, "Failed to read shoes clothing image, please try again", true)
			sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] File path exists, but error on getting file %s: %v", payload.TryOnID, *tryOnGeneration.ShoesClothing.ImageURL, err))
			return err
		}
		defer func(path string) {
			if err := os.Remove(path); err != nil {
				fmt.Printf("[Clothing: %v] Error removing temporary file %s: %v\n", payload.TryOnID, path, err)
			} else {
				fmt.Printf("[Clothing: %v] Successfully removed temporary file %s\n", payload.TryOnID, path)
			}
		}(shoesImgPath)
	}

	clothesToWear := []string{topImgPath, bottomImgPath, shoesImgPath, accessoryImgPath}
	personAvatarBytes, personFileName, err := fetchR2File(awsService, user.UserFullBodyImageURL, "person-avatar")
	if err != nil {
		saveTryOnGenerationFail(db, tryOnGeneration, "Failed to fetch user avatar image, please try again", true)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] R2 Fetch person avatar file error, but error on getting file %s: %v", payload.TryOnID, *user.UserFullBodyImageURL, err))
		return err
	}
	personAvatarPath, err := services.CreateTempFile(personAvatarBytes, personFileName)
	if err != nil {
		saveTryOnGenerationFail(db, tryOnGeneration, "Failed to read user avatar image, please try again", true)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] File path exists, but error on getting file %s: %v", payload.TryOnID, *user.UserFullBodyImageURL, err))
		return err
	}
	defer func(path string) {
		if err := os.Remove(path); err != nil {
			fmt.Printf("[Clothing: %v] Error removing temporary file %s: %v\n", payload.TryOnID, path, err)
		} else {
			fmt.Printf("[Clothing: %v] Successfully removed temporary file %s\n", payload.TryOnID, path)
		}
	}(personAvatarPath)
	fmt.Printf("[Try on Gen: %v] Clothing to wear paths: %v", payload.TryOnID, clothesToWear)
	clothingLLMResponse, err := llmProcessor.GenerateTryOn(personAvatarPath, clothesToWear, model)
	fmt.Printf("[Try on Gen: %v] Images length: %d", payload.TryOnID, len(clothingLLMResponse.Images))
	fmt.Println("Images length:", len(clothingLLMResponse.Images))
	if err != nil {
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] Error on generating study material %s: %v", payload.TryOnID, "", err))
		saveTryOnGenerationFail(db, tryOnGeneration, "Failed to generate study material, please try again", true)
		return err
	}
	clothingLLMResponseText := clothingLLMResponse.Response
	if clothingLLMResponseText != "" {
		fmt.Printf("[Try on Gen: %v] Response is nil but no error provided on generating study material %s: %s", payload.TryOnID, "", clothingLLMResponseText)
	}
	if len(clothingLLMResponse.Images) == 0 {
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] Response image is nil or empty on generating try on %s: %v", payload.TryOnID, "", err))
		saveTryOnGenerationFail(db, tryOnGeneration, "Failed to generate generating preview, please try again", true)
		return fmt.Errorf("[Try on Gen: %v] Response image is nil or empty on generating try on %s: %v", payload.TryOnID, "", err)
	}
	if len(clothingLLMResponse.Images) > 1 {
		fmt.Printf("[Try on Gen: %v] Warning: More than 1 image returned, using the first one\n", payload.TryOnID)
	}
	generatedImageBytes := clothingLLMResponse.Images[0]
	// err = os.WriteFile("nanobanana.png", generatedImageBytes, 0644)
	// if err != nil {
	// 	log.Fatalf("failed to write file: %s", err)
	// }

	fmt.Println("Successfully wrote data to file1.txt")
	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	// todo clean and map the same file name as in FE UI otherwise **FAIL**
	safeFileName := fmt.Sprintf("/tryon/%v/generation/%s", tryOnGeneration.ID, "generation.png")

	uploadUrl, presignErr := awsService.PresignLink(context.Background(), bucketName, safeFileName)
	if presignErr != nil {
		fmt.Printf("[Try on Gen: %v] Youtube Unable to create presign link for tryon %s!\n", tryOnGeneration.ID, presignErr)
		sentry.CaptureException(fmt.Errorf("[Clothing: %v] Unable to create presign for tryon %s", payload.TryOnID, presignErr))
		return presignErr
	}
	// parse file from Output of ytdlp file path in fmt.Sprintf("clothing-%v.%%(ext)s", clothing.ID)
	respBody, statusCode, err := awsService.UploadToPresignedURL(context.Background(), bucketName, uploadUrl, generatedImageBytes)
	fmt.Printf("[Try on: %v] R2 Upload response body: %s, status code: %v\n", payload.TryOnID, respBody, statusCode)
	if err != nil || statusCode > 299 {
		fmt.Printf("[Try on Gen: %v] Try on Error on uploading generated file %s: %v\n", payload.TryOnID, safeFileName, err)
		sentry.CaptureException(fmt.Errorf("[Try on Gen: %v] Error on uploading file %s: %v", payload.TryOnID, safeFileName, err))
		return err
	}
	fmt.Printf("[Try on Gen: %v] Success.. Removing local one\n", payload.TryOnID)
	tryOnGeneration.TryOnPreviewImageURL = &safeFileName
	tryOnGeneration.Status = "completed"

	tryOnGeneration.LLMTotalTokenCount = &clothingLLMResponse.TotalTokenCount
	tryOnGeneration.LLMInputTokenCount = &clothingLLMResponse.InputTokenCount
	tryOnGeneration.LLMThoughtsTokenCount = &clothingLLMResponse.ThoughtsTokenCount
	tryOnGeneration.LLMOutputTokenCount = &clothingLLMResponse.OutputTokenCount
	tryOnGeneration.LLMThoughts = &clothingLLMResponse.Thoughts
	tryOnGeneration.LLMModel = &modelString

	// save question from llm

	tx := db.Save(&tryOnGeneration)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Try on Gen %v] Error on saving clothing at the end", payload.TryOnID))
		return tx.Error
	}
	fmt.Printf("[Try on Gen: %v] Transcribing finished succesfully..", payload.TryOnID)

	// Save result back to database
	return nil
}

func saveTryOnGenerationFail(db *gorm.DB, tryOnGeneration models.ClothingTryonGeneration, message string, shouldRetry bool) error {
	// clothing.QuizStatus = "failed"
	tryOnGeneration.GenerationRetryTimes = tryOnGeneration.GenerationRetryTimes + 1
	if !shouldRetry || tryOnGeneration.GenerationRetryTimes >= 3 {

		tryOnGeneration.GenerationErrorMessage = services.StrPointer(message)
		tryOnGeneration.Status = "failed"
	}

	tx := db.Save(&tryOnGeneration)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Fail Try On %v] Error on saving clothing quiz for failed status", tryOnGeneration.ID))
		return tx.Error
	}
	return nil
}

func saveClothingProcessingFail(db *gorm.DB, clothing models.Clothing, msg string, shouldRetry bool) error {
	clothing.ProcessRetryTimes = clothing.ProcessRetryTimes + 1
	if !shouldRetry || clothing.ProcessRetryTimes >= 3 {
		clothing.ProcessErrorMessage = &msg

		clothing.Status = "failed"
	}
	tx := db.Save(&clothing)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Fail Clothing %v] Error on saving clothing for failed status", clothing.ID))
		return tx.Error
	}
	return nil
}

func saveUserAvatarProcessingFail(db *gorm.DB, user models.UserAccount, msg string, shouldRetry bool) error {
	user.AvatarProcessRetryTimes = user.AvatarProcessRetryTimes + 1
	if !shouldRetry || user.AvatarProcessRetryTimes >= 3 {
		user.FullBodyAvatarProcessingErrorMessage = &msg

		user.Status = "failed"
	}
	tx := db.Save(&user)
	if tx.Error != nil {
		sentry.CaptureException(fmt.Errorf("[Fail Avatar %v] Error on saving user avatar for failed status", user.ID))
		return tx.Error
	}
	return nil
}
func ScheduledQuizAlertTask(ctx context.Context, t *asynq.Task, db *gorm.DB, fbApp *firebase.App) error {

	fmt.Printf("[Scheduled] Processing something\n")

	return nil
}
