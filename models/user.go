package models

import "time"

type UserAccount struct {
	JsonModel
	Name     string `json:"name"`
	Email    string `json:"email" gorm:"unique"`
	Password string `json:"-"`
	Banned   bool   `gorm:"default:false" json:"-"`
	LastIp   string `json:"-"`
	//"INVITATION_PENDING", "STARTED_AUTH", "FINISHED_AUTH"
	Status              string            `json:"-"`
	GoogleID            string            `json:"-"`
	AppleID             string            `json:"-"`
	UTMSource           string            `json:"utm_source"`
	FacebookID          string            `json:"-"`
	Platform            Platform          `sql:"type:ENUM('ios', 'android', 'web')" json:"platform"`
	Memberships         []UserCompanyRole `gorm:"foreignKey:UserAccountID"`
	AdminInCompanys     []Company         `gorm:"foreignKey:OwnerID"`
	TelegramUsername    string            `json:"telegram_username"`
	Subscription        *string           `json:"subscription"`
	ExpirationDate      *time.Time        `json:"-"`
	ConfirmedDeleteDate *time.Time        `json:"-"`
	// Notifications settings
	ReceiveNotifications bool `json:"receive_notifications"`
	// mainly for LLM models token explanation etc
	IsSuperadmin bool `json:"is_superadmin"`
	// user app image/avatar
	AvatarURL string `json:"avatar_url"`

	AvatarProcessRetryTimes              int     `json:"-"`
	FullBodyAvatarStatus                 string  `json:"full_body_avatar_status"` // idle, processing, completed, failed
	FullBodyAvatarProcessingErrorMessage *string `json:"full_body_avatar_processing_error_message"`
	// deprecated
	FullBodyAvatarSet bool `json:"full_body_avatar_set"`
	// user full body avatar for try ons!
	UserFullBodyImageURL  *string `json:"user_image_url"`
	LLMTotalTokenCount    *int32  `json:"llm_total_token_count"`
	LLMInputTokenCount    *int32  `json:"llm_input_token_count"`
	LLMThoughtsTokenCount *int32  `json:"llm_thoughts_token_count"`
	LLMOutputTokenCount   *int32  `json:"llm_output_token_count"`
	LLMThoughts           *string `json:"llm_thoughts"`
	LLMModel              *string `json:"llm_model"`
	// Person characteristics for avatar generation
	BodyType       *string `json:"body_type"`
	ShoulderType   *string `json:"shoulder_type"`
	BodyToLegRatio *string `json:"body_to_leg_ratio"`
	HandType       *string `json:"hand_type"`
	UpperLimbType  *string `json:"upper_limb_type"`
	Weight         *int    `json:"weight"`
	Height         *string `json:"height"`
	WaistSize      *int    `json:"waist_size"`
	// Active                    bool `json:"active"`
}

type UserPushToken struct {
	JsonModel
	UserAccountID uint
	UserAccount   UserAccount `json:"user_account"`
	Platform      Platform    `sql:"type:ENUM('ios', 'android', 'web')" json:"platform"`
	Token         string      `json:"token"`
	Active        bool        `gorm:"default:false" json:"-"`
}

type UserPushIn struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

type UserSettingsIn struct {
	ReceiveSalesNotifications bool `json:"receive_notifications"`
	// Platform string `json:"platform"`
}

type UserCompanyRole struct {
	JsonModel
	UserAccountID    uint
	UserAccount      UserAccount `json:"user_account"`
	Active           bool        `gorm:"default:false" json:"-"`
	Role             Role        `sql:"type:ENUM('OWNER', 'ADMIN', 'SALES')" json:"role"`
	InviteCode       *string     `json:"-"`
	InviteAcceptedAt *int64      `json:"invite_accepted_at"`
	CompanyID        uint
	Company          Company `json:"company"`
}

type Company struct {
	JsonModel
	Name                       string            `json:"name"`
	Address                    *string           `json:"address"`
	ImageUrl                   *string           `json:"image_url"`
	Owner                      UserAccount       `json:"-"`
	OwnerID                    uint              `json:"-"`
	Subscription               Subscription      `json:"subscription"`
	TrialStartedDate           *int64            `json:"trial_started_date"`
	TrialDays                  *uint             `json:"trial_days"`
	Members                    []UserCompanyRole `json:"members"`
	Currency                   string            `json:"currency"`
	Language                   string            `json:"language"`
	Active                     bool              `json:"active"`
	EnforcedDailyNoteLimit     *int32            `json:"enforced_daily_note_limit"`
	EnforcedDailyClothingLimit *int32            `json:"enforced_daily_clothing_limit"`
	EnforcedDailyTryOnLimit    *int32            `json:"enforced_daily_try_on_limit"`
	EnforcedLLMModel           *int32            `json:"enforced_llm_model"`
	FullAdminAccess            bool              `json:"full_admin_access"`
}

type CompanySubscription struct {
	JsonModel

	Subscription Subscription `json:"subscription"`

	PaidDate time.Time `json:"paid_date"`
	IsAnnual bool      `json:"is_annual"`
}

// ---
type PlayerIn struct {
	DeviceId string   `json:"device_id" validate:"required"`
	Platform Platform `json:"platform" validate:"required,platform"` //ios,android,web
}

type PlayerDeviceIn struct {
	// tgusername or random id
	DeviceId string `json:"device_id" validate:"required"`
}
