package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"letryapi/models"
	"letryapi/services"
	"letryapi/tasks"

	firebase "firebase.google.com/go/v4"
	"github.com/getsentry/sentry-go"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ClothingRoutes struct to hold dependencies
// UpdateClothingRequest defines the request body for updating quiz alerts
type UpdateClothingRequest struct {
	// QuizAlertsEnabled  *bool `json:"quiz_alerts_enabled" validate:"required"`
	AlertWhenProcessed *bool `json:"alert_when_processed"`
}

type ClothingUploadFileRequest struct {
	FileName *string `json:"file_name" validate:"required,max=200"`
}

// Request structs for validation
type CreateClothingIn struct {
	Name         string  `json:"name" validate:"omitempty,max=100"`
	FileName     *string `json:"file_name" validate:"required,max=200"`
	Description  *string `json:"description" validate:"omitempty,max=500"`
	ClothingType string  `json:"clothing_type" validate:"required,oneof=top bottom shoes accessory"` // e.g., top, bottom, shoes, accessory
	AddToCloset  *bool   `json:"add_to_closet" validate:"required"`
}

type GenerateTryOnIn struct {
	TopClothingID    *uint `json:"top_clothing_id"`
	BottomClothingID *uint `json:"bottom_clothing_id"`
	ShoesClothingID  *uint `json:"shoes_clothing_id"`
	AccessoryID      *uint `json:"accessory_id"`

	Status string `json:"status" validate:"required,oneof=temporary in_closet"`
}

// Removed ClothingUploadFileRequest and CreateFolderRequest - not needed

type GenericResponse struct {
	message string
}

