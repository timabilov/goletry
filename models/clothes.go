package models

type Clothing struct {
	JsonModel
	Name                string      `json:"name"`
	Description         *string     `gorm:"type:text" json:"description"`
	ClothingType        string      `json:"clothing_type"` // e.g., top, bottom, shoes, accessory
	Owner               UserAccount `json:"-"`
	OwnerID             uint        `json:"-"`
	CompanyID           uint        `json:"-"`
	Company             Company     `json:"company"`
	Status              string      `json:"status"`            // temporary, in_closet
	ImageStatus         string      `json:"image_status"`      // draft, uploaded
	ProcessingStatus    string      `json:"processing_status"` // idle, generating, completed, failed
	ProcessRetryTimes   int         `json:"process_retry_times"`
	ProcessErrorMessage *string     `json:"process_error_message"`
	ImageURL            *string     `json:"image_url"`
}

// Removed Folder and Question models - not needed for fashion AI

type ClothingTryonGeneration struct {
	JsonModel
	TopClothingID    *uint       `json:"top_clothing_id"`
	TopClothing      *Clothing   `json:"top_clothing"`
	BottomClothingID *uint       `json:"bottom_clothing_id"`
	BottomClothing   *Clothing   `json:"bottom_clothing"`
	ShoesClothingID  *uint       `json:"shoes_clothing_id"`
	ShoesClothing    *Clothing   `json:"shoes_clothing"`
	AccessoryID      *uint       `json:"accessory_id"`
	Accessory        *Clothing   `json:"accessory"`
	UserAccountID    uint        `json:"-"`
	UserAccount      UserAccount `json:"user_account"`
	CompanyID        uint        `json:"company_id"`
	Company          Company     `json:"company"`

	// user avatar at the point of generation
	GeneratedWithAvatarURL string `json:"generated_with_avatar_url"`

	TryOnPreviewImageURL   *string  `json:"try_on_preview_image_url"`
	Status                 string   `json:"status"`   // pending, completed, failed
	Duration               *float64 `json:"duration"` // in seconds
	LLMTokenUsage          *int     `json:"llm_token_usage"`
	LLMModel               *string  `json:"llm_model"`
	LLMInputTokenCount     *int32   `json:"llm_input_token_usage"`
	LLMOutputTokenCount    *int32   `json:"llm_output_token_usage"`
	LLMTotalTokenCount     *int32   `json:"llm_total_token_usage"`
	LLMThoughtsTokenCount  *int32   `json:"llm_thoughts_token_count"`
	LLMThoughts            *string  `json:"llm_thoughts"`
	GenerationRetryTimes   int      `json:"generation_retry_times"`
	GenerationErrorMessage *string  `json:"generation_error_message"`
}