// Response structs
type ClothingResponse struct {
	ID           uint    `json:"id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	ClothingType string  `json:"clothing_type"`
	Status       string  `json:"status"`
	Uri          *string `json:"uri,omitempty"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type ClothingCreatedResponse struct {
	ClothingResponse ClothingResponse `json:"clothes"`
	FileUploadUrl    string           `json:"file_upload_url"`
}

type TryOnGenerationCreatedResponse struct {
	TryOnID              uint    `json:"try_on_id"`
	Status               string  `json:"status"`
	TryOnPreviewImageURL *string `json:"try_on_preview_image_url,omitempty"`
}

type ClothesListResponse struct {
	Tops        []ClothingResponse `json:"tops"`
	Bottoms     []ClothingResponse `json:"bottoms"`
	Shoes       []ClothingResponse `json:"shoes"`
	Accessories []ClothingResponse `json:"accessories"`
}

type ClothesController struct {
	Google      services.GoogleServiceProvider
	AWSService  services.AWSServiceProvider
	FirebaseApp *firebase.App
	URLCache    services.URLCacheServiceProvider
}

func (controller *ClothesController) ClothingRoutes(g *echo.Group) {
	g.POST("/create", controller.CreateClothing)
	g.POST("/tryon", controller.CreateClothing)
	g.GET("/list", controller.ListClothes)
}

func (controller *ClothesController) CreateClothing(c echo.Context) error {
	var req CreateClothingIn
	if err := c.Bind(&req); err != nil {
		fmt.Println(err)
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
	asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}

	if req.FileName == nil || *req.FileName == "" {
		sentry.CaptureException(fmt.Errorf("Image was not provided when creating clothing %s, user %v", req.Name, user.ID))
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Sorry, it seems image was not provided, please try again"})
	}
	company := user.Memberships[0].Company
	if string(company.Subscription) == "free" {
		var totalClothingCount int64
		// if currentCompany.EnforcedDailyClothingLimit == nil {
		if err := db.Model(&models.Clothing{}).Where("company_id = ?", company.ID).Count(&totalClothingCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get clothe data"})
		}
		fmt.Printf("[User %v] Free plan, clothe count: %v", user.ID, totalClothingCount)
		if totalClothingCount >= 2 {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "You have reached the free limit of total 2 clothes, please subscribe"})
		}
	}

	if company.EnforcedDailyClothingLimit != nil {
		// get daily clothe count of user
		var dailyClothingCount int64
		today := time.Now().UTC().Format("2006-01-02")
		if err := db.Model(&models.Clothing{}).Where("company_id = ? AND DATE(created_at) = ?", company.ID, today).Count(&dailyClothingCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get clothe data"})
		}
		fmt.Printf("[User %v] Enforced daily limit, clothe count: %v", user.ID, dailyClothingCount)
		if dailyClothingCount >= int64(*company.EnforcedDailyClothingLimit) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": fmt.Sprintf("You have reached the limit of %v daily clothes. Please wait for the next day.", dailyClothingCount)})
		}
	}
	clothing := models.Clothing{
		Name:         req.Name,
		Description:  req.Description,
		ClothingType: req.ClothingType,
		OwnerID:      user.ID,
		// Status:       req.Status,
		CompanyID: user.Memberships[0].CompanyID,
		// Company:    user.Memberships[0].Company,
	}
	var bucketName = services.GetEnv("R2_BUCKET_NAME", "")
	var uploadUrl string
	var presignErr error
	// todo clean and map the same file name as in FE UI otherwise **FAIL**
	safeFileName := fmt.Sprintf("clothes/%s", *req.FileName)

	// TODO LIMIT FILE SIZE THAT USER CAN UPLOAD !!!!!!
	uploadUrl, presignErr = controller.AWSService.PresignLink(context.Background(), bucketName, safeFileName)
	clothing.ImageURL = &safeFileName
	if presignErr != nil {
		log.Printf("Unable to presign generate for %s!, %s", clothing.Name, presignErr)
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message": "Error while creating clothe with attachment",
		})
	}
	// Save to database
	if err := db.Create(&clothing).Error; err != nil {
		sentry.CaptureException(err)
		return err
	}
	if req.AddToCloset != nil && *req.AddToCloset {
		clothing.ProcessingStatus = "pending"
		if err := db.Save(&clothing).Error; err != nil {
			sentry.CaptureException(err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Failed to update clothe status, please try again"})
		}
		task, err := tasks.NewClothingProcessingTask(clothing.ID)
		if err != nil {
			sentry.CaptureException(err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Sorry, could not process clothing, please try again"})
		}
		info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Queue("generate"))
		if err != nil {
			sentry.CaptureException(err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Sorry, could not process clothing, please try again"})
		}
		fmt.Println("[Queue] Process clothing task submitted, Clothing ID: ", clothing.ID, " Task ID: ", info.ID)
	}

	// Prepare response
	response := ClothingCreatedResponse{
		ClothingResponse: ClothingResponse{
			ID:           clothing.ID,
			Name:         clothing.Name,
			Description:  clothing.Description,
			ClothingType: clothing.ClothingType,
			Status:       clothing.Status,
			CreatedAt:    clothing.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:    clothing.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		},
		FileUploadUrl: uploadUrl,
	}

	return c.JSON(http.StatusCreated, response)
}

func (controller *ClothesController) GenerateTryOn(c echo.Context) error {
	var req GenerateTryOnIn
	if err := c.Bind(&req); err != nil {
		fmt.Println(err)
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
	if user.UserFullBodyImageURL == nil || *user.UserFullBodyImageURL == "" {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "You have to set your avatar first before generating try-on"})
	}
	asynqClient, ok := c.Get("__asynqclient").(*asynq.Client)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Service is not available, please try again a bit later"})
	}
	company := user.Memberships[0].Company
	if string(company.Subscription) == "free" {
		var totalClothingCount int64
		// if currentCompany.EnforcedDailyClothingLimit == nil {
		if err := db.Model(&models.Clothing{}).Where("company_id = ?", company.ID).Count(&totalClothingCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get clothe data"})
		}
		fmt.Printf("[User %v] Free plan, clothe count: %v", user.ID, totalClothingCount)
		if totalClothingCount >= 2 {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "You have reached the free limit of total 2 generations, please subscribe"})
		}
	}

	if company.EnforcedDailyTryOnLimit != nil {
		// get daily clothe count of user
		var dailyClothingCount int64
		today := time.Now().UTC().Format("2006-01-02")
		if err := db.Model(&models.Clothing{}).Where("company_id = ? AND DATE(created_at) = ?", company.ID, today).Count(&dailyClothingCount).Error; err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get clothe data"})
		}
		fmt.Printf("[User %v] Enforced daily limit, clothe count: %v", user.ID, dailyClothingCount)
		if dailyClothingCount >= int64(*company.EnforcedDailyTryOnLimit) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": fmt.Sprintf("You have reached the limit of %v daily generations. Please wait for the next day.", dailyClothingCount)})
		}
	}
	// TODO check R2 head request for all clothes too see whether files were uploaded maximum for 2 seconds!
	try_on_generation := models.ClothingTryonGeneration{
		TopClothingID:          req.TopClothingID,
		BottomClothingID:       req.BottomClothingID,
		ShoesClothingID:        req.ShoesClothingID,
		AccessoryID:            req.AccessoryID,
		UserAccountID:          user.ID,
		CompanyID:              company.ID,
		GeneratedWithAvatarURL: *user.UserFullBodyImageURL,
		Status:                 "pending",
	}

	// Save to database
	if err := db.Create(&try_on_generation).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to generate try-on, please try again"})
	}

	// Prepare response
	response := TryOnGenerationCreatedResponse{
		TryOnID:              try_on_generation.ID,
		Status:               try_on_generation.Status,
		TryOnPreviewImageURL: try_on_generation.TryOnPreviewImageURL,
	}

	task, err := tasks.NewTryOnGenerationTask(user.ID, try_on_generation.ID)
	if err != nil {
		sentry.CaptureException(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Sorry, could not start generation, please try again"})
	}
	info, err := asynqClient.Enqueue(task, asynq.MaxRetry(3), asynq.Queue("generate"))
	if err != nil {
		sentry.CaptureException(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"message": "Sorry, could not start generation, please try again"})
	}
	fmt.Println("[Queue] Try on generation task submitted, Try ID: ", try_on_generation.ID, " Task ID: ", info.ID)

	return c.JSON(http.StatusCreated, response)
}

// populatePresignedClothingImages takes raw clothing models and enriches them with presigned URLs concurrently.
// This version includes a failsafe for when the cache system itself fails.
func (controller *ClothesController) populatePresignedClothingImages(ctx context.Context, clothes []models.Clothing) []ClothingResponse {
	if len(clothes) == 0 {
		return []ClothingResponse{}
	}

	var wg sync.WaitGroup
	processedResponses := make([]ClothingResponse, len(clothes))
	bucketName := services.GetEnv("R2_BUCKET_NAME", "") // Assuming you have a way to get this

	for i, clothingItem := range clothes {
		wg.Add(1)
		go func(index int, item models.Clothing) {
			defer wg.Done()

			var imageUrl string
			if item.ImageURL != nil && *item.ImageURL != "" {
				objectKey := *item.ImageURL

				// Attempt to get the URL from the cache service first.
				url, err := controller.URLCache.GetReadURL(ctx, objectKey)

				if err == nil {
					// SUCCESS: The cache system worked (either a hit or a miss+load).
					imageUrl = url
				} else {
					// FAILURE: The cache system itself failed! This is an exceptional event.
					// We will now trigger our manual failsafe.
					log.Printf("CACHE WARNING: Cache system failed for key '%s': %v. Triggering manual R2 fallback.", objectKey, err)

					// Log the cache failure to Sentry for monitoring.
					sentry.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("failure_type", "cache_system")
						scope.SetExtra("objectKey", objectKey)
						sentry.CaptureException(err)
					})

					// Failsafe: Bypass the cache and call the AWS service directly.
					fallbackUrl, fallbackErr := controller.AWSService.GetPresignedR2FileReadURL(ctx, bucketName, objectKey)
					if fallbackErr != nil {
						// The fallback also failed. This is a critical error.
						log.Printf("CRITICAL: Manual R2 fallback also failed for key '%s': %v", objectKey, fallbackErr)
						sentry.CaptureException(fallbackErr)
						// imageUrl remains empty, but we don't fail the entire request.
					} else {
						// Failsafe succeeded.
						imageUrl = fallbackUrl
					}
				}
			}
			// Map the results into the response struct.
			processedResponses[index] = ClothingResponse{
				ID:           item.ID,
				Name:         item.Name,
				Description:  item.Description,
				ClothingType: item.ClothingType,
				Status:       item.Status,
				CreatedAt:    item.CreatedAt.Format("2006-01-02T15:04:05Z"),
				UpdatedAt:    item.UpdatedAt.Format("2006-01-02T15:04:05Z"),
				Uri:          &imageUrl,
			}
		}(i, clothingItem)
	}

	wg.Wait()
	return processedResponses
}

func (controller *ClothesController) ListClothes(c echo.Context) error {
	// Get user and db from context
	user, ok := c.Get("currentUser").(models.UserAccount)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}
	db, ok := c.Get("__db").(*gorm.DB)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Database connection error"})
	}

	// Get all clothes for the user
	var clothes []models.Clothing
	if err := db.Where("owner_id = ? AND company_id = ?", user.ID, user.Memberships[0].CompanyID).Find(&clothes).Error; err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch clothes"})
	}
	// --- 3. Delegate all complex processing to our new helper function ---
	processedResponses := controller.populatePresignedClothingImages(c.Request().Context(), clothes)

	// --- 4. Group the fully-processed results (simple, fast, and readable) ---
	response := ClothesListResponse{
		Tops:        []ClothingResponse{},
		Bottoms:     []ClothingResponse{},
		Shoes:       []ClothingResponse{},
		Accessories: []ClothingResponse{},
	}

	for _, resp := range processedResponses {
		switch resp.ClothingType {
		case "top":
			response.Tops = append(response.Tops, resp)
		case "bottom":
			response.Bottoms = append(response.Bottoms, resp)
		case "shoes":
			response.Shoes = append(response.Shoes, resp)
		case "accessory":
			response.Accessories = append(response.Accessories, resp)
		}
	}

	return c.JSON(http.StatusOK, response)
}
